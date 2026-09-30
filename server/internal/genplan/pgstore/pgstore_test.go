package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/genplan"
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
	for fig, dst := range map[string]*string{"plan-a": &f.userA, "plan-b": &f.userB} {
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

const importID = "cccccccc-0000-4000-8000-000000000001"

func samplePlan() (*genplan.Plan, []byte) {
	p := &genplan.Plan{
		SchemaVersion: 1, DesignVersion: "dv_0123456789abcdef", Fingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		Selection: genplan.PlanSelection{Mode: genplan.ModeAll, ScreenIDs: []string{"screen_1"}},
		Target:    genplan.Target{Framework: "nextjs", Language: "typescript"},
		Units:     []genplan.Unit{{ID: "foundation", Type: genplan.UnitFoundation, Name: "Project foundation"}}, Dependencies: []genplan.Edge{},
		Order: []string{"foundation"}, Stages: [][]string{{"foundation"}}, Assets: []genplan.AssetRef{}, Warnings: []genplan.Warning{},
	}
	raw, _ := json.Marshal(p)
	return p, raw
}

func TestCreateAndGetCheckOwnership(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	plan, raw := samplePlan()

	if _, err := f.s.Create(ctx, f.userB, f.projectA, importID, plan, raw, t0); !errors.Is(err, genplan.ErrNotFound) {
		t.Fatalf("a stranger's project: %v", err)
	}
	rec, err := f.s.Create(ctx, f.userA, f.projectA, importID, plan, raw, t0)
	if err != nil || rec.Plan.ID == "" || rec.Plan.ProjectID != f.projectA || rec.Status != genplan.StatusPlanned {
		t.Fatalf("create: %+v %v", rec, err)
	}
	got, err := f.s.Get(ctx, f.userA, f.projectA, rec.Plan.ID)
	if err != nil || got.Plan.Fingerprint != plan.Fingerprint || got.Plan.ID != rec.Plan.ID || got.ImportID != importID {
		t.Fatalf("get: %+v %v", got, err)
	}
	for name, args := range map[string][3]string{
		"other user":    {f.userB, f.projectA, rec.Plan.ID},
		"other project": {f.userB, f.projectB, rec.Plan.ID},
		"wrong project": {f.userA, f.projectB, rec.Plan.ID},
	} {
		if _, err := f.s.Get(ctx, args[0], args[1], args[2]); !errors.Is(err, genplan.ErrNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestIdenticalPlansAreSeparateImmutableRecords(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	plan, raw := samplePlan()
	a, _ := f.s.Create(ctx, f.userA, f.projectA, importID, plan, raw, t0)
	b, _ := f.s.Create(ctx, f.userA, f.projectA, importID, plan, raw, t0.Add(time.Second))
	if a.Plan.ID == b.Plan.ID {
		t.Fatal("two requests must create two records")
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM generation_plans WHERE fingerprint = $1`, plan.Fingerprint).Scan(&n); err != nil || n != 2 {
		t.Fatalf("records sharing a fingerprint = %d, %v", n, err)
	}
}

func TestPlansGoWithTheirProjectAndRejectBadRows(t *testing.T) {
	f := live(t)
	ctx := context.Background()
	plan, raw := samplePlan()
	rec, _ := f.s.Create(ctx, f.userA, f.projectA, importID, plan, raw, t0)

	if _, err := f.pool.Exec(ctx, `UPDATE projects SET deleted_at = now() WHERE id = $1`, f.projectA); err != nil {
		t.Fatal(err)
	}
	if _, err := f.s.Get(ctx, f.userA, f.projectA, rec.Plan.ID); !errors.Is(err, genplan.ErrNotFound) {
		t.Fatalf("a deleted project's plan: %v", err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE generation_plans SET fingerprint = 'short' WHERE id = $1`, rec.Plan.ID); err == nil {
		t.Fatal("a malformed fingerprint was accepted")
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM projects WHERE id = $1`, f.projectA); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM generation_plans`).Scan(&n); err != nil || n != 0 {
		t.Fatalf("plans left after the project was deleted: %d %v", n, err)
	}
}
