package pgstore

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/imports"
	"github.com/jackc/pgx/v5/pgxpool"
)

var t0 = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

type fixture struct {
	s                  *Store
	pool               *pgxpool.Pool
	userA, userB       string
	projectA, projectB string
}

func live(t *testing.T) fixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `TRUNCATE users CASCADE`); err != nil {
		t.Fatal(err)
	}
	f := fixture{s: New(pool), pool: pool}
	for _, u := range []struct {
		fig string
		dst *string
	}{{"imp-a", &f.userA}, {"imp-b", &f.userB}} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (figma_user_id, display_name, created_at, updated_at) VALUES ($1,'n',now(),now()) RETURNING id`, u.fig).Scan(u.dst); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []struct {
		owner string
		dst   *string
	}{{f.userA, &f.projectA}, {f.userB, &f.projectB}} {
		if err := pool.QueryRow(ctx, `INSERT INTO projects (user_id, name, created_at, updated_at) VALUES ($1,'p',now(),now()) RETURNING id`, p.owner).Scan(p.dst); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f fixture) create(t *testing.T, node string) imports.Import {
	t.Helper()
	imp, err := f.s.Create(context.Background(), f.userA, f.projectA, "FILEKEY123456", node, t0, t0.Add(-10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	return imp
}

func TestCreateEnforcesOwnership(t *testing.T) {
	f := live(t)
	ctx := context.Background()

	if _, err := f.s.Create(ctx, f.userB, f.projectA, "K", "", t0, t0); !errors.Is(err, imports.ErrProjectNotFound) {
		t.Fatalf("foreign project: %v", err)
	}
	if _, err := f.s.Create(ctx, f.userA, "00000000-0000-4000-8000-000000000000", "K", "", t0, t0); !errors.Is(err, imports.ErrProjectNotFound) {
		t.Fatalf("unknown project: %v", err)
	}
	imp := f.create(t, "1:2")
	if imp.Status != imports.StatusPending || imp.NodeID != "1:2" || imp.ProjectID != f.projectA || imp.FileKey != "FILEKEY123456" {
		t.Fatalf("imp = %+v", imp)
	}

	_, _ = f.pool.Exec(ctx, `UPDATE projects SET deleted_at = now() WHERE id = $1`, f.projectA)
	if _, err := f.s.Create(ctx, f.userA, f.projectA, "K", "", t0, t0); !errors.Is(err, imports.ErrProjectNotFound) {
		t.Fatalf("deleted project: %v", err)
	}
	if _, err := f.s.Get(ctx, f.userA, f.projectA, imp.ID); !errors.Is(err, imports.ErrNotFound) {
		t.Fatalf("deleted project read: %v", err)
	}
}

func TestReadsAreOwnerAndProjectScoped(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	imp := f.create(t, "")

	if got, err := f.s.Get(ctx, f.userA, f.projectA, imp.ID); err != nil || got.ID != imp.ID {
		t.Fatalf("owner get: %v", err)
	}
	if got, err := f.s.Latest(ctx, f.userA, f.projectA); err != nil || got.ID != imp.ID {
		t.Fatalf("owner latest: %v", err)
	}
	if _, err := f.s.Get(ctx, f.userB, f.projectA, imp.ID); !errors.Is(err, imports.ErrNotFound) {
		t.Fatalf("foreign get: %v", err)
	}
	if _, err := f.s.Get(ctx, f.userB, f.projectB, imp.ID); !errors.Is(err, imports.ErrNotFound) {
		t.Fatalf("cross-project get: %v", err)
	}
	if _, err := f.s.Latest(ctx, f.userB, f.projectA); !errors.Is(err, imports.ErrNotFound) {
		t.Fatalf("foreign latest: %v", err)
	}
}

func TestStatusTransitionsAreGuarded(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	imp := f.create(t, "1:2")

	if err := f.s.Complete(ctx, imp.ID, imports.Result{}, t0); !errors.Is(err, imports.ErrConflict) {
		t.Fatalf("complete from pending: %v", err)
	}
	if err := f.s.AwaitSelection(ctx, imp.ID, "n", "v", nil, t0); !errors.Is(err, imports.ErrConflict) {
		t.Fatalf("await from pending: %v", err)
	}
	if err := f.s.MarkProcessing(ctx, imp.ID, t0); err != nil {
		t.Fatal(err)
	}
	if err := f.s.MarkProcessing(ctx, imp.ID, t0); !errors.Is(err, imports.ErrConflict) {
		t.Fatalf("second start: %v", err)
	}
	res := imports.Result{FileName: "Landing", Version: "9", NodeID: "1:2", NodeName: "Desktop", RenderFormat: "png", RenderScale: 1, AssetCount: 7, WarningCount: 2,
		NodeIDs: []string{"1:2", "1:3"}, ScreenCount: 2, DesignNodes: 40, DesignWarnings: 3}
	if err := f.s.Complete(ctx, imp.ID, res, t0.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}

	got, _ := f.s.Get(ctx, f.userA, f.projectA, imp.ID)
	if got.Status != imports.StatusCompleted || got.NodeName != "Desktop" || got.Version != "9" || got.RenderScale != 1 || got.AssetCount != 7 || got.WarningCount != 2 || got.ScreenCount != 2 || got.DesignNodes != 40 || got.DesignWarnings != 3 || len(got.NodeIDs) != 2 || got.CompletedAt == nil || !got.CompletedAt.Equal(t0.Add(time.Minute)) {
		t.Fatalf("got = %+v", got)
	}
	if err := f.s.Fail(ctx, imp.ID, "X", t0); err != nil {
		t.Fatal(err)
	}
	if again, _ := f.s.Get(ctx, f.userA, f.projectA, imp.ID); again.Status != imports.StatusCompleted || again.ErrorCode != "" {
		t.Fatalf("a completed import was changed by a late failure: %+v", again)
	}
	if err := f.s.Complete(ctx, imp.ID, res, t0); !errors.Is(err, imports.ErrConflict) {
		t.Fatalf("double complete: %v", err)
	}
}

func awaiting(t *testing.T, f fixture, frames ...imports.Frame) imports.Import {
	t.Helper()
	ctx := context.Background()
	imp := f.create(t, "")
	_ = f.s.MarkProcessing(ctx, imp.ID, t0)
	if err := f.s.AwaitSelection(ctx, imp.ID, "Landing", "5", frames, t0); err != nil {
		t.Fatal(err)
	}
	return imp
}

var threeFrames = []imports.Frame{{ID: "1:2", Name: "Desktop", Type: "FRAME", Page: "Home"}, {ID: "1:3", Name: "Mobile", Type: "FRAME", Page: "Home"}, {ID: "1:4", Name: "Kit", Type: "SECTION"}}

func TestSelectionMustMatchAStoredCandidate(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	imp := awaiting(t, f, threeFrames...)

	if got, _ := f.s.Get(ctx, f.userA, f.projectA, imp.ID); len(got.Candidates) != 3 || got.FileName != "Landing" || got.Candidates[0].Page != "Home" {
		t.Fatalf("candidates = %+v", got)
	}
	pick := func(ids ...string) imports.Selection { return imports.Selection{NodeIDs: ids} }
	for name, sel := range map[string]imports.Selection{
		"unknown node": pick("9:9"), "one unknown among valid": pick("1:2", "9:9"), "duplicate": pick("1:2", "1:2"), "empty": pick(),
		"injection": pick("1:2'; DROP TABLE figma_imports;--"),
	} {
		if _, err := f.s.BeginSelection(ctx, f.userA, f.projectA, imp.ID, sel, 10, t0); !errors.Is(err, imports.ErrInvalidSelection) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := f.s.BeginSelection(ctx, f.userB, f.projectA, imp.ID, pick("1:2"), 10, t0); !errors.Is(err, imports.ErrNotFound) {
		t.Fatalf("foreign user: %v", err)
	}
	if got, _ := f.s.Get(ctx, f.userA, f.projectA, imp.ID); got.Status != imports.StatusAwaitingSelection {
		t.Fatalf("rejected selections changed the import: %+v", got)
	}

	chosen, err := f.s.BeginSelection(ctx, f.userA, f.projectA, imp.ID, pick("1:3"), 10, t0)
	if err != nil || chosen.Status != imports.StatusProcessing || chosen.NodeID != "1:3" || len(chosen.NodeIDs) != 1 {
		t.Fatalf("chosen = %+v, err = %v", chosen, err)
	}
	if _, err := f.s.BeginSelection(ctx, f.userA, f.projectA, imp.ID, pick("1:2"), 10, t0); !errors.Is(err, imports.ErrConflict) {
		t.Fatalf("second selection: %v", err)
	}
}

func TestSeveralScreensOrAllCanBeSelectedInCandidateOrder(t *testing.T) {
	f := live(t)
	ctx := context.Background()

	subset := awaiting(t, f, threeFrames...)
	got, err := f.s.BeginSelection(ctx, f.userA, f.projectA, subset.ID, imports.Selection{NodeIDs: []string{"1:4", "1:2"}}, 10, t0)
	if err != nil || got.NodeID != "1:2" || len(got.NodeIDs) != 2 || got.NodeIDs[0] != "1:2" || got.NodeIDs[1] != "1:4" {
		t.Fatalf("subset = %+v, err = %v", got, err)
	}
	_ = f.s.Fail(ctx, subset.ID, "X", t0)

	all := awaiting(t, f, threeFrames...)
	got, err = f.s.BeginSelection(ctx, f.userA, f.projectA, all.ID, imports.Selection{All: true}, 10, t0)
	if err != nil || len(got.NodeIDs) != 3 || got.NodeIDs[2] != "1:4" {
		t.Fatalf("all = %+v, err = %v", got, err)
	}
	_ = f.s.Fail(ctx, all.ID, "X", t0)

	tooMany := awaiting(t, f, threeFrames...)
	if _, err := f.s.BeginSelection(ctx, f.userA, f.projectA, tooMany.ID, imports.Selection{All: true}, 2, t0); !errors.Is(err, imports.ErrInvalidSelection) {
		t.Fatalf("more screens than allowed: %v", err)
	}
}

func TestConcurrentSelectionsChooseExactlyOnce(t *testing.T) {
	f := live(t)
	imp := awaiting(t, f, threeFrames...)

	var wg sync.WaitGroup
	var mu sync.Mutex
	won := 0
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.BeginSelection(context.Background(), f.userA, f.projectA, imp.ID, imports.Selection{NodeIDs: []string{"1:2"}}, 10, t0)
			mu.Lock()
			defer mu.Unlock()
			if err == nil {
				won++
			} else if !errors.Is(err, imports.ErrConflict) {
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()
	if won != 1 {
		t.Fatalf("selections that won: %d", won)
	}
}

func TestOnlyOneRunningImportPerProject(t *testing.T) {
	f := live(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	var mu sync.Mutex
	won, conflicts := 0, 0
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := f.s.Create(ctx, f.userA, f.projectA, "FILEKEY123456", "", t0, t0.Add(-time.Hour))
			mu.Lock()
			defer mu.Unlock()
			switch {
			case err == nil:
				won++
			case errors.Is(err, imports.ErrConflict):
				conflicts++
			default:
				t.Errorf("unexpected: %v", err)
			}
		}()
	}
	wg.Wait()

	if won != 1 || conflicts != 11 {
		t.Fatalf("won %d conflicts %d", won, conflicts)
	}
}

func TestNewImportSupersedesWaitingAndStaleOnes(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	waiting := f.create(t, "")
	_ = f.s.MarkProcessing(ctx, waiting.ID, t0)
	_ = f.s.AwaitSelection(ctx, waiting.ID, "n", "v", []imports.Frame{{ID: "1:2"}}, t0)

	next, err := f.s.Create(ctx, f.userA, f.projectA, "FILEKEY123456", "", t0.Add(time.Minute), t0)
	if err != nil {
		t.Fatal(err)
	}
	old, _ := f.s.Get(ctx, f.userA, f.projectA, waiting.ID)
	if old.Status != imports.StatusFailed || old.ErrorCode != imports.CodeSuperseded {
		t.Fatalf("waiting import = %+v", old)
	}

	if _, err := f.s.Create(ctx, f.userA, f.projectA, "K", "", t0.Add(2*time.Minute), t0); !errors.Is(err, imports.ErrConflict) {
		t.Fatalf("fresh running import must block: %v", err)
	}
	staleNow := t0.Add(time.Hour)
	if _, err := f.s.Create(ctx, f.userA, f.projectA, "K", "", staleNow, staleNow.Add(-10*time.Minute)); err != nil {
		t.Fatalf("stale running import must not block: %v", err)
	}
	if got, _ := f.s.Get(ctx, f.userA, f.projectA, next.ID); got.ErrorCode != imports.CodeSuperseded {
		t.Fatalf("stale import = %+v", got)
	}
	if latest, _ := f.s.Latest(ctx, f.userA, f.projectA); latest.ID == next.ID {
		t.Fatal("latest should be the newest import")
	}
}

func TestRunningIDsAndProjectCascade(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	imp := f.create(t, "")

	ids, err := f.s.RunningIDs(ctx)
	if err != nil || !ids[imp.ID] || len(ids) != 1 {
		t.Fatalf("ids = %v, err = %v", ids, err)
	}
	_ = f.s.Fail(ctx, imp.ID, "X", t0)
	if ids, _ := f.s.RunningIDs(ctx); len(ids) != 0 {
		t.Fatalf("failed import still running: %v", ids)
	}

	_, _ = f.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, f.projectA)
	var n int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM figma_imports`).Scan(&n)
	if n != 0 {
		t.Fatalf("orphaned imports: %d", n)
	}
}

func TestDatabaseRejectsUnknownStatus(t *testing.T) {
	f := live(t)
	imp := f.create(t, "")

	_, err := f.pool.Exec(context.Background(), `UPDATE figma_imports SET status = 'generating' WHERE id = $1`, imp.ID)

	if err == nil {
		t.Fatal("database accepted a status outside the lifecycle")
	}
}

func TestFailAbandonedRetiresOnlyOldRunningImports(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	old := f.create(t, "1:2")
	if _, err := f.s.FailAbandoned(ctx, t0.Add(-time.Hour), imports.CodeInterrupted, t0); err != nil {
		t.Fatal(err)
	}
	if got, _ := f.s.Get(ctx, f.userA, f.projectA, old.ID); got.Status != imports.StatusPending {
		t.Fatalf("a fresh import must be left alone: %+v", got)
	}

	n, err := f.s.FailAbandoned(ctx, t0.Add(time.Minute), imports.CodeInterrupted, t0.Add(time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("retired = %d err %v", n, err)
	}
	got, _ := f.s.Get(ctx, f.userA, f.projectA, old.ID)
	if got.Status != imports.StatusFailed || got.ErrorCode != imports.CodeInterrupted {
		t.Fatalf("abandoned import = %+v", got)
	}
	if again, _ := f.s.FailAbandoned(ctx, t0.Add(time.Minute), imports.CodeInterrupted, t0); again != 0 {
		t.Fatalf("a failed import must not be retired twice: %d", again)
	}
	if _, err := f.s.Create(ctx, f.userA, f.projectA, "FILEKEY123456", "1:2", t0.Add(2*time.Hour), t0.Add(2*time.Hour-10*time.Minute)); err != nil {
		t.Fatalf("the project must accept a new import afterwards: %v", err)
	}
}
