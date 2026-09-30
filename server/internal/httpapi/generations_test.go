package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/ferousco-dev/layr/server/internal/designapi"
	"github.com/ferousco-dev/layr/server/internal/generation"
	"github.com/ferousco-dev/layr/server/internal/observability"
)

type fakeGenerations struct {
	gen         generation.Generation
	err         error
	lastUser    string
	lastProv    string
	starts      int
	getUser     string
	lastScreens []string
}

func (f *fakeGenerations) Start(_ context.Context, user, _, provider string, screens []string) (generation.Generation, error) {
	f.lastUser, f.lastProv, f.lastScreens = user, provider, screens
	f.starts++
	return f.gen, f.err
}

func (f *fakeGenerations) Get(_ context.Context, user, _, _ string) (generation.Generation, error) {
	f.getUser = user
	return f.gen, f.err
}

func generationHandler(f *fakeGenerations) http.Handler {
	return NewHandler(fakeHealth{}, observability.New(&bytes.Buffer{}, slog.LevelInfo), Options{
		FrontendOrigin: frontend,
		MaxBodyBytes:   1024,
		Auth: &AuthOptions{
			Service:     &tokenAuth{users: map[string]auth.User{"a": userA, "b": userB}},
			Generations: f,
			Limiter:     &fakeLimiter{},
			CookieName:  "layr_session",
			Limits:      Limits{Import: 100},
		},
	})
}

const genID = "cccccccc-0000-4000-8000-000000000001"

var sampleGen = generation.Generation{
	ID: genID, ProjectID: pid, Provider: "anthropic", Status: generation.StatusFailed, ErrorCode: generation.CodeNotAvailable,
	CreatedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC), UpdatedAt: time.Date(2026, 9, 30, 12, 0, 3, 0, time.UTC),
	Steps: []generation.Step{
		{ID: "read_design", Label: "Reading your design", Status: generation.StepDone, Detail: "5 screens"},
		{ID: "write_code", Label: "Writing code with Claude (Anthropic)", Status: generation.StepFailed, Detail: "not available"},
	},
}

func TestGenerationRoutesRequireAuthentication(t *testing.T) {
	h := generationHandler(&fakeGenerations{})
	for _, tc := range []struct{ method, path string }{
		{"POST", "/api/v1/projects/" + pid + "/generations"},
		{"GET", "/api/v1/projects/" + pid + "/generations/" + genID},
	} {
		if w := send(h, tc.method, tc.path, "", `{}`); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestStartingAGenerationUsesTheSessionUserAndReturnsThePlan(t *testing.T) {
	f := &fakeGenerations{gen: sampleGen}
	w := send(generationHandler(f), "POST", "/api/v1/projects/"+pid+"/generations", "b", `{"provider":"anthropic"}`)
	if w.Code != http.StatusAccepted || f.lastUser != userB.ID || f.lastProv != "anthropic" {
		t.Fatalf("%d %s (user %q)", w.Code, w.Body.String(), f.lastUser)
	}
	d := decodeData(t, w)
	steps := d["steps"].([]any)
	prog := d["progress"].(map[string]any)
	if d["provider_label"] != "Claude (Anthropic)" || len(steps) != 2 || prog["done"] != float64(1) || prog["total"] != float64(2) {
		t.Fatalf("data = %v", d)
	}
	e := d["error"].(map[string]any)
	if e["code"] != "GENERATION_NOT_AVAILABLE" || !strings.Contains(e["message"].(string), "not available yet") {
		t.Fatalf("error = %v", e)
	}
	for _, leak := range []string{"api_key", "sk-", "ciphertext"} {
		if strings.Contains(w.Body.String(), leak) {
			t.Fatalf("response leaks %q", leak)
		}
	}
}

func TestStartValidationAndErrorMapping(t *testing.T) {
	path := "/api/v1/projects/" + pid + "/generations"
	cases := []struct {
		name, body string
		err        error
		status     int
		code       string
	}{
		{"no provider", `{}`, nil, 400, "INVALID_PROVIDER"},
		{"unknown provider", `{"provider":"gemini"}`, generation.ErrUnknownProvider, 400, "INVALID_PROVIDER"},
		{"no saved key", `{"provider":"openai"}`, generation.ErrNoKey, 400, "KEY_REQUIRED"},
		{"design not ready", `{"provider":"xai"}`, generation.ErrDesignNotReady, 409, "DESIGN_NOT_READY"},
		{"no design at all", `{"provider":"xai"}`, designapi.ErrDesignNotFound, 409, "DESIGN_NOT_READY"},
		{"already running", `{"provider":"xai"}`, generation.ErrConflict, 409, "GENERATION_RUNNING"},
		{"foreign project", `{"provider":"xai"}`, designapi.ErrProjectNotFound, 404, "PROJECT_NOT_FOUND"},
		{"smuggled field", `{"provider":"xai","user_id":"` + userB.ID + `"}`, nil, 400, "INVALID_REQUEST"},
		{"malformed", `{"provider":`, nil, 400, "INVALID_REQUEST"},
		{"outage", `{"provider":"xai"}`, errors.New("SECRET boom"), 500, "INTERNAL_ERROR"},
	}
	for _, tc := range cases {
		f := &fakeGenerations{gen: sampleGen, err: tc.err}
		w := send(generationHandler(f), "POST", path, "a", tc.body)
		if w.Code != tc.status || errorCode(w) != tc.code || strings.Contains(w.Body.String(), "SECRET") {
			t.Errorf("%s: %d %s", tc.name, w.Code, w.Body.String())
		}
		if tc.status == 400 && tc.code == "INVALID_REQUEST" && f.starts != 0 {
			t.Errorf("%s reached the service", tc.name)
		}
	}
	if w := send(generationHandler(&fakeGenerations{}), "POST", "/api/v1/projects/not-a-uuid/generations", "a", `{"provider":"xai"}`); w.Code != 400 || errorCode(w) != "INVALID_PROJECT_ID" {
		t.Errorf("bad project id: %d %s", w.Code, w.Body.String())
	}
}

func TestGettingAGenerationIsScopedAndValidatesIDs(t *testing.T) {
	f := &fakeGenerations{gen: sampleGen}
	h := generationHandler(f)
	if w := send(h, "GET", "/api/v1/projects/"+pid+"/generations/"+genID, "a", ""); w.Code != 200 || f.getUser != userA.ID {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	for _, bad := range []string{"not-a-uuid", "..%2Fx", strings.Repeat("a", 60)} {
		if w := send(h, "GET", "/api/v1/projects/"+pid+"/generations/"+bad, "a", ""); w.Code != 404 {
			t.Errorf("id %q = %d", bad, w.Code)
		}
	}
	gone := generationHandler(&fakeGenerations{err: generation.ErrNotFound})
	if w := send(gone, "GET", "/api/v1/projects/"+pid+"/generations/"+genID, "b", ""); w.Code != 404 || errorCode(w) != "GENERATION_NOT_FOUND" {
		t.Fatalf("stranger: %d %s", w.Code, w.Body.String())
	}
}

func TestOnlyPostStartsAndOnlyGetReads(t *testing.T) {
	h := generationHandler(&fakeGenerations{gen: sampleGen})
	if w := send(h, "GET", "/api/v1/projects/"+pid+"/generations", "a", ""); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET list = %d", w.Code)
	}
	if w := send(h, "DELETE", "/api/v1/projects/"+pid+"/generations/"+genID, "a", ""); w.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE = %d", w.Code)
	}
}

func TestChosenScreensAreValidatedAndPassedOn(t *testing.T) {
	path := "/api/v1/projects/" + pid + "/generations"
	ok := []string{"screen_00000000000000a1", "screen_00000000000000b2"}

	f := &fakeGenerations{gen: sampleGen}
	w := send(generationHandler(f), "POST", path, "a", `{"provider":"anthropic","screen_ids":["`+ok[0]+`","`+ok[1]+`"]}`)
	if w.Code != http.StatusAccepted || len(f.lastScreens) != 2 || f.lastScreens[1] != ok[1] {
		t.Fatalf("%d %s (screens %v)", w.Code, w.Body.String(), f.lastScreens)
	}
	if d := decodeData(t, w); d["screen_ids"] != nil {
		t.Fatalf("the fake job covers all screens, so screen_ids must be null: %v", d["screen_ids"])
	}

	none := &fakeGenerations{gen: sampleGen}
	if w := send(generationHandler(none), "POST", path, "a", `{"provider":"anthropic"}`); w.Code != http.StatusAccepted || none.lastScreens != nil {
		t.Fatalf("no selection means all screens: %d %v", w.Code, none.lastScreens)
	}

	bad := []string{`[]`, `["nope"]`, `["screen_../x"]`, `["` + ok[0] + `",""]`, `[` + strings.Repeat(`"`+ok[0]+`",`, 600) + `"` + ok[0] + `"]`}
	for _, list := range bad {
		f := &fakeGenerations{gen: sampleGen}
		w := send(NewHandler(fakeHealth{}, observability.New(&bytes.Buffer{}, slog.LevelInfo), Options{
			FrontendOrigin: frontend, MaxBodyBytes: 1 << 20,
			Auth: &AuthOptions{Service: &tokenAuth{users: map[string]auth.User{"a": userA}}, Generations: f, Limiter: &fakeLimiter{}, CookieName: "layr_session", Limits: Limits{Import: 100}},
		}), "POST", path, "a", `{"provider":"anthropic","screen_ids":`+list+`}`)
		if w.Code != 400 || errorCode(w) != "INVALID_SELECTION" || f.starts != 0 {
			t.Errorf("%.30s: %d %s (starts %d)", list, w.Code, w.Body.String(), f.starts)
		}
	}

	wrong := &fakeGenerations{gen: sampleGen, err: generation.ErrInvalidSelection}
	if w := send(generationHandler(wrong), "POST", path, "a", `{"provider":"anthropic","screen_ids":["`+ok[0]+`"]}`); w.Code != 400 || errorCode(w) != "INVALID_SELECTION" {
		t.Fatalf("a screen that is not in the design: %d %s", w.Code, w.Body.String())
	}
}
