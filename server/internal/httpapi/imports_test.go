package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/imports"
	importstore "github.com/ferousco-dev/layr/server/internal/imports/pgstore"
	"github.com/ferousco-dev/layr/server/internal/observability"
	"github.com/ferousco-dev/layr/server/internal/workspace"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type fakeImports struct {
	imp      imports.Import
	err      error
	lastUser string
	lastURL  string
	lastSel  imports.Selection
	starts   int
	wait     time.Duration
}

func (f *fakeImports) Start(_ context.Context, user, _, url string) (imports.Import, error) {
	f.lastUser, f.lastURL = user, url
	f.starts++
	return f.imp, f.err
}

func (f *fakeImports) Latest(_ context.Context, user, _ string) (imports.Import, error) {
	f.lastUser = user
	return f.imp, f.err
}

func (f *fakeImports) Get(_ context.Context, user, _, _ string) (imports.Import, error) {
	f.lastUser = user
	return f.imp, f.err
}

func (f *fakeImports) Refresh(_ context.Context, user, _ string) (imports.Import, error) {
	f.lastUser = user
	f.starts++
	return f.imp, f.err
}

func (f *fakeImports) Select(_ context.Context, user, _, _ string, sel imports.Selection) (imports.Import, error) {
	f.lastUser, f.lastSel = user, sel
	return f.imp, f.err
}

const (
	pid = "aaaaaaaa-1111-4111-8111-000000000001"
	iid = "cccccccc-0000-4000-8000-000000000001"
)

func importHandler(svc ImportService, limits Limits) http.Handler {
	return NewHandler(fakeHealth{}, observability.New(&bytes.Buffer{}, slog.LevelInfo), Options{
		FrontendOrigin: frontend, MaxBodyBytes: 4096,
		Auth: &AuthOptions{
			Service: &tokenAuth{users: map[string]auth.User{"a": userA, "b": userB}}, Imports: svc,
			Limiter: &fakeLimiter{}, CookieName: "layr_session", Limits: limits,
		},
	})
}

func sampleImport(status string) imports.Import {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	return imports.Import{ID: iid, ProjectID: pid, FileKey: "FILEKEY123456", Status: status, CreatedAt: now, UpdatedAt: now}
}

func TestImportRoutesRequireAuthentication(t *testing.T) {
	h := importHandler(&fakeImports{}, Limits{})
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/v1/projects/" + pid + "/import"}, {"GET", "/api/v1/projects/" + pid + "/import"},
		{"GET", "/api/v1/projects/" + pid + "/imports/" + iid}, {"POST", "/api/v1/projects/" + pid + "/imports/" + iid + "/select"},
	} {
		if w := send(h, tc.method, tc.path, "", `{"figma_url":"x","node_id":"1:2"}`); w.Code != 401 {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestStartUsesSessionUserAndReturnsAccepted(t *testing.T) {
	f := &fakeImports{imp: sampleImport(imports.StatusPending)}
	h := importHandler(f, Limits{})

	w := send(h, "POST", "/api/v1/projects/"+pid+"/import", "b", `{"figma_url":"https://www.figma.com/design/KEY123456/x"}`)

	data := decodeData(t, w)
	if w.Code != http.StatusAccepted || f.lastUser != userB.ID || f.lastURL != "https://www.figma.com/design/KEY123456/x" || data["status"] != "pending" {
		t.Fatalf("status %d user %q body %s", w.Code, f.lastUser, w.Body.String())
	}
}

func TestStartValidation(t *testing.T) {
	f := &fakeImports{imp: sampleImport(imports.StatusPending)}
	h := importHandler(f, Limits{})
	path := "/api/v1/projects/" + pid + "/import"

	cases := []struct {
		name, body string
		status     int
		code       string
	}{
		{"missing url", `{}`, 400, "INVALID_FIGMA_URL"},
		{"malformed json", `{"figma_url":`, 400, "INVALID_REQUEST"},
		{"wrong type", `{"figma_url":5}`, 400, "INVALID_REQUEST"},
		{"user id smuggled", `{"figma_url":"x","user_id":"` + userB.ID + `"}`, 400, "INVALID_REQUEST"},
		{"project id smuggled", `{"figma_url":"x","project_id":"` + pid + `"}`, 400, "INVALID_REQUEST"},
		{"oversized", `{"figma_url":"` + strings.Repeat("a", 5000) + `"}`, 413, "REQUEST_TOO_LARGE"},
	}
	for _, tc := range cases {
		w := send(h, "POST", path, "a", tc.body)
		if w.Code != tc.status || errorCode(w) != tc.code {
			t.Errorf("%s: %d %q (%s)", tc.name, w.Code, errorCode(w), w.Body.String())
		}
	}
	if f.starts != 0 {
		t.Fatalf("service called %d times for invalid requests", f.starts)
	}
}

func TestImportErrorMapping(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
	}{
		{imports.ErrInvalidURL, 400, "INVALID_FIGMA_URL"},
		{imports.ErrInvalidID, 400, "INVALID_ID"},
		{imports.ErrInvalidSelection, 400, "INVALID_SELECTION"},
		{imports.ErrProjectNotFound, 404, "PROJECT_NOT_FOUND"},
		{imports.ErrNotFound, 404, "IMPORT_NOT_FOUND"},
		{imports.ErrConflict, 409, "IMPORT_CONFLICT"},
		{imports.ErrBusy, 503, "IMPORT_BUSY"},
		{errors.New("postgres: password=hunter2 refused"), 500, "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		h := importHandler(&fakeImports{err: tc.err}, Limits{})
		for _, r := range []struct{ method, path, body string }{
			{"POST", "/api/v1/projects/" + pid + "/import", `{"figma_url":"x"}`},
			{"GET", "/api/v1/projects/" + pid + "/import", ""},
			{"GET", "/api/v1/projects/" + pid + "/imports/" + iid, ""},
			{"POST", "/api/v1/projects/" + pid + "/imports/" + iid + "/select", `{"node_id":"1:2"}`},
		} {
			w := send(h, r.method, r.path, "a", r.body)
			if w.Code != tc.status || errorCode(w) != tc.code || strings.Contains(w.Body.String(), "hunter2") {
				t.Errorf("%v on %s %s: %d %q %s", tc.err, r.method, r.path, w.Code, errorCode(w), w.Body.String())
			}
		}
	}
}

func TestSelectValidation(t *testing.T) {
	f := &fakeImports{imp: sampleImport(imports.StatusProcessing)}
	h := importHandler(f, Limits{Import: 100})
	path := "/api/v1/projects/" + pid + "/imports/" + iid + "/select"

	for name, body := range map[string]string{"missing": `{}`, "empty": `{"node_id":""}`, "two modes": `{"node_id":"1:2","all":true}`, "empty in list": `{"node_ids":["1:2",""]}`, "all false": `{"all":false}`, "extra field": `{"node_id":"1:2","status":"completed"}`, "bad json": `[`} {
		if w := send(h, "POST", path, "a", body); w.Code != 400 {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if w := send(h, "POST", path, "a", `{"node_id":"1:2"}`); w.Code != http.StatusAccepted || len(f.lastSel.NodeIDs) != 1 || f.lastSel.NodeIDs[0] != "1:2" {
		t.Fatalf("valid selection: %d", w.Code)
	}
	if w := send(h, "POST", path, "a", `{"node_ids":["1:2","1:3"]}`); w.Code != http.StatusAccepted || len(f.lastSel.NodeIDs) != 2 {
		t.Fatalf("several screens: %d", w.Code)
	}
	if w := send(h, "POST", path, "a", `{"all":true}`); w.Code != http.StatusAccepted || !f.lastSel.All {
		t.Fatalf("all screens: %d", w.Code)
	}
}

func TestImportJSONShapesAreSafe(t *testing.T) {
	waiting := sampleImport(imports.StatusAwaitingSelection)
	waiting.Candidates = []imports.Frame{{ID: "1:2", Name: "Desktop", Type: "FRAME"}}
	done := sampleImport(imports.StatusCompleted)
	done.NodeID, done.NodeName, done.RenderFormat, done.RenderScale, done.FileName = "1:2", "Desktop", "png", 1, "Landing"
	at := done.CreatedAt
	done.CompletedAt = &at
	failed := sampleImport(imports.StatusFailed)
	failed.ErrorCode = "FIGMA_PERMISSION_DENIED"

	get := func(imp imports.Import) map[string]any {
		return decodeData(t, send(importHandler(&fakeImports{imp: imp}, Limits{}), "GET", "/api/v1/projects/"+pid+"/imports/"+iid, "a", ""))
	}

	w := get(waiting)
	frames, _ := w["frames"].([]any)
	if w["requires_selection"] != true || len(frames) != 1 || frames[0].(map[string]any)["name"] != "Desktop" {
		t.Fatalf("waiting = %v", w)
	}
	c := get(done)
	render, _ := c["reference_render"].(map[string]any)
	if c["figma_node_name"] != "Desktop" || render["format"] != "png" || c["completed_at"] == nil || c["requires_selection"] != false {
		t.Fatalf("completed = %v", c)
	}
	f := get(failed)
	errBody, _ := f["error"].(map[string]any)
	if errBody["code"] != "FIGMA_PERMISSION_DENIED" || !strings.Contains(errBody["message"].(string), "permission") {
		t.Fatalf("failed = %v", f)
	}
	for _, m := range []map[string]any{w, c, f} {
		raw, _ := json.Marshal(m)
		for _, forbidden := range []string{"temporary_url", "/tmp", "workspace", "amazonaws", "token"} {
			if strings.Contains(strings.ToLower(string(raw)), forbidden) {
				t.Fatalf("response leaks %q: %s", forbidden, raw)
			}
		}
	}
}

func TestImportStartsAreRateLimitedPerUser(t *testing.T) {
	f := &fakeImports{imp: sampleImport(imports.StatusPending)}
	h := importHandler(f, Limits{Import: 2})
	path := "/api/v1/projects/" + pid + "/import"

	for i := 0; i < 2; i++ {
		if w := send(h, "POST", path, "a", `{"figma_url":"x"}`); w.Code != 202 {
			t.Fatalf("start %d: %d", i, w.Code)
		}
	}
	if w := send(h, "POST", path, "a", `{"figma_url":"x"}`); w.Code != 429 || errorCode(w) != "RATE_LIMITED" {
		t.Fatalf("third start: %d", w.Code)
	}
	if w := send(h, "POST", path, "b", `{"figma_url":"x"}`); w.Code != 202 {
		t.Fatalf("another user must have their own budget: %d", w.Code)
	}
	if w := send(h, "GET", path, "a", ""); w.Code != 200 {
		t.Fatalf("reads must not spend the import budget: %d", w.Code)
	}
}

// liveFigma serves a canned two-frame file and one renderable node.
type liveFigma struct{ single bool }

func (l liveFigma) GetFile(_ context.Context, _, _ string, opts figma.FileOptions) (*figma.File, error) {
	_, _ = opts.Snapshot.Write([]byte(`{"name":"Landing","version":"3"}`))
	frames := []figma.Node{{ID: "1:2", Name: "Desktop", Type: "FRAME"}, {ID: "1:3", Name: "Mobile", Type: "FRAME"}}
	if l.single {
		frames = frames[:1]
	}
	return &figma.File{FileInfo: figma.FileInfo{Name: "Landing", Version: "3"},
		Document: figma.Node{Type: "DOCUMENT", Children: []figma.Node{{Type: "CANVAS", Children: frames}}}}, nil
}

func (liveFigma) GetFileNodes(_ context.Context, _, _ string, ids []string, opts figma.NodesOptions) (*figma.FileNodes, error) {
	_, _ = opts.Snapshot.Write([]byte(`{"name":"Landing","nodes":{}}`))
	return &figma.FileNodes{FileInfo: figma.FileInfo{Name: "Landing", Version: "3"},
		Nodes: map[string]figma.NodeEntry{ids[0]: {Document: figma.Node{ID: ids[0], Name: "Frame " + ids[0], Type: "FRAME"}}}}, nil
}

func (liveFigma) RenderNodes(_ context.Context, _, _ string, ids []string, _ figma.RenderOptions) (*figma.Renders, error) {
	return &figma.Renders{URLs: map[string]string{ids[0]: "https://figma-alpha-api.s3.example/secret-render.png"}}, nil
}

// TestLiveImportFlowAndIsolation drives the importer over HTTP against real PostgreSQL and a real workspace.
func TestLiveImportFlowAndIsolation(t *testing.T) {
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
	projects := map[string]string{}
	for token, fig := range map[string]string{"a": "live-imp-a", "b": "live-imp-b"} {
		var uid, prj string
		_ = pool.QueryRow(ctx, `INSERT INTO users (figma_user_id, display_name, created_at, updated_at) VALUES ($1,'n',now(),now()) RETURNING id`, fig).Scan(&uid)
		_ = pool.QueryRow(ctx, `INSERT INTO projects (user_id, name, created_at, updated_at) VALUES ($1,'p',now(),now()) RETURNING id`, uid).Scan(&prj)
		users[token], projects[token] = auth.User{ID: uid}, prj
	}
	root := t.TempDir()
	mgr, _ := workspace.NewManager(root, 1<<20, 1<<22)
	svc := imports.NewService(importstore.New(pool), liveFigma{}, mgr, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		imports.Config{Timeout: 5 * time.Second, WorkspaceTTL: time.Hour})
	defer svc.Close(ctx)
	h := NewHandler(fakeHealth{}, observability.New(&bytes.Buffer{}, slog.LevelInfo), Options{
		FrontendOrigin: frontend, MaxBodyBytes: 4096,
		Auth: &AuthOptions{Service: &tokenAuth{users: users}, Imports: svc, Limiter: &fakeLimiter{}, CookieName: "layr_session"},
	})

	base := "/api/v1/projects/" + projects["a"]
	start := send(h, "POST", base+"/import", "a", `{"figma_url":"https://www.figma.com/design/AbCdEfGhIjKlMnOp/Landing"}`)
	if start.Code != 202 {
		t.Fatalf("start: %d %s", start.Code, start.Body.String())
	}
	id := decodeData(t, start)["id"].(string)

	if w := send(h, "POST", "/api/v1/projects/"+projects["a"]+"/import", "b", `{"figma_url":"https://www.figma.com/design/AbCdEfGhIjKlMnOp/x"}`); w.Code != 404 {
		t.Fatalf("user B starting on A's project: %d", w.Code)
	}
	for _, r := range []struct{ method, path, body string }{
		{"GET", base + "/import", ""}, {"GET", base + "/imports/" + id, ""}, {"POST", base + "/imports/" + id + "/select", `{"node_id":"1:3"}`},
	} {
		if w := send(h, r.method, r.path, "b", r.body); w.Code != 404 {
			t.Fatalf("user B %s %s: %d", r.method, r.path, w.Code)
		}
	}
	if w := send(h, "GET", "/api/v1/projects/"+projects["b"]+"/imports/"+id, "b", ""); w.Code != 404 {
		t.Fatalf("user B via own project: %d", w.Code)
	}

	var waiting map[string]any
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		waiting = decodeData(t, send(h, "GET", base+"/imports/"+id, "a", ""))
		if waiting["status"] == "awaiting_selection" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if waiting["status"] != "awaiting_selection" || len(waiting["frames"].([]any)) != 2 {
		t.Fatalf("waiting = %v", waiting)
	}

	if w := send(h, "POST", base+"/imports/"+id+"/select", "a", `{"node_id":"9:9"}`); w.Code != 400 || errorCode(w) != "INVALID_SELECTION" {
		t.Fatalf("bad selection: %d %s", w.Code, w.Body.String())
	}
	if w := send(h, "POST", base+"/imports/"+id+"/select", "a", `{"node_id":"1:3"}`); w.Code != 202 {
		t.Fatalf("select: %d %s", w.Code, w.Body.String())
	}

	var done map[string]any
	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		w := send(h, "GET", base+"/import", "a", "")
		done = decodeData(t, w)
		if done["status"] == "completed" {
			if strings.Contains(w.Body.String(), "secret-render") || strings.Contains(w.Body.String(), root) {
				t.Fatalf("response leaks a temporary URL or path: %s", w.Body.String())
			}
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if done["status"] != "completed" || done["figma_node_id"] != "1:3" || done["figma_node_name"] != "Frame 1:3" || done["figma_version"] != "3" {
		t.Fatalf("done = %v", done)
	}
	dir, _ := mgr.Create(id)
	for _, f := range []struct{ sub, name string }{{workspace.Raw, "file.json"}, {workspace.Raw, "target-node.json"}, {workspace.Reference, "render.json"}} {
		if _, err := dir.ReadFile(f.sub, f.name); err != nil {
			t.Fatalf("%s/%s missing: %v", f.sub, f.name, err)
		}
	}
	if w := send(h, "POST", base+"/imports/"+id+"/select", "a", `{"node_id":"1:2"}`); w.Code != 409 {
		t.Fatalf("re-selecting a completed import: %d", w.Code)
	}
}

func TestDatabaseOutageIsA503WhileOtherFailuresStayAGeneric500(t *testing.T) {
	path := "/api/v1/projects/" + pid + "/import"
	for name, tc := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"server shutting down": {&pgconn.PgError{Code: "57P01"}, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
		"connection refused":   {&net.OpError{Op: "dial", Err: errors.New("refused")}, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE"},
		"unexpected bug":       {errors.New("SECRET boom"), http.StatusInternalServerError, "INTERNAL_ERROR"},
	} {
		h := importHandler(&fakeImports{err: tc.err}, Limits{})
		w := send(h, "GET", path, "a", "")
		if w.Code != tc.status || errorCode(w) != tc.code || strings.Contains(w.Body.String(), "SECRET") || strings.Contains(w.Body.String(), "57P01") {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
		if tc.status == http.StatusServiceUnavailable && w.Header().Get("Retry-After") == "" {
			t.Errorf("%s: a retryable outage needs Retry-After", name)
		}
	}
}

func TestRefreshRerunsTheImportForTheSessionUserOnly(t *testing.T) {
	f := &fakeImports{imp: sampleImport(imports.StatusPending)}
	h := importHandler(f, Limits{Import: 100})
	path := "/api/v1/projects/" + pid + "/import/refresh"

	if w := send(h, "POST", path, "", ""); w.Code != http.StatusUnauthorized {
		t.Fatalf("no session = %d", w.Code)
	}
	if w := send(h, "GET", path, "a", ""); w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET = %d", w.Code)
	}
	w := send(h, "POST", path, "b", "")
	if w.Code != http.StatusAccepted || f.lastUser != userB.ID || decodeData(t, w)["status"] != "pending" {
		t.Fatalf("%d %s (user %q)", w.Code, w.Body.String(), f.lastUser)
	}
	if w := send(h, "POST", path, "a", `{"figma_url":"https://www.figma.com/design/EVIL123456/x"}`); w.Code != http.StatusAccepted {
		t.Fatalf("a body must be ignored, never used to point the refresh elsewhere: %d", w.Code)
	}

	for name, tc := range map[string]struct {
		err    error
		status int
		code   string
	}{
		"nothing imported yet": {imports.ErrNotFound, 404, "IMPORT_NOT_FOUND"},
		"another import runs":  {imports.ErrConflict, 409, "IMPORT_CONFLICT"},
		"too busy":             {imports.ErrBusy, 503, "IMPORT_BUSY"},
		"foreign project":      {imports.ErrProjectNotFound, 404, "PROJECT_NOT_FOUND"},
	} {
		w := send(importHandler(&fakeImports{err: tc.err}, Limits{Import: 100}), "POST", path, "a", "")
		if w.Code != tc.status || errorCode(w) != tc.code {
			t.Errorf("%s: %d %s", name, w.Code, w.Body.String())
		}
	}
}

func (f *fakeImports) RetryAfter(string) time.Duration { return f.wait }

func TestRateLimitedImportsSayHowLongToWait(t *testing.T) {
	f := &fakeImports{err: &imports.RateLimitedError{RetryAfter: 4*24*time.Hour + 3*time.Hour}}
	h := importHandler(f, Limits{})
	w := send(h, "POST", "/api/v1/projects/"+pid+"/import", "b", `{"figma_url":"https://www.figma.com/design/KEY123456/x"}`)
	if w.Code != http.StatusTooManyRequests || errorCode(w) != "FIGMA_RATE_LIMITED" || w.Header().Get("Retry-After") == "" {
		t.Fatalf("%d %s %v", w.Code, w.Body.String(), w.Header())
	}
	if !strings.Contains(w.Body.String(), "about 4 days") {
		t.Fatalf("message = %s", w.Body.String())
	}
}
