package pgstore

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/project"
	"github.com/jackc/pgx/v5/pgxpool"
)

var t0 = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

func liveStore(t *testing.T) (*Store, *pgxpool.Pool, string, string) {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx := context.Background()
	if _, err := pool.Exec(ctx, `TRUNCATE users CASCADE`); err != nil {
		t.Fatal(err)
	}
	var a, b string
	insert := `INSERT INTO users (figma_user_id, display_name, created_at, updated_at) VALUES ($1, 'n', now(), now()) RETURNING id`
	if err := pool.QueryRow(ctx, insert, "fig-a").Scan(&a); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, insert, "fig-b").Scan(&b); err != nil {
		t.Fatal(err)
	}
	return New(pool), pool, a, b
}

func TestOwnerScopedCRUD(t *testing.T) {
	s, _, a, b := liveStore(t)
	ctx := context.Background()

	created, err := s.Create(ctx, a, "Landing Page", t0)
	if err != nil || created.UserID != a || created.Name != "Landing Page" || !created.CreatedAt.Equal(t0) {
		t.Fatalf("created = %#v, err = %v", created, err)
	}
	if got, err := s.Get(ctx, a, created.ID); err != nil || got.ID != created.ID {
		t.Fatalf("owner get: %#v %v", got, err)
	}

	if _, err := s.Get(ctx, b, created.ID); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("foreign get: %v", err)
	}
	if _, err := s.Rename(ctx, b, created.ID, "Hijacked", t0); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("foreign rename: %v", err)
	}
	if err := s.Delete(ctx, b, created.ID, t0); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("foreign delete: %v", err)
	}
	if got, _ := s.Get(ctx, a, created.ID); got.Name != "Landing Page" {
		t.Fatalf("project changed by non-owner: %#v", got)
	}

	renamed, err := s.Rename(ctx, a, created.ID, "Renamed", t0.Add(time.Minute))
	if err != nil || renamed.Name != "Renamed" || !renamed.UpdatedAt.Equal(t0.Add(time.Minute)) || !renamed.CreatedAt.Equal(t0) {
		t.Fatalf("renamed = %#v, err = %v", renamed, err)
	}

	if err := s.Delete(ctx, a, created.ID, t0); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(ctx, a, created.ID, t0); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("second delete: %v", err)
	}
	if _, err := s.Get(ctx, a, "00000000-0000-4000-8000-000000000000"); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("unknown get: %v", err)
	}
}

func TestListIsOwnerScopedOrderedAndPaged(t *testing.T) {
	s, _, a, b := liveStore(t)
	ctx := context.Background()
	for i, name := range []string{"one", "two", "three", "four", "five"} {
		if _, err := s.Create(ctx, a, name, t0.Add(time.Duration(i)*time.Second)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.Create(ctx, b, "not-mine", t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}

	first, err := s.List(ctx, a, nil, 2)
	if err != nil || len(first) != 2 || first[0].Name != "five" || first[1].Name != "four" {
		t.Fatalf("first = %#v, err = %v", first, err)
	}
	next, _ := s.List(ctx, a, &project.Cursor{UpdatedAt: first[1].UpdatedAt, ID: first[1].ID}, 10)
	names := []string{}
	for _, p := range next {
		names = append(names, p.Name)
	}
	if len(names) != 3 || names[0] != "three" || names[2] != "one" {
		t.Fatalf("next = %v", names)
	}
	for _, p := range append(first, next...) {
		if p.UserID != a {
			t.Fatal("list leaked another user's project")
		}
	}
}

func TestListBreaksTimestampTiesByID(t *testing.T) {
	s, _, a, _ := liveStore(t)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		_, _ = s.Create(ctx, a, "same", t0)
	}

	page1, _ := s.List(ctx, a, nil, 2)
	page2, _ := s.List(ctx, a, &project.Cursor{UpdatedAt: page1[1].UpdatedAt, ID: page1[1].ID}, 10)

	seen := map[string]bool{}
	for _, p := range append(page1, page2...) {
		seen[p.ID] = true
	}
	if len(page2) != 2 || len(seen) != 4 {
		t.Fatalf("page2 %d unique %d", len(page2), len(seen))
	}
}

func TestConcurrentCreates(t *testing.T) {
	s, pool, a, _ := liveStore(t)
	ctx := context.Background()

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Create(ctx, a, "parallel", t0); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()

	var n, distinct int
	_ = pool.QueryRow(ctx, `SELECT count(*), count(DISTINCT id) FROM projects WHERE user_id = $1`, a).Scan(&n, &distinct)
	if n != 20 || distinct != 20 {
		t.Fatalf("rows %d distinct %d", n, distinct)
	}
}

func TestDeletingUserCascadesToProjects(t *testing.T) {
	s, pool, a, _ := liveStore(t)
	ctx := context.Background()
	_, _ = s.Create(ctx, a, "gone", t0)

	_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, a)

	var n int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM projects`).Scan(&n)
	if n != 0 {
		t.Fatalf("orphaned projects: %d", n)
	}
}

func TestDatabaseRejectsBlankAndOversizedNames(t *testing.T) {
	s, _, a, _ := liveStore(t)
	ctx := context.Background()

	if _, err := s.Create(ctx, a, "", t0); err == nil {
		t.Fatal("empty name accepted by database")
	}
	long := make([]rune, 121)
	for i := range long {
		long[i] = 'x'
	}
	if _, err := s.Create(ctx, a, string(long), t0); err == nil {
		t.Fatal("oversized name accepted by database")
	}
}

func TestSoftDeleteHidesRestoresAndPurges(t *testing.T) {
	s, pool, a, b := liveStore(t)
	ctx := context.Background()
	p, _ := s.Create(ctx, a, "Precious", t0)

	if err := s.Delete(ctx, a, p.ID, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, a, p.ID); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("deleted project still readable: %v", err)
	}
	if _, err := s.Rename(ctx, a, p.ID, "x", t0); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("deleted project renamable: %v", err)
	}
	if list, _ := s.List(ctx, a, nil, 10); len(list) != 0 {
		t.Fatalf("deleted project listed: %d", len(list))
	}

	if _, err := s.Restore(ctx, b, p.ID); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("foreign restore: %v", err)
	}
	restored, err := s.Restore(ctx, a, p.ID)
	if err != nil || restored.Name != "Precious" || !restored.UpdatedAt.Equal(t0) {
		t.Fatalf("restored = %#v, err = %v", restored, err)
	}
	if _, err := s.Restore(ctx, a, p.ID); !errors.Is(err, project.ErrNotFound) {
		t.Fatalf("restoring a live project: %v", err)
	}

	old, _ := s.Create(ctx, a, "old", t0)
	recent, _ := s.Create(ctx, a, "recent", t0)
	_ = s.Delete(ctx, a, old.ID, t0)
	_ = s.Delete(ctx, a, recent.ID, t0.Add(48*time.Hour))
	if err := s.PurgeExpired(ctx, t0.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var gone, kept int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM projects WHERE id = $1`, old.ID).Scan(&gone)
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM projects WHERE id = $1`, recent.ID).Scan(&kept)
	if gone != 0 || kept != 1 {
		t.Fatalf("purge removed %d expired, kept %d recent", 1-gone, kept)
	}
}
