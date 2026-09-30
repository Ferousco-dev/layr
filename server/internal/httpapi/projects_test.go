package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/ferousco-dev/layr/server/internal/observability"
	"github.com/ferousco-dev/layr/server/internal/project"
	projectstore "github.com/ferousco-dev/layr/server/internal/project/pgstore"
	"github.com/jackc/pgx/v5/pgxpool"
)

// tokenAuth maps cookie values to users so tests can act as several people.
type tokenAuth struct {
	fakeAuth
	users map[string]auth.User
}

func (t *tokenAuth) Authenticate(_ context.Context, token string) (auth.User, error) {
	if u, ok := t.users[token]; ok {
		return u, nil
	}
	return auth.User{}, auth.ErrSessionInvalid
}

// memRepo mimics the owner-scoped SQL contract in memory.
type memRepo struct {
	rows    []project.Project
	deleted map[string]bool
	seq     int
}

func (m *memRepo) Create(_ context.Context, owner, name string, now time.Time) (project.Project, error) {
	m.seq++
	p := project.Project{ID: fmt.Sprintf("00000000-0000-4000-8000-%012d", m.seq), UserID: owner, Name: name, CreatedAt: now, UpdatedAt: now}
	m.rows = append([]project.Project{p}, m.rows...)
	return p, nil
}

func (m *memRepo) find(owner, id string) int {
	for i, p := range m.rows {
		if p.ID == id && p.UserID == owner && !m.deleted[id] {
			return i
		}
	}
	return -1
}

func (m *memRepo) Get(_ context.Context, owner, id string) (project.Project, error) {
	if i := m.find(owner, id); i >= 0 {
		return m.rows[i], nil
	}
	return project.Project{}, project.ErrNotFound
}

func (m *memRepo) List(_ context.Context, owner string, _ *project.Cursor, limit int) ([]project.Project, error) {
	var out []project.Project
	for _, p := range m.rows {
		if p.UserID == owner && !m.deleted[p.ID] && len(out) < limit {
			out = append(out, p)
		}
	}
	return out, nil
}

func (m *memRepo) Rename(_ context.Context, owner, id, name string, now time.Time) (project.Project, error) {
	i := m.find(owner, id)
	if i < 0 {
		return project.Project{}, project.ErrNotFound
	}
	m.rows[i].Name, m.rows[i].UpdatedAt = name, now
	return m.rows[i], nil
}

func (m *memRepo) Delete(_ context.Context, owner, id string, _ time.Time) error {
	if m.find(owner, id) < 0 {
		return project.ErrNotFound
	}
	if m.deleted == nil {
		m.deleted = map[string]bool{}
	}
	m.deleted[id] = true
	return nil
}

func (m *memRepo) Restore(_ context.Context, owner, id string) (project.Project, error) {
	for _, p := range m.rows {
		if p.ID == id && p.UserID == owner && m.deleted[id] {
			delete(m.deleted, id)
			return p, nil
		}
	}
	return project.Project{}, project.ErrNotFound
}

func (m *memRepo) PurgeExpired(context.Context, time.Time) error { return nil }

var (
	userA = auth.User{ID: "aaaaaaaa-0000-4000-8000-000000000001", DisplayName: "A"}
	userB = auth.User{ID: "bbbbbbbb-0000-4000-8000-000000000002", DisplayName: "B"}
)

func projectHandler(repo project.Repository, users map[string]auth.User) http.Handler {
	return NewHandler(fakeHealth{}, observability.New(&bytes.Buffer{}, slog.LevelInfo), Options{
		FrontendOrigin: frontend,
		MaxBodyBytes:   1024,
		Auth: &AuthOptions{
			Service:    &tokenAuth{users: users},
			Projects:   project.NewService(repo),
			Limiter:    &fakeLimiter{},
			CookieName: "layr_session",
		},
	})
}

func send(h http.Handler, method, path, token, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		r.AddCookie(&http.Cookie{Name: "layr_session", Value: token})
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func decodeData(t *testing.T, w *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return body.Data
}

func errorCode(w *httptest.ResponseRecorder) string {
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Error.Code
}

const someID = "00000000-0000-4000-8000-000000000001"

func TestProjectRoutesRequireAuthentication(t *testing.T) {
	h := projectHandler(&memRepo{}, map[string]auth.User{"a": userA})

	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/v1/projects"}, {"GET", "/api/v1/projects"}, {"GET", "/api/v1/projects/" + someID},
		{"PATCH", "/api/v1/projects/" + someID}, {"DELETE", "/api/v1/projects/" + someID},
		{"POST", "/api/v1/projects/" + someID + "/restore"},
	} {
		none := send(h, tc.method, tc.path, "", `{"name":"x"}`)
		bad := send(h, tc.method, tc.path, "forged", `{"name":"x"}`)
		if none.Code != 401 || errorCode(none) != "AUTH_REQUIRED" || bad.Code != 401 || errorCode(bad) != "INVALID_SESSION" {
			t.Fatalf("%s %s: %d %d", tc.method, tc.path, none.Code, bad.Code)
		}
	}
}

func TestProjectCRUDFlow(t *testing.T) {
	h := projectHandler(&memRepo{}, map[string]auth.User{"a": userA})

	created := send(h, "POST", "/api/v1/projects", "a", `{"name":"  Landing Page  "}`)
	data := decodeData(t, created)
	id, _ := data["id"].(string)
	if created.Code != 201 || data["name"] != "Landing Page" || id == "" {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	if _, leaked := data["user_id"]; leaked {
		t.Fatal("response exposes user_id")
	}

	if got := send(h, "GET", "/api/v1/projects/"+id, "a", ""); got.Code != 200 || decodeData(t, got)["name"] != "Landing Page" {
		t.Fatalf("get: %d %s", got.Code, got.Body.String())
	}

	list := send(h, "GET", "/api/v1/projects?limit=5", "a", "")
	var listed struct {
		Data       []map[string]any `json:"data"`
		NextCursor any              `json:"next_cursor"`
	}
	_ = json.Unmarshal(list.Body.Bytes(), &listed)
	if list.Code != 200 || len(listed.Data) != 1 || listed.NextCursor != nil {
		t.Fatalf("list: %d %s", list.Code, list.Body.String())
	}

	if up := send(h, "PATCH", "/api/v1/projects/"+id, "a", `{"name":"Renamed"}`); up.Code != 200 || decodeData(t, up)["name"] != "Renamed" {
		t.Fatalf("patch: %d %s", up.Code, up.Body.String())
	}

	if del := send(h, "DELETE", "/api/v1/projects/"+id, "a", ""); del.Code != 204 {
		t.Fatalf("delete: %d", del.Code)
	}
	if gone := send(h, "GET", "/api/v1/projects/"+id, "a", ""); gone.Code != 404 || errorCode(gone) != "PROJECT_NOT_FOUND" {
		t.Fatalf("after delete: %d %s", gone.Code, gone.Body.String())
	}

	if back := send(h, "POST", "/api/v1/projects/"+id+"/restore", "a", ""); back.Code != 200 || decodeData(t, back)["name"] != "Renamed" {
		t.Fatalf("restore: %d %s", back.Code, back.Body.String())
	}
	if again := send(h, "POST", "/api/v1/projects/"+id+"/restore", "a", ""); again.Code != 404 {
		t.Fatalf("restoring a live project: %d", again.Code)
	}
}

func TestProjectValidation(t *testing.T) {
	h := projectHandler(&memRepo{}, map[string]auth.User{"a": userA})
	created := decodeData(t, send(h, "POST", "/api/v1/projects", "a", `{"name":"ok"}`))
	id := created["id"].(string)

	cases := []struct {
		name, method, path, body string
		status                   int
		code                     string
	}{
		{"malformed json", "POST", "/api/v1/projects", `{"name":`, 400, "INVALID_REQUEST"},
		{"missing name", "POST", "/api/v1/projects", `{}`, 400, "INVALID_PROJECT_NAME"},
		{"empty name", "POST", "/api/v1/projects", `{"name":""}`, 400, "INVALID_PROJECT_NAME"},
		{"blank name", "POST", "/api/v1/projects", `{"name":"      "}`, 400, "INVALID_PROJECT_NAME"},
		{"long name", "POST", "/api/v1/projects", `{"name":"` + strings.Repeat("a", 121) + `"}`, 400, "INVALID_PROJECT_NAME"},
		{"mass assignment user_id", "POST", "/api/v1/projects", `{"name":"x","user_id":"` + userB.ID + `"}`, 400, "INVALID_REQUEST"},
		{"mass assignment id", "PATCH", "/api/v1/projects/" + id, `{"name":"x","id":"` + someID + `"}`, 400, "INVALID_REQUEST"},
		{"trailing json", "POST", "/api/v1/projects", `{"name":"x"}{"name":"y"}`, 400, "INVALID_REQUEST"},
		{"wrong type", "POST", "/api/v1/projects", `{"name":5}`, 400, "INVALID_REQUEST"},
		{"oversized body", "POST", "/api/v1/projects", `{"name":"` + strings.Repeat("a", 2000) + `"}`, 413, "REQUEST_TOO_LARGE"},
		{"invalid id get", "GET", "/api/v1/projects/not-a-uuid", ``, 400, "INVALID_PROJECT_ID"},
		{"invalid id patch", "PATCH", "/api/v1/projects/not-a-uuid", `{"name":"x"}`, 400, "INVALID_PROJECT_ID"},
		{"invalid id delete", "DELETE", "/api/v1/projects/not-a-uuid", ``, 400, "INVALID_PROJECT_ID"},
		{"empty patch", "PATCH", "/api/v1/projects/" + id, `{}`, 400, "INVALID_REQUEST"},
		{"blank patch name", "PATCH", "/api/v1/projects/" + id, `{"name":" "}`, 400, "INVALID_PROJECT_NAME"},
		{"limit zero", "GET", "/api/v1/projects?limit=0", ``, 400, "INVALID_REQUEST"},
		{"limit too big", "GET", "/api/v1/projects?limit=101", ``, 400, "INVALID_REQUEST"},
		{"limit text", "GET", "/api/v1/projects?limit=abc", ``, 400, "INVALID_REQUEST"},
		{"limit negative", "GET", "/api/v1/projects?limit=-3", ``, 400, "INVALID_REQUEST"},
		{"bad cursor", "GET", "/api/v1/projects?cursor=@@@", ``, 400, "INVALID_REQUEST"},
		{"method not allowed", "PUT", "/api/v1/projects", ``, 405, "METHOD_NOT_ALLOWED"},
	}
	for _, tc := range cases {
		w := send(h, tc.method, tc.path, "a", tc.body)
		if w.Code != tc.status || errorCode(w) != tc.code {
			t.Fatalf("%s: got %d %q, want %d %q (%s)", tc.name, w.Code, errorCode(w), tc.status, tc.code, w.Body.String())
		}
	}
}

func TestCrossUserAccessIsDeniedAsNotFound(t *testing.T) {
	h := projectHandler(&memRepo{}, map[string]auth.User{"a": userA, "b": userB})
	idA := decodeData(t, send(h, "POST", "/api/v1/projects", "a", `{"name":"A project"}`))["id"].(string)
	idB := decodeData(t, send(h, "POST", "/api/v1/projects", "b", `{"name":"B project"}`))["id"].(string)

	for _, tc := range []struct{ token, id string }{{"a", idB}, {"b", idA}} {
		for _, method := range []string{"GET", "PATCH", "DELETE", "POST"} {
			path := "/api/v1/projects/" + tc.id
			if method == "POST" {
				path += "/restore"
			}
			w := send(h, method, path, tc.token, `{"name":"hijack"}`)
			if w.Code != 404 || errorCode(w) != "PROJECT_NOT_FOUND" {
				t.Fatalf("%s %s as %s: %d %s", method, tc.id, tc.token, w.Code, w.Body.String())
			}
		}
	}

	missing := send(h, "GET", "/api/v1/projects/00000000-0000-4000-8000-0000000000ff", "a", "")
	foreign := send(h, "GET", "/api/v1/projects/"+idB, "a", "")
	if missing.Body.String() == "" || strings.ReplaceAll(missing.Body.String(), decodeReqID(missing), "") != strings.ReplaceAll(foreign.Body.String(), decodeReqID(foreign), "") {
		t.Fatalf("foreign and missing responses differ:\n%s\n%s", missing.Body.String(), foreign.Body.String())
	}

	if untouched := send(h, "GET", "/api/v1/projects/"+idA, "a", ""); decodeData(t, untouched)["name"] != "A project" {
		t.Fatal("project A was altered by user B")
	}
	if stillThere := send(h, "GET", "/api/v1/projects/"+idB, "b", ""); stillThere.Code != 200 {
		t.Fatal("project B was deleted by user A")
	}

	for token, want := range map[string]string{"a": "A project", "b": "B project"} {
		list := send(h, "GET", "/api/v1/projects", token, "")
		var body struct {
			Data []map[string]any `json:"data"`
		}
		_ = json.Unmarshal(list.Body.Bytes(), &body)
		if len(body.Data) != 1 || body.Data[0]["name"] != want {
			t.Fatalf("list for %s: %s", token, list.Body.String())
		}
	}
}

func decodeReqID(w *httptest.ResponseRecorder) string {
	var body struct {
		Error struct {
			RequestID string `json:"request_id"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &body)
	return body.Error.RequestID
}

// TestLiveCrossUserIsolation runs the same denial checks over real PostgreSQL.
func TestLiveCrossUserIsolation(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, _ = pool.Exec(ctx, `TRUNCATE users CASCADE`)

	users := map[string]auth.User{}
	for token, fig := range map[string]string{"a": "live-a", "b": "live-b"} {
		var id string
		if err := pool.QueryRow(ctx, `INSERT INTO users (figma_user_id, display_name, created_at, updated_at) VALUES ($1,'n',now(),now()) RETURNING id`, fig).Scan(&id); err != nil {
			t.Fatal(err)
		}
		users[token] = auth.User{ID: id}
	}
	h := projectHandler(projectstore.New(pool), users)

	idA := decodeData(t, send(h, "POST", "/api/v1/projects", "a", `{"name":"Live A"}`))["id"].(string)
	idB := decodeData(t, send(h, "POST", "/api/v1/projects", "b", `{"name":"Live B"}`))["id"].(string)

	for _, tc := range []struct{ token, id string }{{"a", idB}, {"b", idA}} {
		for _, method := range []string{"GET", "PATCH", "DELETE"} {
			if w := send(h, method, "/api/v1/projects/"+tc.id, tc.token, `{"name":"hijack"}`); w.Code != 404 {
				t.Fatalf("%s as %s: %d", method, tc.token, w.Code)
			}
		}
	}
	var rows int
	_ = pool.QueryRow(ctx, `SELECT count(*) FROM projects WHERE name IN ('Live A','Live B')`).Scan(&rows)
	if rows != 2 {
		t.Fatalf("projects altered or deleted across users: %d rows", rows)
	}
}

func TestOtherUserCannotRestoreDeletedProject(t *testing.T) {
	h := projectHandler(&memRepo{}, map[string]auth.User{"a": userA, "b": userB})
	id := decodeData(t, send(h, "POST", "/api/v1/projects", "a", `{"name":"mine"}`))["id"].(string)
	send(h, "DELETE", "/api/v1/projects/"+id, "a", "")

	if w := send(h, "POST", "/api/v1/projects/"+id+"/restore", "b", ""); w.Code != 404 {
		t.Fatalf("foreign restore: %d", w.Code)
	}
	if w := send(h, "POST", "/api/v1/projects/"+id+"/restore", "a", ""); w.Code != 200 {
		t.Fatalf("owner restore: %d", w.Code)
	}
}

func TestProjectAPIRateLimits(t *testing.T) {
	limiter := &fakeLimiter{}
	h := NewHandler(fakeHealth{}, observability.New(&bytes.Buffer{}, slog.LevelInfo), Options{
		FrontendOrigin: frontend, MaxBodyBytes: 1024,
		Auth: &AuthOptions{Service: &tokenAuth{users: map[string]auth.User{"a": userA}}, Projects: project.NewService(&memRepo{}), Limiter: limiter, CookieName: "layr_session"},
	})

	for i := 1; i <= apiWriteLimit; i++ {
		if w := send(h, "POST", "/api/v1/projects", "a", `{"name":"p"}`); w.Code != 201 {
			t.Fatalf("write %d: %d", i, w.Code)
		}
	}
	over := send(h, "POST", "/api/v1/projects", "a", `{"name":"p"}`)
	if over.Code != http.StatusTooManyRequests || errorCode(over) != "RATE_LIMITED" || over.Header().Get("Retry-After") == "" {
		t.Fatalf("write over limit: %d %s", over.Code, over.Body.String())
	}
	if read := send(h, "GET", "/api/v1/projects", "a", ""); read.Code != 200 {
		t.Fatalf("reads must not share the write budget: %d", read.Code)
	}

	flooded := &fakeLimiter{n: apiIPLimit}
	h2 := NewHandler(fakeHealth{}, observability.New(&bytes.Buffer{}, slog.LevelInfo), Options{
		FrontendOrigin: frontend, MaxBodyBytes: 1024,
		Auth: &AuthOptions{Service: &tokenAuth{}, Projects: project.NewService(&memRepo{}), Limiter: flooded, CookieName: "layr_session"},
	})
	if w := send(h2, "GET", "/api/v1/me", "", ""); w.Code != http.StatusTooManyRequests {
		t.Fatalf("per-address limit not applied before auth: %d", w.Code)
	}
}

func TestRateLimitsAreConfigurable(t *testing.T) {
	h := NewHandler(fakeHealth{}, observability.New(&bytes.Buffer{}, slog.LevelInfo), Options{
		FrontendOrigin: frontend, MaxBodyBytes: 1024,
		Auth: &AuthOptions{
			Service: &tokenAuth{users: map[string]auth.User{"a": userA}}, Projects: project.NewService(&memRepo{}),
			Limiter: &fakeLimiter{}, CookieName: "layr_session",
			Limits: Limits{Login: 1, API: 1000, Write: 2},
		},
	})

	for i := 0; i < 2; i++ {
		if w := send(h, "POST", "/api/v1/projects", "a", `{"name":"p"}`); w.Code != 201 {
			t.Fatalf("write %d: %d", i, w.Code)
		}
	}
	if w := send(h, "POST", "/api/v1/projects", "a", `{"name":"p"}`); w.Code != http.StatusTooManyRequests {
		t.Fatalf("third write: %d", w.Code)
	}
	if first := send(h, "GET", "/auth/figma", "", ""); first.Code != http.StatusFound {
		t.Fatalf("first login request: %d", first.Code)
	}
	if second := send(h, "GET", "/auth/figma", "", ""); second.Code != http.StatusTooManyRequests {
		t.Fatalf("second login request: %d", second.Code)
	}
}
