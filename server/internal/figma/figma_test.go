package figma

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

var fixedNow = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newClient(t *testing.T, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return New(Config{
		ClientID:     "cid",
		ClientSecret: "csecret",
		RedirectURI:  "http://localhost:8080/auth/figma/callback",
		Now:          func() time.Time { return fixedNow },
		TokenURL:     srv.URL + "/token",
		RefreshURL:   srv.URL + "/refresh",
		MeURL:        srv.URL + "/me",
	})
}

func TestAuthorizeURLCarriesRequiredParameters(t *testing.T) {
	c := New(Config{ClientID: "cid", RedirectURI: "http://localhost/cb"})

	u, err := url.Parse(c.AuthorizeURL("st", "ch"))
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()

	if u.Host != "www.figma.com" || q.Get("response_type") != "code" || q.Get("state") != "st" ||
		q.Get("code_challenge") != "ch" || q.Get("code_challenge_method") != "S256" ||
		q.Get("scope") != "current_user:read file_content:read" || q.Get("redirect_uri") != "http://localhost/cb" {
		t.Fatalf("unexpected query: %v", q)
	}
	if strings.Contains(u.String(), "csecret") {
		t.Fatal("secret in authorize URL")
	}
}

func TestExchangeSuccess(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		_ = r.ParseForm()
		if user != "cid" || pass != "csecret" || r.PostForm.Get("code") != "the-code" ||
			r.PostForm.Get("code_verifier") != "ver" || r.PostForm.Get("grant_type") != "authorization_code" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"user_id_string":"u1","access_token":"a","refresh_token":"r","expires_in":3600,"token_type":"bearer"}`))
	})

	tok, err := c.Exchange(context.Background(), "the-code", "ver")

	if err != nil || tok.Access != "a" || tok.Refresh != "r" || tok.UserID != "u1" || !tok.ExpiresAt.Equal(fixedNow.Add(time.Hour)) {
		t.Fatalf("tok = %#v, err = %v", tok, err)
	}
}

func TestExchangeFailures(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   error
	}{
		"provider rejection": {http.StatusBadRequest, `{"error":"invalid_grant","secret":"leak"}`, ErrRejected},
		"provider outage":    {http.StatusBadGateway, `oops`, ErrUnavailable},
		"malformed json":     {http.StatusOK, `not json`, ErrMalformed},
		"missing refresh":    {http.StatusOK, `{"user_id_string":"u","access_token":"a","expires_in":10}`, ErrMalformed},
		"missing expiry":     {http.StatusOK, `{"user_id_string":"u","access_token":"a","refresh_token":"r"}`, ErrMalformed},
	}
	for name, tc := range cases {
		c := newClient(t, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})

		_, err := c.Exchange(context.Background(), "code", "ver")

		if !errors.Is(err, tc.want) || strings.Contains(err.Error(), "leak") {
			t.Fatalf("%s: err = %v, want %v", name, err, tc.want)
		}
	}
}

func TestExchangeTimeoutAndCancellation(t *testing.T) {
	block := make(chan struct{})
	c := newClient(t, func(http.ResponseWriter, *http.Request) { <-block })
	c.cfg.HTTP = &http.Client{Timeout: 50 * time.Millisecond}
	defer close(block)

	if _, err := c.Exchange(context.Background(), "c", "v"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("timeout err = %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := c.Exchange(ctx, "c", "v"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("cancel err = %v", err)
	}
}

func TestRefreshKeepsRefreshTokenEmptyWhenOmitted(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.PostForm.Get("refresh_token") != "old-refresh" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_, _ = w.Write([]byte(`{"access_token":"new","expires_in":7200}`))
	})

	tok, err := c.Refresh(context.Background(), "old-refresh")

	if err != nil || tok.Access != "new" || tok.Refresh != "" || !tok.ExpiresAt.Equal(fixedNow.Add(2*time.Hour)) {
		t.Fatalf("tok = %#v, err = %v", tok, err)
	}
}

func TestRefreshRejected(t *testing.T) {
	c := newClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusForbidden) })

	if _, err := c.Refresh(context.Background(), "revoked"); !errors.Is(err, ErrRejected) {
		t.Fatalf("err = %v", err)
	}
}

func TestMe(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		want   error
	}{
		"success":      {http.StatusOK, `{"id":"u1","email":"a@b.c","handle":"Ada","img_url":"https://img"}`, nil},
		"unauthorized": {http.StatusUnauthorized, `{}`, ErrRejected},
		"malformed":    {http.StatusOK, `[`, ErrMalformed},
		"no id":        {http.StatusOK, `{"email":"a@b.c"}`, ErrMalformed},
	}
	for name, tc := range cases {
		c := newClient(t, func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer tok" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(tc.body))
		})

		id, err := c.Me(context.Background(), "tok")

		if !errors.Is(err, tc.want) {
			t.Fatalf("%s: err = %v, want %v", name, err, tc.want)
		}
		if tc.want == nil && (id.ID != "u1" || id.Email != "a@b.c" || id.DisplayName != "Ada" || id.AvatarURL != "https://img") {
			t.Fatalf("%s: id = %#v", name, id)
		}
	}
}
