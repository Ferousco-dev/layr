package generation

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/account"
	"github.com/ferousco-dev/layr/server/internal/designapi"
)

type memStore struct {
	mu     sync.Mutex
	rows   map[string]*Generation
	seq    int
	owners map[string]string // project -> owner
	saves  int
}

func newMem() *memStore {
	return &memStore{rows: map[string]*Generation{}, owners: map[string]string{"p1": "u1", "p2": "u2"}}
}

func (m *memStore) Create(_ context.Context, owner, project, provider string, screens []string, steps []Step, now time.Time) (Generation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.owners[project] != owner {
		return Generation{}, ErrNotFound
	}
	for _, g := range m.rows {
		if g.ProjectID == project && g.Status == StatusRunning {
			return Generation{}, ErrConflict
		}
	}
	m.seq++
	g := &Generation{ID: "g" + string(rune('0'+m.seq)), ProjectID: project, Provider: provider, ScreenIDs: screens, Status: StatusRunning, Steps: append([]Step(nil), steps...), CreatedAt: now, UpdatedAt: now}
	m.rows[g.ID] = g
	return *g, nil
}

func (m *memStore) Get(_ context.Context, owner, project, id string) (Generation, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.rows[id]
	if !ok || g.ProjectID != project || m.owners[project] != owner {
		return Generation{}, ErrNotFound
	}
	cp := *g
	cp.Steps = append([]Step(nil), g.Steps...)
	return cp, nil
}

func (m *memStore) SaveProgress(_ context.Context, id string, steps []Step, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.saves++
	m.rows[id].Steps, m.rows[id].UpdatedAt = append([]Step(nil), steps...), now
	return nil
}

func (m *memStore) Finish(_ context.Context, id, status, code string, steps []Step, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.rows[id]
	g.Status, g.ErrorCode, g.Steps, g.UpdatedAt, g.CompletedAt = status, code, append([]Step(nil), steps...), now, &now
	return nil
}

func (m *memStore) FailAbandoned(_ context.Context, before time.Time, code string, now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, g := range m.rows {
		if g.Status == StatusRunning && g.UpdatedAt.Before(before) {
			g.Status, g.ErrorCode, g.UpdatedAt = StatusFailed, code, now
			n++
		}
	}
	return n, nil
}

type fakeDesigns struct{ status string }

func (f fakeDesigns) Design(context.Context, string, string) (*designapi.Design, error) {
	return &designapi.Design{Status: f.status, Counts: &designapi.Counts{Screens: 5, SharedComponents: 1}}, nil
}
func (fakeDesigns) ScreenIDs(context.Context, string, string) ([]string, error) {
	return []string{"s1", "s2", "s3", "s4", "s5"}, nil
}
func (fakeDesigns) Tokens(context.Context, string, string) (*designapi.DesignTokens, error) {
	return &designapi.DesignTokens{Colors: make([]designapi.ColorSwatch, 3), Fonts: make([]designapi.FontFamily, 2)}, nil
}

type fakeKeys map[string]string // user+provider -> key

func (f fakeKeys) OpenKey(_ context.Context, user, provider string) (string, error) {
	if k, ok := f[user+provider]; ok {
		return k, nil
	}
	return "", account.ErrKeyNotFound
}

const secretKey = "sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789"

func newService(t *testing.T, stages []Stage) (*Service, *memStore) {
	t.Helper()
	store := newMem()
	keys := fakeKeys{"u1anthropic": secretKey}
	var svc *Service
	if stages == nil {
		svc = NewService(store, fakeDesigns{status: designapi.StatusReady}, keys, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	} else {
		svc = NewServiceWithStages(store, fakeDesigns{status: designapi.StatusReady}, keys, stages, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	}
	t.Cleanup(func() { svc.Close(context.Background()) })
	return svc, store
}

func settle(t *testing.T, svc *Service, id string) Generation {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		g, _ := svc.Get(context.Background(), "u1", "p1", id)
		if g.Status != StatusRunning {
			return g
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return Generation{}
}

func TestTheRealStepsRunThenStopHonestlyAtWritingCode(t *testing.T) {
	svc, _ := newService(t, nil)
	gen, err := svc.Start(context.Background(), "u1", "p1", "anthropic", nil)
	if err != nil || gen.Status != StatusRunning || len(gen.Steps) != 4 {
		t.Fatalf("start = %+v, %v", gen, err)
	}
	done := settle(t, svc, gen.ID)

	if done.Status != StatusFailed || done.ErrorCode != CodeNotAvailable {
		t.Fatalf("final = %s / %s", done.Status, done.ErrorCode)
	}
	want := []struct{ id, status, detail string }{
		{"read_design", StepDone, "5 screens, 1 shared component"},
		{"extract_tokens", StepDone, "3 colors, 2 fonts"},
		{"prepare_model", StepDone, "ending in 6789"},
		{"write_code", StepFailed, "not available yet"},
	}
	for i, w := range want {
		s := done.Steps[i]
		if s.ID != w.id || s.Status != w.status || !strings.Contains(s.Detail, w.detail) || s.StartedAt == nil || s.FinishedAt == nil {
			t.Errorf("step %d = %+v, want %+v", i, s, w)
		}
	}
	if !strings.Contains(done.Steps[2].Label, "Claude (Anthropic)") || !strings.Contains(done.Steps[3].Label, "Claude (Anthropic)") {
		t.Fatalf("labels must name the model: %q / %q", done.Steps[2].Label, done.Steps[3].Label)
	}
	for _, s := range done.Steps {
		if strings.Contains(s.Detail, "abcdefghij") || strings.Contains(s.Detail, "sk-ant") {
			t.Fatalf("a step leaked the key: %+v", s)
		}
	}
}

func TestProgressIsSavedAsTheJobRuns(t *testing.T) {
	gate := make(chan struct{})
	stages := []Stage{
		{ID: "one", Label: fixed("One"), Run: func(context.Context, *Job) (string, error) { return "first done", nil }},
		{ID: "two", Label: fixed("Two"), Run: func(ctx context.Context, _ *Job) (string, error) {
			select {
			case <-gate:
				return "second done", nil
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}},
	}
	svc, _ := newService(t, stages)
	gen, err := svc.Start(context.Background(), "u1", "p1", "anthropic", nil)
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.Now().Add(2 * time.Second)
	var mid Generation
	for time.Now().Before(deadline) {
		mid, _ = svc.Get(context.Background(), "u1", "p1", gen.ID)
		if mid.Steps[0].Status == StepDone && mid.Steps[1].Status == StepRunning {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if mid.Status != StatusRunning || mid.Steps[0].Detail != "first done" || mid.Steps[1].Status != StepRunning {
		t.Fatalf("a poll during the run must see real progress: %+v", mid)
	}
	close(gate)
	if final := settle(t, svc, gen.ID); final.Status != StatusCompleted || final.Steps[1].Status != StepDone || final.CompletedAt == nil {
		t.Fatalf("final = %+v", final)
	}
}

func TestBadRequestsNeverCreateAJob(t *testing.T) {
	svc, store := newService(t, nil)
	ctx := context.Background()
	if _, err := svc.Start(ctx, "u1", "p1", "gemini", nil); !errors.Is(err, ErrUnknownProvider) {
		t.Errorf("unknown provider: %v", err)
	}
	if _, err := svc.Start(ctx, "u1", "p1", "openai", nil); !errors.Is(err, ErrNoKey) {
		t.Errorf("no key saved: %v", err)
	}
	if _, err := svc.Start(ctx, "u2", "p2", "anthropic", nil); !errors.Is(err, ErrNoKey) {
		t.Errorf("another person's key must not be used: %v", err)
	}
	notReady := NewService(store, fakeDesigns{status: designapi.StatusImporting}, fakeKeys{"u1anthropic": secretKey}, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)))
	defer notReady.Close(ctx)
	if _, err := notReady.Start(ctx, "u1", "p1", "anthropic", nil); !errors.Is(err, ErrDesignNotReady) {
		t.Errorf("design still importing: %v", err)
	}
	if len(store.rows) != 0 {
		t.Fatalf("jobs created: %d", len(store.rows))
	}
}

func TestOneJobRunsPerProjectAndStrangersCannotReadIt(t *testing.T) {
	gate := make(chan struct{})
	stages := []Stage{{ID: "wait", Label: fixed("Wait"), Run: func(ctx context.Context, _ *Job) (string, error) {
		select {
		case <-gate:
			return "", nil
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}}}
	svc, _ := newService(t, stages)
	ctx := context.Background()
	gen, err := svc.Start(ctx, "u1", "p1", "anthropic", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Start(ctx, "u1", "p1", "anthropic", nil); !errors.Is(err, ErrConflict) {
		t.Fatalf("second start: %v", err)
	}
	if _, err := svc.Get(ctx, "u2", "p1", gen.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("stranger: %v", err)
	}
	if _, err := svc.Get(ctx, "u1", "p2", gen.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong project: %v", err)
	}
	close(gate)
	settle(t, svc, gen.ID)
	if _, err := svc.Start(ctx, "u1", "p1", "anthropic", nil); err != nil {
		t.Fatalf("a new job after the last one finished: %v", err)
	}
}

func TestAPanicFailsOnlyThatJob(t *testing.T) {
	svc, _ := newService(t, []Stage{{ID: "boom", Label: fixed("Boom"), Run: func(context.Context, *Job) (string, error) { panic("SECRET key material") }}})
	gen, _ := svc.Start(context.Background(), "u1", "p1", "anthropic", nil)
	if done := settle(t, svc, gen.ID); done.Status != StatusFailed || done.ErrorCode != CodeInternal {
		t.Fatalf("done = %+v", done)
	}
}

func TestShutdownStopsRunningJobsAndRecordsWhy(t *testing.T) {
	svc, store := newService(t, []Stage{{ID: "wait", Label: fixed("Wait"), Run: func(ctx context.Context, _ *Job) (string, error) {
		<-ctx.Done()
		return "", ctx.Err()
	}}})
	gen, _ := svc.Start(context.Background(), "u1", "p1", "anthropic", nil)
	time.Sleep(30 * time.Millisecond)
	svc.Close(context.Background())
	got := store.rows[gen.ID]
	if got.Status != StatusFailed || got.ErrorCode != CodeInterrupted {
		t.Fatalf("after shutdown = %s / %s", got.Status, got.ErrorCode)
	}
}

func TestSweepFailsJobsAbandonedByACrashedProcess(t *testing.T) {
	svc, store := newService(t, nil)
	old := time.Now().Add(-time.Hour)
	store.rows["old"] = &Generation{ID: "old", ProjectID: "p1", Status: StatusRunning, UpdatedAt: old}
	store.rows["fresh"] = &Generation{ID: "fresh", ProjectID: "p2", Status: StatusRunning, UpdatedAt: time.Now()}
	svc.Sweep(context.Background())
	if store.rows["old"].Status != StatusFailed || store.rows["old"].ErrorCode != CodeInterrupted || store.rows["fresh"].Status != StatusRunning {
		t.Fatalf("old = %+v fresh = %+v", store.rows["old"], store.rows["fresh"])
	}
}

func TestChoosingScreensIsCheckedAgainstTheDesign(t *testing.T) {
	svc, _ := newService(t, nil)
	ctx := context.Background()

	for name, ids := range map[string][]string{"unknown screen": {"s1", "nope"}, "empty list": {}} {
		if _, err := svc.Start(ctx, "u1", "p1", "anthropic", ids); !errors.Is(err, ErrInvalidSelection) {
			t.Errorf("%s: %v", name, err)
		}
	}

	gen, err := svc.Start(ctx, "u1", "p1", "anthropic", []string{"s2", "s4", "s2"})
	if err != nil || len(gen.ScreenIDs) != 2 || gen.ScreenIDs[0] != "s2" || gen.ScreenIDs[1] != "s4" {
		t.Fatalf("subset = %+v, %v", gen, err)
	}
	done := settle(t, svc, gen.ID)
	if !strings.Contains(done.Steps[0].Detail, "2 of 5 screens selected") {
		t.Fatalf("the first step must say what was chosen: %q", done.Steps[0].Detail)
	}

	all, err := svc.Start(ctx, "u1", "p1", "anthropic", []string{"s1", "s2", "s3", "s4", "s5"})
	if err != nil || all.ScreenIDs != nil {
		t.Fatalf("choosing every screen is the same as all: %+v, %v", all, err)
	}
	if final := settle(t, svc, all.ID); !strings.Contains(final.Steps[0].Detail, "5 screens") || strings.Contains(final.Steps[0].Detail, "selected") {
		t.Fatalf("all: %q", final.Steps[0].Detail)
	}
}
