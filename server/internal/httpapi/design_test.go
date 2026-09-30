package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/ferousco-dev/layr/server/internal/designapi"
	"github.com/ferousco-dev/layr/server/internal/designapi/designtest"
	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/imports"
	importstore "github.com/ferousco-dev/layr/server/internal/imports/pgstore"
	"github.com/ferousco-dev/layr/server/internal/observability"
	"github.com/ferousco-dev/layr/server/internal/project"
	projectstore "github.com/ferousco-dev/layr/server/internal/project/pgstore"
	"github.com/ferousco-dev/layr/server/internal/workspace"
	"github.com/jackc/pgx/v5/pgxpool"
)

func designHandler(e *designtest.Env) http.Handler {
	return NewHandler(fakeHealth{}, observability.New(&bytes.Buffer{}, slog.LevelInfo), Options{
		FrontendOrigin: frontend, MaxBodyBytes: 4096,
		Auth: &AuthOptions{
			Service: &tokenAuth{users: map[string]auth.User{"a": {ID: designtest.OwnerA}, "b": {ID: designtest.OwnerB}}},
			Design:  e.Service(100), Limiter: &fakeLimiter{}, CookieName: "layr_session",
		},
	})
}

func base() string { return "/api/v1/projects/" + designtest.ProjectA + "/design" }

func TestDesignRoutesRequireAuthentication(t *testing.T) {
	h := designHandler(designtest.New(t))
	for _, path := range []string{
		base(), base() + "/screens", base() + "/screens/" + designtest.ScreenID(1), base() + "/screens/" + designtest.ScreenID(1) + "/preview",
		base() + "/flows/" + designtest.SectionID(1),
	} {
		if w := send(h, "GET", path, "", ""); w.Code != 401 {
			t.Errorf("%s without a session: %d", path, w.Code)
		}
		if w := send(h, "GET", path, "forged", ""); w.Code != 401 {
			t.Errorf("%s with a forged session: %d", path, w.Code)
		}
		if w := send(h, "POST", path, "a", `{}`); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s accepts writes: %d", path, w.Code)
		}
	}
}

func TestDesignSummaryOverHTTP(t *testing.T) {
	e := designtest.New(t)
	h := designHandler(e)

	w := send(h, "GET", base(), "a", "")

	if w.Code != 200 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	data := decodeData(t, w)
	screens, _ := data["screens"].([]any)
	if data["status"] != "ready" || len(screens) != 7 || len(data["flows"].([]any)) != 2 || data["design_version"] == "" {
		t.Fatalf("data = %v", data)
	}
	body := w.Body.String()
	for _, leak := range []string{e.Root, "/tmp", "reference/", "assets/", `"root"`, `"children"`, "sha256", "https://", "FILEKEY123456"} {
		if strings.Contains(body, leak) {
			t.Fatalf("response leaks %q", leak)
		}
	}
	if cc := w.Header().Get("Cache-Control"); strings.Contains(cc, "public") {
		t.Fatalf("Cache-Control = %q", cc)
	}
}

func TestOnlyTheOwnerCanReachADesign(t *testing.T) {
	h := designHandler(designtest.New(t))
	paths := []string{
		base(), base() + "/screens", base() + "/screens/" + designtest.ScreenID(2), base() + "/screens/" + designtest.ScreenID(2) + "/preview",
		base() + "/flows/" + designtest.SectionID(1),
	}
	missing := strings.Replace(base(), designtest.ProjectA, "00000000-0000-4000-8000-000000000000", 1)

	for _, path := range paths {
		foreign := send(h, "GET", path, "b", "")
		ghost := send(h, "GET", strings.Replace(path, base(), missing, 1), "b", "")
		if foreign.Code != 404 || errorCode(foreign) != "PROJECT_NOT_FOUND" || errorCode(ghost) != errorCode(foreign) || ghost.Code != foreign.Code {
			t.Errorf("%s: foreign %d %q, missing %d %q", path, foreign.Code, errorCode(foreign), ghost.Code, errorCode(ghost))
		}
		if strings.Contains(foreign.Body.String(), "SaaS Dashboard") || len(foreign.Body.Bytes()) > 400 {
			t.Errorf("%s: a foreign request received design data: %s", path, foreign.Body.String())
		}
	}
	if w := send(h, "GET", strings.Replace(base(), designtest.ProjectA, "not-a-uuid", 1), "a", ""); w.Code != 404 {
		t.Fatalf("malformed project: %d", w.Code)
	}
}

func TestPreviewIsPrivateStreamedAndConditional(t *testing.T) {
	e := designtest.New(t)
	h := designHandler(e)
	path := base() + "/screens/" + designtest.ScreenID(3) + "/preview"

	w := send(h, "GET", path, "a", "")

	if w.Code != 200 || w.Header().Get("Content-Type") != "image/png" || !bytes.Equal(w.Body.Bytes(), e.PNG) || w.Header().Get("Content-Length") != fmt.Sprint(len(e.PNG)) {
		t.Fatalf("status %d type %q bytes %d", w.Code, w.Header().Get("Content-Type"), w.Body.Len())
	}
	etag := w.Header().Get("ETag")
	if etag != `"`+fmt.Sprintf("%064x", 3)+`"` || w.Header().Get("Vary") != "Cookie" || w.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("headers = %v", w.Header())
	}
	if cc := w.Header().Get("Cache-Control"); cc != "private, no-cache" {
		t.Fatalf("without the version the browser must revalidate: %q", cc)
	}

	version := decodeData(t, send(h, "GET", base(), "a", ""))["design_version"].(string)
	versioned := send(h, "GET", path+"?v="+url.QueryEscape(version), "a", "")
	if cc := versioned.Header().Get("Cache-Control"); cc != "private, max-age=86400, immutable" {
		t.Fatalf("versioned Cache-Control = %q", cc)
	}
	if stale := send(h, "GET", path+"?v=dv_stale", "a", ""); stale.Header().Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("a stale version must not be cached long: %q", stale.Header().Get("Cache-Control"))
	}

	for _, inm := range []string{etag, "W/" + etag, `"other", ` + etag, "*"} {
		r := httptest.NewRequest("GET", path, nil)
		r.AddCookie(&http.Cookie{Name: "layr_session", Value: "a"})
		r.Header.Set("If-None-Match", inm)
		cond := httptest.NewRecorder()
		h.ServeHTTP(cond, r)
		if cond.Code != http.StatusNotModified || cond.Body.Len() != 0 || cond.Header().Get("ETag") != etag {
			t.Errorf("If-None-Match %q: %d body %d", inm, cond.Code, cond.Body.Len())
		}
	}
	r := httptest.NewRequest("GET", path, nil)
	r.AddCookie(&http.Cookie{Name: "layr_session", Value: "a"})
	r.Header.Set("If-None-Match", `"different"`)
	other := httptest.NewRecorder()
	h.ServeHTTP(other, r)
	if other.Code != 200 {
		t.Fatalf("a different ETag must get the image: %d", other.Code)
	}
}

func TestPreviewIdentifiersCannotBeUsedToReachFiles(t *testing.T) {
	e := designtest.New(t)
	h := designHandler(e)

	for _, id := range []string{
		"..%2F..%2Fetc%2Fpasswd", "screen_../../x", "SCREEN_0000000000000001", "screen_000000000000000", "screen_00000000000000010",
		"%2e%2e", "screen_%2e%2e%2e%2e%2e%2e%2e%2e", "reference", "x.png", "screen_zzzzzzzzzzzzzzzz",
	} {
		for _, suffix := range []string{"", "/preview"} {
			w := send(h, "GET", base()+"/screens/"+id+suffix, "a", "")
			// The router may clean a traversal path and redirect; either way no data is returned.
			rejected := w.Code == 404 || w.Code == http.StatusTemporaryRedirect
			if !rejected || strings.Contains(w.Body.String(), "SaaS") || (w.Code == 404 && errorCode(w) != "SCREEN_NOT_FOUND" && errorCode(w) != "NOT_FOUND") {
				t.Errorf("%q%s: %d %q", id, suffix, w.Code, errorCode(w))
			}
		}
	}
	for _, id := range []string{"..%2Fx", "section_../x", "SECTION_0000000000000001", "x"} {
		w := send(h, "GET", base()+"/flows/"+id, "a", "")
		if (w.Code != 404 && w.Code != http.StatusTemporaryRedirect) || (w.Code == 404 && errorCode(w) != "FLOW_NOT_FOUND" && errorCode(w) != "NOT_FOUND") {
			t.Errorf("flow %q: %d %q", id, w.Code, errorCode(w))
		}
	}
}

func TestDesignStatesOverHTTP(t *testing.T) {
	t.Run("missing preview", func(t *testing.T) {
		e := designtest.New(t)
		_ = e.Dir.Remove(workspace.Reference, "screen-2.png")
		w := send(designHandler(e), "GET", base()+"/screens/"+designtest.ScreenID(2)+"/preview", "a", "")
		if w.Code != 404 || errorCode(w) != "PREVIEW_NOT_AVAILABLE" {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
		if detail := send(designHandler(e), "GET", base()+"/screens/"+designtest.ScreenID(2), "a", ""); detail.Code != 200 {
			t.Fatalf("screen metadata must stay usable: %d", detail.Code)
		}
	})
	t.Run("expired", func(t *testing.T) {
		e := designtest.New(t)
		h := designHandler(e)
		_ = e.Manager.Cleanup(designtest.ImportID)
		if w := send(h, "GET", base(), "a", ""); w.Code != 200 || decodeData(t, w)["status"] != "expired" {
			t.Fatalf("summary: %d %s", w.Code, w.Body.String())
		}
		for _, path := range []string{base() + "/screens", base() + "/screens/" + designtest.ScreenID(1), base() + "/screens/" + designtest.ScreenID(1) + "/preview"} {
			if w := send(h, "GET", path, "a", ""); w.Code != http.StatusGone || errorCode(w) != "DESIGN_DATA_EXPIRED" {
				t.Errorf("%s: %d %q", path, w.Code, errorCode(w))
			}
		}
	})
	t.Run("not ready and no design", func(t *testing.T) {
		e := designtest.New(t)
		e.Imports.Rows = nil
		h := designHandler(e)
		if w := send(h, "GET", base(), "a", ""); w.Code != 404 || errorCode(w) != "DESIGN_NOT_FOUND" {
			t.Fatalf("no import: %d %s", w.Code, w.Body.String())
		}
	})
	t.Run("corrupt design", func(t *testing.T) {
		e := designtest.New(t)
		_ = e.Dir.WriteBytes(workspace.Design, "design-ir.json", []byte(`{"schema_version":1,`))
		w := send(designHandler(e), "GET", base(), "a", "")
		if w.Code != 500 || errorCode(w) != "DESIGN_IR_INVALID" || strings.Contains(w.Body.String(), e.Root) || strings.Contains(w.Body.String(), "unexpected") {
			t.Fatalf("%d %s", w.Code, w.Body.String())
		}
	})
}

func TestScreenListQueriesOverHTTP(t *testing.T) {
	h := designHandler(designtest.New(t))

	w := send(h, "GET", base()+"/screens?limit=2&offset=1&q=o", "a", "")
	var body struct {
		Data   []map[string]any `json:"data"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != 200 {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if body.Total != 3 || len(body.Data) != 2 || body.Limit != 2 || body.Offset != 1 || body.Data[0]["name"] != "Forgot Password" {
		t.Fatalf("page = %+v", body)
	}
	for _, q := range []string{"limit=0", "limit=abc", "limit=-1", "offset=-1", "offset=x", "flow=nope", "limit=501"} {
		if w := send(h, "GET", base()+"/screens?"+q, "a", ""); w.Code != 400 || errorCode(w) != "INVALID_REQUEST" {
			t.Errorf("%s: %d %q", q, w.Code, errorCode(w))
		}
	}
	if w := send(h, "GET", base()+"/screens?flow="+designtest.SectionID(7), "a", ""); w.Code != 404 || errorCode(w) != "FLOW_NOT_FOUND" {
		t.Errorf("unknown flow: %d", w.Code)
	}
}

func TestConcurrentDesignRequestsAreRaceFree(t *testing.T) {
	e := designtest.New(t)
	h := designHandler(e)

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for _, path := range []string{base(), base() + "/screens?limit=3", base() + "/screens/" + designtest.ScreenID(i%7+1), base() + "/screens/" + designtest.ScreenID(i%7+1) + "/preview"} {
				if w := send(h, "GET", path, "a", ""); w.Code != 200 {
					t.Errorf("%s: %d", path, w.Code)
				}
			}
		}(i)
	}
	wg.Wait()
}

// TestLiveDesignAfterARealImport runs import, storage and the design API together on real PostgreSQL.
func TestLiveDesignAfterARealImport(t *testing.T) {
	dbURL := os.Getenv("TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	_, _ = pool.Exec(ctx, `TRUNCATE users CASCADE`)

	users, projects := map[string]auth.User{}, map[string]string{}
	for token, fig := range map[string]string{"a": "live-design-a", "b": "live-design-b"} {
		var uid, prj string
		_ = pool.QueryRow(ctx, `INSERT INTO users (figma_user_id, display_name, created_at, updated_at) VALUES ($1,'n',now(),now()) RETURNING id`, fig).Scan(&uid)
		_ = pool.QueryRow(ctx, `INSERT INTO projects (user_id, name, created_at, updated_at) VALUES ($1,'p',now(),now()) RETURNING id`, uid).Scan(&prj)
		users[token], projects[token] = auth.User{ID: uid}, prj
	}
	root := t.TempDir()
	mgr, _ := workspace.NewManager(root, 8<<20, 32<<20)
	store := importstore.New(pool)
	svc := imports.NewService(store, liveFigma{single: true}, mgr, slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)),
		imports.Config{Timeout: 5 * time.Second, WorkspaceTTL: time.Hour})
	svc.SetDesign(imports.DesignConfig{MaxNodes: 10000, MaxBytes: 4 << 20})
	defer svc.Close(ctx)
	projectSvc := project.NewService(projectstore.New(pool))
	designSvc := designapi.New(projectSvc, store, mgr, nil, designapi.Config{Limits: designir.DefaultLimits, MaxIRBytes: 4 << 20})
	h := NewHandler(fakeHealth{}, observability.New(&bytes.Buffer{}, slog.LevelInfo), Options{
		FrontendOrigin: frontend, MaxBodyBytes: 4096,
		Auth: &AuthOptions{Service: &tokenAuth{users: users}, Imports: svc, Design: designSvc, Limiter: &fakeLimiter{}, CookieName: "layr_session"},
	})
	design := "/api/v1/projects/" + projects["a"] + "/design"

	if w := send(h, "GET", design, "a", ""); w.Code != 404 || errorCode(w) != "DESIGN_NOT_FOUND" {
		t.Fatalf("before any import: %d %s", w.Code, w.Body.String())
	}
	if w := send(h, "POST", "/api/v1/projects/"+projects["a"]+"/import", "a", `{"figma_url":"https://www.figma.com/design/AbCdEfGhIjKlMnOp/Landing"}`); w.Code != 202 {
		t.Fatalf("start: %d %s", w.Code, w.Body.String())
	}

	var data map[string]any
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data = decodeData(t, send(h, "GET", design, "a", ""))
		if data["status"] == "ready" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	screens, _ := data["screens"].([]any)
	if data["status"] != "ready" || len(screens) != 1 || data["design_version"] == "" {
		t.Fatalf("design = %v", data)
	}
	screen := screens[0].(map[string]any)
	id := screen["id"].(string)
	if screen["name"] != "Frame 1:2" || screen["preview"].(map[string]any)["available"] != false {
		t.Fatalf("screen = %v", screen)
	}
	if w := send(h, "GET", design+"/screens/"+id, "a", ""); w.Code != 200 {
		t.Fatalf("detail: %d %s", w.Code, w.Body.String())
	}
	if w := send(h, "GET", design+"/screens/"+id+"/preview", "a", ""); w.Code != 404 || errorCode(w) != "PREVIEW_NOT_AVAILABLE" {
		t.Fatalf("preview without a stored reference: %d %s", w.Code, w.Body.String())
	}
	for _, path := range []string{design, design + "/screens", design + "/screens/" + id, design + "/screens/" + id + "/preview"} {
		if w := send(h, "GET", path, "b", ""); w.Code != 404 || errorCode(w) != "PROJECT_NOT_FOUND" {
			t.Fatalf("stranger on %s: %d %s", path, w.Code, w.Body.String())
		}
	}
}

func TestAssetRoutesListAndStreamTheDesignsFilesPrivately(t *testing.T) {
	ir := designtest.BuildIR(2, false)
	ir.Assets = []designir.Asset{{ID: "asset_00000000000000a1", Kind: "image", Format: "png", MediaType: "image/png", Path: "assets/hero-a1b2c3.png", SizeBytes: 8, SHA256: strings.Repeat("a", 64), ScreenIDs: []string{}}}
	e := designtest.NewWith(t, ir)
	e.WriteAsset("hero-a1b2c3.png", []byte("PNGDATA!"))
	h := designHandler(e)
	fileURL := base() + "/assets/asset_00000000000000a1/file"

	for _, path := range []string{base() + "/assets", fileURL} {
		if w := send(h, "GET", path, "", ""); w.Code != 401 {
			t.Errorf("%s without a session: %d", path, w.Code)
		}
		if w := send(h, "GET", path, "b", ""); w.Code != 404 || errorCode(w) != "PROJECT_NOT_FOUND" {
			t.Errorf("%s for a stranger: %d %s", path, w.Code, w.Body.String())
		}
		if w := send(h, "POST", path, "a", `{}`); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s accepts writes: %d", path, w.Code)
		}
	}

	list := send(h, "GET", base()+"/assets", "a", "")
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &body); err != nil || list.Code != 200 || len(body.Data) != 1 || body.Data[0]["name"] != "hero" {
		t.Fatalf("list = %d %s", list.Code, list.Body.String())
	}
	for _, leak := range []string{e.Root, "assets/hero", "sha256", "a1b2c3", strings.Repeat("a", 64)} {
		if strings.Contains(list.Body.String(), leak) {
			t.Fatalf("the list leaks %q", leak)
		}
	}
	if !strings.HasPrefix(body.Data[0]["url"].(string), fileURL) {
		t.Fatalf("url = %v", body.Data[0]["url"])
	}

	w := send(h, "GET", fileURL, "a", "")
	etag := w.Header().Get("ETag")
	if w.Code != 200 || w.Body.String() != "PNGDATA!" || w.Header().Get("Content-Type") != "image/png" || etag != `"`+strings.Repeat("a", 64)+`"` ||
		w.Header().Get("Cache-Control") != "private, no-cache" || w.Header().Get("Vary") != "Cookie" || w.Header().Get("Content-Security-Policy") == "" {
		t.Fatalf("file = %d %q headers %v", w.Code, w.Body.String(), w.Header())
	}
	r := httptest.NewRequest("GET", fileURL, nil)
	r.AddCookie(&http.Cookie{Name: "layr_session", Value: "a"})
	r.Header.Set("If-None-Match", etag)
	cond := httptest.NewRecorder()
	h.ServeHTTP(cond, r)
	if cond.Code != http.StatusNotModified || cond.Body.Len() != 0 {
		t.Fatalf("conditional = %d", cond.Code)
	}

	for _, id := range []string{"asset_ffffffffffffffff", "not-an-asset", "..%2Fsecret", "asset_" + strings.Repeat("0", 40)} {
		if w := send(h, "GET", base()+"/assets/"+id+"/file", "a", ""); w.Code != 404 {
			t.Errorf("id %q = %d", id, w.Code)
		}
	}
}
