package httpapi

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/account"
	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/ferousco-dev/layr/server/internal/observability"
)

type fakeAccount struct {
	profile  account.Profile
	err      error
	saved    map[string]string
	lastUser string
	deleted  []string
}

func (f *fakeAccount) Profile(_ context.Context, user string) (account.Profile, error) {
	f.lastUser = user
	return f.profile, f.err
}
func (f *fakeAccount) SaveKey(_ context.Context, user, provider, key string) (account.KeyStatus, error) {
	f.lastUser = user
	if f.err != nil {
		return account.KeyStatus{}, f.err
	}
	if err := account.ValidateKey(provider, key); err != nil {
		return account.KeyStatus{}, err
	}
	if f.saved == nil {
		f.saved = map[string]string{}
	}
	f.saved[user+provider] = key
	return account.KeyStatus{Provider: provider, Saved: true, Hint: account.Hint(key), UpdatedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)}, nil
}
func (f *fakeAccount) DeleteKey(_ context.Context, user, provider string) error {
	f.lastUser = user
	if _, ok := map[string]bool{"anthropic": true, "openai": true, "xai": true}[provider]; !ok {
		return account.ErrUnknownProvider
	}
	return f.err
}
func (f *fakeAccount) DeleteAccount(_ context.Context, user string) error {
	f.lastUser = user
	if f.err == nil {
		f.deleted = append(f.deleted, user)
	}
	return f.err
}

func accountHandler(f *fakeAccount, logs *bytes.Buffer) http.Handler {
	return NewHandler(fakeHealth{}, observability.New(logs, slog.LevelInfo), Options{
		FrontendOrigin: frontend,
		MaxBodyBytes:   1024,
		Auth: &AuthOptions{
			Service:    &tokenAuth{users: map[string]auth.User{"a": userA, "b": userB}},
			Account:    f,
			Limiter:    &fakeLimiter{},
			CookieName: "layr_session",
		},
	})
}

const testKey = "sk-ant-api03-abcdefghijklmnopqrstuvwxyz0123456789"

func TestProfileRoutesRequireAuthentication(t *testing.T) {
	h := accountHandler(&fakeAccount{}, &bytes.Buffer{})
	for _, tc := range []struct{ method, path string }{
		{"GET", "/api/v1/profile"}, {"DELETE", "/api/v1/profile"},
		{"PUT", "/api/v1/profile/ai-keys/anthropic"}, {"DELETE", "/api/v1/profile/ai-keys/anthropic"},
	} {
		if w := send(h, tc.method, tc.path, "", `{}`); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s = %d", tc.method, tc.path, w.Code)
		}
		if w := send(h, tc.method, tc.path, "not-a-session", `{}`); w.Code != http.StatusUnauthorized {
			t.Errorf("%s %s with a bad session = %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestProfileShowsIdentityConnectionAndKeyHintsOnly(t *testing.T) {
	f := &fakeAccount{profile: account.Profile{FigmaStatus: "active", Keys: []account.KeyStatus{
		{Provider: "anthropic", Saved: true, Hint: "6789", UpdatedAt: time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)},
		{Provider: "openai"}, {Provider: "xai"},
	}}}
	w := send(accountHandler(f, &bytes.Buffer{}), "GET", "/api/v1/profile", "a", "")
	if w.Code != 200 || f.lastUser != userA.ID {
		t.Fatalf("%d %s (asked for %q)", w.Code, w.Body.String(), f.lastUser)
	}
	data := decodeData(t, w)
	if data["figma"].(map[string]any)["status"] != "active" || data["user"].(map[string]any)["name"] != "A" {
		t.Fatalf("data = %v", data)
	}
	keys := data["ai_keys"].([]any)
	first, last := keys[0].(map[string]any), keys[2].(map[string]any)
	if len(keys) != 3 || first["saved"] != true || first["hint"] != "6789" || last["saved"] != false || last["hint"] != nil {
		t.Fatalf("keys = %v", keys)
	}
	for _, leak := range []string{"ciphertext", "api_key", "sk-"} {
		if strings.Contains(w.Body.String(), leak) {
			t.Fatalf("response leaks %q: %s", leak, w.Body.String())
		}
	}
}

func TestSavingAKeyNeverEchoesOrLogsIt(t *testing.T) {
	f, logs := &fakeAccount{}, &bytes.Buffer{}
	h := accountHandler(f, logs)

	w := send(h, "PUT", "/api/v1/profile/ai-keys/anthropic", "a", `{"api_key":"`+testKey+`"}`)
	if w.Code != 200 || f.saved[userA.ID+"anthropic"] != testKey {
		t.Fatalf("%d %s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), testKey) || strings.Contains(logs.String(), testKey) || strings.Contains(logs.String(), "abcdefghijkl") {
		t.Fatalf("the key leaked\nresponse: %s\nlogs: %s", w.Body.String(), logs.String())
	}
	if d := decodeData(t, w); d["hint"] != "6789" || d["saved"] != true {
		t.Fatalf("data = %v", d)
	}
	if f.saved[userB.ID+"anthropic"] != "" {
		t.Fatal("saved for the wrong person")
	}
}

func TestKeyValidationAndMassAssignment(t *testing.T) {
	f := &fakeAccount{}
	h := accountHandler(f, &bytes.Buffer{})
	cases := []struct {
		name, path, body string
		status           int
		code             string
	}{
		{"wrong shape", "/api/v1/profile/ai-keys/anthropic", `{"api_key":"nope"}`, 400, "INVALID_API_KEY"},
		{"wrong provider prefix", "/api/v1/profile/ai-keys/xai", `{"api_key":"` + testKey + `"}`, 400, "INVALID_API_KEY"},
		{"missing key", "/api/v1/profile/ai-keys/anthropic", `{}`, 400, "INVALID_API_KEY"},
		{"unknown provider", "/api/v1/profile/ai-keys/gemini", `{"api_key":"` + testKey + `"}`, 404, "UNKNOWN_PROVIDER"},
		{"traversal provider", "/api/v1/profile/ai-keys/..%2Fusers", `{"api_key":"` + testKey + `"}`, 404, ""},
		{"smuggled user id", "/api/v1/profile/ai-keys/anthropic", `{"api_key":"` + testKey + `","user_id":"` + userB.ID + `"}`, 400, "INVALID_REQUEST"},
		{"malformed json", "/api/v1/profile/ai-keys/anthropic", `{"api_key":`, 400, "INVALID_REQUEST"},
		{"oversized", "/api/v1/profile/ai-keys/anthropic", `{"api_key":"` + strings.Repeat("a", 3000) + `"}`, 413, "REQUEST_TOO_LARGE"},
	}
	for _, tc := range cases {
		w := send(h, "PUT", tc.path, "a", tc.body)
		if w.Code != tc.status || (tc.code != "" && errorCode(w) != tc.code) {
			t.Errorf("%s: %d %s", tc.name, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), testKey) {
			t.Errorf("%s: the error echoed the key", tc.name)
		}
	}
	if len(f.saved) != 0 {
		t.Fatalf("invalid requests saved %v", f.saved)
	}
}

func TestDeletingAKeyIsIdempotentAndRejectsUnknownProviders(t *testing.T) {
	h := accountHandler(&fakeAccount{}, &bytes.Buffer{})
	for i := 0; i < 2; i++ {
		if w := send(h, "DELETE", "/api/v1/profile/ai-keys/openai", "a", ""); w.Code != http.StatusNoContent {
			t.Fatalf("delete %d = %d", i, w.Code)
		}
	}
	if w := send(h, "DELETE", "/api/v1/profile/ai-keys/gemini", "a", ""); w.Code != 404 || errorCode(w) != "UNKNOWN_PROVIDER" {
		t.Fatalf("unknown provider: %d %s", w.Code, w.Body.String())
	}
}

func TestDeletingTheAccountNeedsTheExactPhraseAndClearsTheCookie(t *testing.T) {
	f := &fakeAccount{}
	h := accountHandler(f, &bytes.Buffer{})

	for name, body := range map[string]string{
		"no body": `{}`, "wrong phrase": `{"confirm":"yes"}`, "case": `{"confirm":"Delete My Account"}`, "extra field": `{"confirm":"delete my account","user_id":"` + userB.ID + `"}`,
	} {
		if w := send(h, "DELETE", "/api/v1/profile", "a", body); w.Code != 400 {
			t.Errorf("%s = %d", name, w.Code)
		}
	}
	if len(f.deleted) != 0 {
		t.Fatalf("deleted without confirmation: %v", f.deleted)
	}

	w := send(h, "DELETE", "/api/v1/profile", "a", `{"confirm":"delete my account"}`)
	if w.Code != http.StatusNoContent || len(f.deleted) != 1 || f.deleted[0] != userA.ID {
		t.Fatalf("%d, deleted %v", w.Code, f.deleted)
	}
	cleared := false
	for _, c := range w.Result().Cookies() {
		if c.Name == "layr_session" && c.MaxAge < 0 {
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("the session cookie must be cleared")
	}
}

func TestAccountFailuresAre503ForOutagesAnd404ForGoneAccounts(t *testing.T) {
	gone := accountHandler(&fakeAccount{err: account.ErrUserNotFound}, &bytes.Buffer{})
	if w := send(gone, "DELETE", "/api/v1/profile", "a", `{"confirm":"delete my account"}`); w.Code != 404 || errorCode(w) != "ACCOUNT_NOT_FOUND" {
		t.Fatalf("gone: %d %s", w.Code, w.Body.String())
	}
	bug := accountHandler(&fakeAccount{err: errors.New("SECRET boom")}, &bytes.Buffer{})
	if w := send(bug, "GET", "/api/v1/profile", "a", ""); w.Code != 500 || strings.Contains(w.Body.String(), "SECRET") {
		t.Fatalf("bug: %d %s", w.Code, w.Body.String())
	}
}

func TestCORSAllowsPutForProfileKeys(t *testing.T) {
	h := accountHandler(&fakeAccount{}, &bytes.Buffer{})
	r := httptest.NewRequest("OPTIONS", "/api/v1/profile/ai-keys/anthropic", nil)
	r.Header.Set("Origin", frontend)
	r.Header.Set("Access-Control-Request-Method", "PUT")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if !strings.Contains(w.Header().Get("Access-Control-Allow-Methods"), "PUT") {
		t.Fatalf("methods = %q", w.Header().Get("Access-Control-Allow-Methods"))
	}
}
