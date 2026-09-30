package pgstore

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/generation"
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
	for fig, dst := range map[string]*string{"gen-a": &f.userA, "gen-b": &f.userB} {
		if err := pool.QueryRow(ctx, `INSERT INTO users (figma_user_id, display_name, created_at, updated_at) VALUES ($1,'n',now(),now()) RETURNING id`, fig).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	for owner, dst := range map[string]*string{f.userA: &f.projectA, f.userB: &f.projectB} {
		if err := pool.QueryRow(ctx, `INSERT INTO projects (user_id, name, created_at, updated_at) VALUES ($1,'p',now(),now()) RETURNING id`, owner).Scan(dst); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

var plan = []generation.Step{{ID: "a", Label: "A", Status: generation.StepPending}, {ID: "b", Label: "B", Status: generation.StepPending}}

func TestCreateChecksOwnershipAndAllowsOneRunningJob(t *testing.T) {
	f := live(t)
	ctx := context.Background()

	if _, err := f.s.Create(ctx, f.userB, f.projectA, "anthropic", nil, plan, t0); !errors.Is(err, generation.ErrNotFound) {
		t.Fatalf("a stranger's project: %v", err)
	}
	g, err := f.s.Create(ctx, f.userA, f.projectA, "anthropic", nil, plan, t0)
	if err != nil || g.Status != generation.StatusRunning || len(g.Steps) != 2 {
		t.Fatalf("create = %+v, %v", g, err)
	}
	if _, err := f.s.Create(ctx, f.userA, f.projectA, "openai", nil, plan, t0); !errors.Is(err, generation.ErrConflict) {
		t.Fatalf("second running job: %v", err)
	}
	if _, err := f.s.Create(ctx, f.userB, f.projectB, "xai", nil, plan, t0); err != nil {
		t.Fatalf("another project runs independently: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO generations (project_id, provider, status, steps, created_at, updated_at) VALUES ($1,'gemini','running','[]',now(),now())`, f.projectB); err == nil {
		t.Fatal("the database must refuse an unknown provider")
	}
}

func TestProgressAndFinishRoundTripAndGetIsOwnerScoped(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	g, _ := f.s.Create(ctx, f.userA, f.projectA, "anthropic", nil, plan, t0)

	started := t0.Add(time.Second)
	steps := []generation.Step{{ID: "a", Label: "A", Status: generation.StepDone, Detail: "5 screens", StartedAt: &t0, FinishedAt: &started}, {ID: "b", Label: "B", Status: generation.StepRunning}}
	if err := f.s.SaveProgress(ctx, g.ID, steps, started); err != nil {
		t.Fatal(err)
	}
	mid, err := f.s.Get(ctx, f.userA, f.projectA, g.ID)
	if err != nil || mid.Steps[0].Detail != "5 screens" || mid.Steps[1].Status != generation.StepRunning || mid.Steps[0].FinishedAt == nil {
		t.Fatalf("mid = %+v, %v", mid, err)
	}

	steps[1].Status = generation.StepFailed
	end := t0.Add(2 * time.Second)
	if err := f.s.Finish(ctx, g.ID, generation.StatusFailed, generation.CodeNotAvailable, steps, end); err != nil {
		t.Fatal(err)
	}
	fin, _ := f.s.Get(ctx, f.userA, f.projectA, g.ID)
	if fin.Status != generation.StatusFailed || fin.ErrorCode != generation.CodeNotAvailable || fin.CompletedAt == nil {
		t.Fatalf("final = %+v", fin)
	}
	if err := f.s.SaveProgress(ctx, g.ID, steps[:1], end.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if again, _ := f.s.Get(ctx, f.userA, f.projectA, g.ID); len(again.Steps) != 2 {
		t.Fatal("a finished job must not be changed by a late progress write")
	}

	if _, err := f.s.Get(ctx, f.userB, f.projectA, g.ID); !errors.Is(err, generation.ErrNotFound) {
		t.Fatalf("stranger: %v", err)
	}
	if _, err := f.s.Get(ctx, f.userA, f.projectB, g.ID); !errors.Is(err, generation.ErrNotFound) {
		t.Fatalf("wrong project: %v", err)
	}
	if _, err := f.s.Create(ctx, f.userA, f.projectA, "openai", nil, plan, end); err != nil {
		t.Fatalf("a new job after one finished: %v", err)
	}
}

func TestChosenScreensRoundTripAndNullMeansAll(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	chosen, err := f.s.Create(ctx, f.userA, f.projectA, "anthropic", []string{"screen_a", "screen_b"}, plan, t0)
	if err != nil || len(chosen.ScreenIDs) != 2 {
		t.Fatalf("create = %+v, %v", chosen, err)
	}
	got, _ := f.s.Get(ctx, f.userA, f.projectA, chosen.ID)
	if len(got.ScreenIDs) != 2 || got.ScreenIDs[1] != "screen_b" {
		t.Fatalf("stored = %+v", got.ScreenIDs)
	}
	everything, _ := f.s.Create(ctx, f.userB, f.projectB, "xai", nil, plan, t0)
	back, _ := f.s.Get(ctx, f.userB, f.projectB, everything.ID)
	if back.ScreenIDs != nil {
		t.Fatalf("all screens must come back as nil, got %v", back.ScreenIDs)
	}
}

func TestFailAbandonedRetiresOnlyOldRunningJobs(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	g, _ := f.s.Create(ctx, f.userA, f.projectA, "anthropic", nil, plan, t0)

	if n, _ := f.s.FailAbandoned(ctx, t0.Add(-time.Hour), generation.CodeInterrupted, t0); n != 0 {
		t.Fatalf("a fresh job was retired")
	}
	n, err := f.s.FailAbandoned(ctx, t0.Add(time.Hour), generation.CodeInterrupted, t0.Add(2*time.Hour))
	if err != nil || n != 1 {
		t.Fatalf("retired = %d, %v", n, err)
	}
	got, _ := f.s.Get(ctx, f.userA, f.projectA, g.ID)
	if got.Status != generation.StatusFailed || got.ErrorCode != generation.CodeInterrupted {
		t.Fatalf("got = %+v", got)
	}
}

func TestDeletingAProjectRemovesItsJobs(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	_, _ = f.s.Create(ctx, f.userA, f.projectA, "anthropic", nil, plan, t0)
	_, _ = f.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, f.projectA)
	var n int
	_ = f.pool.QueryRow(ctx, `SELECT count(*) FROM generations`).Scan(&n)
	if n != 0 {
		t.Fatalf("generations left: %d", n)
	}
}
