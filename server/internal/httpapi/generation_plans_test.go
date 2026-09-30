package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/ferousco-dev/layr/server/internal/designapi/designtest"
	"github.com/ferousco-dev/layr/server/internal/genplan"
	"github.com/ferousco-dev/layr/server/internal/observability"
)

// memPlans mimics the owner-scoped SQL contract of the plan store.
type memPlans struct {
	mu     sync.Mutex
	owners map[string]string
	rows   map[string]genplan.Record
	n      int
}

func (m *memPlans) Create(_ context.Context, owner, projectID, importID string, p *genplan.Plan, _ []byte, now time.Time) (genplan.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.owners[projectID] != owner {
		return genplan.Record{}, genplan.ErrNotFound
	}
	m.n++
	stored := *p
	stored.ID, stored.ProjectID = fmt.Sprintf("00000000-0000-4000-8000-%012d", m.n), projectID
	rec := genplan.Record{Plan: &stored, Status: genplan.StatusPlanned, ImportID: importID, CreatedAt: now}
	m.rows[stored.ID] = rec
	return rec, nil
}

func (m *memPlans) Get(_ context.Context, owner, projectID, id string) (genplan.Record, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec, ok := m.rows[id]
	if !ok || m.owners[projectID] != owner || rec.Plan.ProjectID != projectID {
		return genplan.Record{}, genplan.ErrNotFound
	}
	return rec, nil
}

func planHandler(e *designtest.Env) http.Handler {
	store := &memPlans{owners: e.Projects.Owners, rows: map[string]genplan.Record{}}
	svc := genplan.NewService(e.Service(100), store, genplan.Limits{}, nil)
	return NewHandler(fakeHealth{}, observability.New(&bytes.Buffer{}, slog.LevelInfo), Options{
		FrontendOrigin: frontend, MaxBodyBytes: 1 << 20,
		Auth: &AuthOptions{
			Service: &tokenAuth{users: map[string]auth.User{"a": {ID: designtest.OwnerA}, "b": {ID: designtest.OwnerB}}},
			Design:  e.Service(100), Plans: svc, Limiter: &fakeLimiter{}, CookieName: "layr_session",
		},
	})
}

func plansPath(project string) string { return "/api/v1/projects/" + project + "/generation-plans" }

func designVersion(t *testing.T, h http.Handler) string {
	t.Helper()
	w := send(h, "GET", base(), "a", "")
	v, _ := decodeData(t, w)["design_version"].(string)
	if v == "" {
		t.Fatalf("no design version: %s", w.Body.String())
	}
	return v
}

func planBody(version, selection string) string {
	return fmt.Sprintf(`{"design_version":%q,"selection":%s,"target":{"framework":"nextjs","language":"typescript"}}`, version, selection)
}

func TestPlanRoutesRequireAuthenticationAndAllowedMethods(t *testing.T) {
	h := planHandler(designtest.New(t))
	paths := []string{plansPath(designtest.ProjectA), plansPath(designtest.ProjectA) + "/00000000-0000-4000-8000-000000000001"}
	for _, p := range paths {
		if w := send(h, "GET", p, "", ""); w.Code != 401 {
			t.Errorf("%s without a session: %d", p, w.Code)
		}
		if w := send(h, "POST", p, "forged", `{}`); w.Code != 401 {
			t.Errorf("%s with a forged session: %d", p, w.Code)
		}
	}
	for _, m := range []string{"PATCH", "PUT", "DELETE"} {
		for _, p := range paths {
			if w := send(h, m, p, "a", `{}`); w.Code != http.StatusMethodNotAllowed {
				t.Errorf("%s %s: %d", m, p, w.Code)
			}
		}
	}
}

func TestPlanSelectionModesOverHTTP(t *testing.T) {
	e := designtest.New(t)
	h := planHandler(e)
	v := designVersion(t, h)
	cases := []struct {
		name, selection string
		screens         int
	}{
		{"one", fmt.Sprintf(`{"mode":"one","screen_id":%q}`, designtest.ScreenID(5)), 1},
		{"selected", fmt.Sprintf(`{"mode":"selected","screen_ids":[%q,%q,%q]}`, designtest.ScreenID(3), designtest.ScreenID(1), designtest.ScreenID(3)), 2},
		{"flow", fmt.Sprintf(`{"mode":"flow","flow_id":%q}`, designtest.SectionID(1)), 3},
		{"all", `{"mode":"all"}`, 7},
	}
	for _, c := range cases {
		w := send(h, "POST", plansPath(designtest.ProjectA), "a", planBody(v, c.selection))
		if w.Code != http.StatusCreated {
			t.Fatalf("%s: %d %s", c.name, w.Code, w.Body.String())
		}
		data := decodeData(t, w)
		summary := data["summary"].(map[string]any)
		if int(summary["screens"].(float64)) != c.screens || data["design_version"] != v || data["status"] != "planned" {
			t.Fatalf("%s: data = %v", c.name, data)
		}
		if int(summary["shared_components"].(float64)) != 1 {
			t.Fatalf("%s: the shared Button must appear once: %v", c.name, summary)
		}
		if data["fingerprint"] == "" || len(data["stages"].([]any)) < 4 {
			t.Fatalf("%s: %v", c.name, data)
		}

		got := send(h, "GET", plansPath(designtest.ProjectA)+"/"+data["id"].(string), "a", "")
		if got.Code != 200 || decodeData(t, got)["fingerprint"] != data["fingerprint"] {
			t.Fatalf("%s: get %d %s", c.name, got.Code, got.Body.String())
		}
		for _, leak := range []string{e.Root, "/tmp", "reference/", "assets/", "https://", "FILEKEY123456", "\"path\""} {
			if strings.Contains(got.Body.String(), leak) {
				t.Fatalf("%s: plan leaks %q", c.name, leak)
			}
		}
	}
}

func TestPlanRejectionsOverHTTP(t *testing.T) {
	h := planHandler(designtest.New(t))
	v := designVersion(t, h)
	one := fmt.Sprintf(`{"mode":"one","screen_id":%q}`, designtest.ScreenID(1))
	cases := []struct {
		name, body string
		status     int
		code       string
	}{
		{"stale version", planBody("dv_0000000000000000", one), 409, "DESIGN_VERSION_UNAVAILABLE"},
		{"bad version", planBody("nope", one), 400, "INVALID_DESIGN_VERSION"},
		{"missing screen", planBody(v, `{"mode":"one","screen_id":"screen_ffffffffffffffff"}`), 404, "SCREEN_NOT_FOUND"},
		{"missing flow", planBody(v, `{"mode":"flow","flow_id":"section_ffffffffffffffff"}`), 404, "FLOW_NOT_FOUND"},
		{"empty selected", planBody(v, `{"mode":"selected","screen_ids":[]}`), 400, "EMPTY_SELECTION"},
		{"malformed id", planBody(v, `{"mode":"one","screen_id":"../../etc"}`), 400, "INVALID_SELECTION"},
		{"no selection", fmt.Sprintf(`{"design_version":%q}`, v), 400, "INVALID_SELECTION"},
		{"unsupported target", fmt.Sprintf(`{"design_version":%q,"selection":%s,"target":{"framework":"vue","language":"typescript"}}`, v, one), 400, "GENERATION_TARGET_UNSUPPORTED"},
		{"client edges", fmt.Sprintf(`{"design_version":%q,"selection":%s,"dependencies":[{"unit":"a","depends_on":"b"}]}`, v, one), 400, "INVALID_REQUEST"},
		{"client units", fmt.Sprintf(`{"design_version":%q,"selection":%s,"units":[]}`, v, one), 400, "INVALID_REQUEST"},
		{"client user", fmt.Sprintf(`{"design_version":%q,"selection":%s,"user_id":"x"}`, v, one), 400, "INVALID_REQUEST"},
	}
	for _, c := range cases {
		w := send(h, "POST", plansPath(designtest.ProjectA), "a", c.body)
		if w.Code != c.status || errorCode(w) != c.code {
			t.Errorf("%s: %d %s, want %d %s", c.name, w.Code, errorCode(w), c.status, c.code)
		}
	}
}

func TestPlansOfOneUserAreInvisibleToAnother(t *testing.T) {
	e := designtest.New(t)
	h := planHandler(e)
	v := designVersion(t, h)
	all := planBody(v, `{"mode":"all"}`)
	planA := decodeData(t, send(h, "POST", plansPath(designtest.ProjectA), "a", all))["id"].(string)

	if w := send(h, "POST", plansPath(designtest.ProjectA), "b", all); w.Code != 404 || errorCode(w) != "PROJECT_NOT_FOUND" {
		t.Fatalf("create in a stranger's project: %d %s", w.Code, w.Body.String())
	}
	if w := send(h, "GET", plansPath(designtest.ProjectA)+"/"+planA, "b", ""); w.Code != 404 || errorCode(w) != "GENERATION_PLAN_NOT_FOUND" {
		t.Fatalf("read a stranger's plan through its project: %d %s", w.Code, w.Body.String())
	}
	if w := send(h, "GET", plansPath(designtest.ProjectB)+"/"+planA, "b", ""); w.Code != 404 || errorCode(w) != "GENERATION_PLAN_NOT_FOUND" {
		t.Fatalf("read a stranger's plan through own project: %d %s", w.Code, w.Body.String())
	}
	// Project B has no design: user A's design version and screens cannot be smuggled into it.
	one := fmt.Sprintf(`{"mode":"one","screen_id":%q}`, designtest.ScreenID(1))
	if w := send(h, "POST", plansPath(designtest.ProjectB), "b", planBody(v, one)); w.Code != 409 || errorCode(w) != "DESIGN_NOT_READY" {
		t.Fatalf("design of another project: %d %s", w.Code, w.Body.String())
	}
	if w := send(h, "POST", plansPath(designtest.ProjectA), "a", planBody(v, one)); w.Code != 201 {
		t.Fatalf("the owner is unaffected: %d", w.Code)
	}
	if w := send(h, "GET", plansPath(designtest.ProjectA)+"/not-a-uuid", "a", ""); w.Code != 404 {
		t.Fatalf("malformed plan id: %d", w.Code)
	}
}

func TestPlanCannotTargetAnExpiredDesign(t *testing.T) {
	e := designtest.New(t)
	h := planHandler(e)
	v := designVersion(t, h)
	if err := e.Manager.Cleanup(designtest.ImportID); err != nil {
		t.Fatal(err)
	}
	w := send(h, "POST", plansPath(designtest.ProjectA), "a", planBody(v, `{"mode":"all"}`))
	if w.Code != 409 || errorCode(w) != "DESIGN_VERSION_UNAVAILABLE" {
		t.Fatalf("expired design: %d %s", w.Code, w.Body.String())
	}
}
