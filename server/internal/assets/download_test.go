package assets

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestBlockedIPCoversInternalRanges(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "127.9.9.9", "::1", "10.0.0.1", "172.16.0.1", "172.31.255.255", "192.168.1.1", "169.254.169.254", "169.254.0.5",
		"100.64.0.1", "0.0.0.0", "::", "fe80::1", "fc00::1", "fd12:3456::1", "fd00:ec2::254", "224.0.0.1", "ff02::1",
		"::ffff:127.0.0.1", "::ffff:10.1.2.3", "::ffff:169.254.169.254", "198.18.0.1", "240.0.0.1",
	}
	for _, s := range blocked {
		if !blockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "52.216.0.1", "172.32.0.1", "2606:4700::1111", "3.5.140.2"} {
		if blockedIP(netip.MustParseAddr(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestPolicyValidateURL(t *testing.T) {
	strict := Policy{}
	cases := []struct {
		raw string
		ok  bool
	}{
		{"https://s3-alpha-sig.figma.com/img/abc?Expires=1&Signature=x", true},
		{"https://figma-alpha-api.s3.us-west-2.amazonaws.com/images/x", true},
		{"https://example.com:443/x", true},
		{"http://example.com/x", false},
		{"ftp://example.com/x", false},
		{"file:///etc/passwd", false},
		{"javascript:alert(1)", false},
		{"data:image/png;base64,AAAA", false},
		{"https://user:pw@example.com/x", false},
		{"https://example.com:8443/x", false},
		{"https://127.0.0.1/x", false},
		{"https://[::1]/x", false},
		{"https://10.0.0.5/x", false},
		{"https://169.254.169.254/latest/meta-data", false},
		{"https://[::ffff:127.0.0.1]/x", false},
		{"https://localhost/x", false},
		{"https://api.localhost/x", false},
		{"https://db.internal/x", false},
		{"https://printer.local/x", false},
		{"https:///nohost", false},
	}
	for _, tc := range cases {
		u, err := url.Parse(tc.raw)
		if err != nil {
			if tc.ok {
				t.Errorf("%s: %v", tc.raw, err)
			}
			continue
		}
		if got := strict.validateURL(u) == nil; got != tc.ok {
			t.Errorf("%s: allowed=%v, want %v", tc.raw, got, tc.ok)
		}
	}
}

// TestControlChecksTheResolvedAddress covers DNS rebinding: whatever a name resolves to is judged at connect time.
func TestControlChecksTheResolvedAddress(t *testing.T) {
	strict := Policy{}
	for address, ok := range map[string]bool{
		"127.0.0.1:443": false, "[::1]:443": false, "10.0.0.1:443": false, "169.254.169.254:80": false,
		"[::ffff:10.0.0.1]:443": false, "[fe80::1]:443": false, "not-an-address": false, "8.8.8.8:443": true, "[2606:4700::1111]:443": true,
	} {
		if got := strict.control("tcp", address, nil) == nil; got != ok {
			t.Errorf("%s: allowed=%v, want %v", address, got, ok)
		}
	}
	if (Policy{AllowInsecure: true}).control("tcp", "127.0.0.1:1", nil) != nil {
		t.Error("test policy must allow loopback")
	}
}

func TestFetchRefusesInternalHostsWithoutConnecting(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	d := NewDownloader(Policy{}, 1<<20)

	for _, target := range []string{srv.URL, "https://127.0.0.1:1/x", "http://169.254.169.254/latest/meta-data"} {
		_, err := d.Fetch(context.Background(), target, io.Discard)
		if !errors.Is(err, ErrURLBlocked) {
			t.Errorf("%s: err = %v", target, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatal("a request reached a loopback server")
	}
}

func testDownloader(max int64) *Downloader { return NewDownloader(Policy{AllowInsecure: true}, max) }

func TestFetchStreamsBodyAndReportsContentType(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("hello"))
	}))
	defer srv.Close()
	var buf bytes.Buffer

	got, err := testDownloader(100).Fetch(context.Background(), srv.URL, &buf)

	if err != nil || got.Size != 5 || got.ContentType != "image/png" || buf.String() != "hello" {
		t.Fatalf("got %+v err %v body %q", got, err, buf.String())
	}
}

func TestFetchStopsAtTheSizeLimitWithoutBufferingIt(t *testing.T) {
	var sent atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		chunk := make([]byte, 32<<10)
		for i := 0; i < 4096; i++ { // up to 128 MiB, streamed
			n, err := w.Write(chunk)
			sent.Add(int64(n))
			if err != nil {
				return
			}
		}
	}))
	defer srv.Close()
	var buf countingWriter

	_, err := testDownloader(1<<20).Fetch(context.Background(), srv.URL, &buf)

	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
	if buf.n > 1<<20+1 {
		t.Fatalf("wrote %d bytes past the limit", buf.n)
	}
}

type countingWriter struct{ n int64 }

func (c *countingWriter) Write(p []byte) (int, error) { c.n += int64(len(p)); return len(p), nil }

func TestFetchRejectsDeclaredOversizeImmediately(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Length", "5000")
		_, _ = w.Write(make([]byte, 5000))
	}))
	defer srv.Close()
	var buf countingWriter

	_, err := testDownloader(1000).Fetch(context.Background(), srv.URL, &buf)

	if !errors.Is(err, ErrTooLarge) || buf.n != 0 {
		t.Fatalf("err = %v, wrote %d", err, buf.n)
	}
}

func TestFetchStatusErrorsCarryOnlyTheStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<Error>SignatureDoesNotMatch secret-token</Error>"))
	}))
	defer srv.Close()

	_, err := testDownloader(100).Fetch(context.Background(), srv.URL+"/x?Signature=SUPER_SECRET", io.Discard)

	var se *StatusError
	if !errors.As(err, &se) || se.Status != 403 || strings.Contains(err.Error(), "SUPER_SECRET") || strings.Contains(err.Error(), "secret-token") {
		t.Fatalf("err = %v", err)
	}
}

func TestTransportErrorsNeverExposeTheSignedURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	addr := srv.URL
	srv.Close()

	_, err := testDownloader(100).Fetch(context.Background(), addr+"/img?Signature=SUPER_SECRET", io.Discard)

	if !errors.Is(err, ErrDownload) || strings.Contains(err.Error(), "SUPER_SECRET") || strings.Contains(err.Error(), "127.0.0.1") {
		t.Fatalf("err = %v", err)
	}
}

func TestRedirects(t *testing.T) {
	var loops atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/final", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("done")) })
	mux.HandleFunc("/hop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/final", http.StatusFound) })
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) {
		loops.Add(1)
		http.Redirect(w, r, "/loop", http.StatusFound)
	})
	mux.HandleFunc("/file", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "file:///etc/passwd")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/creds", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://user:pw@127.0.0.1/x")
		w.WriteHeader(http.StatusFound)
	})
	mux.HandleFunc("/bad", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Location", "http://%zz")
		w.WriteHeader(http.StatusFound)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	d := testDownloader(100)

	var buf bytes.Buffer
	if _, err := d.Fetch(context.Background(), srv.URL+"/hop", &buf); err != nil || buf.String() != "done" {
		t.Fatalf("allowed redirect: %v %q", err, buf.String())
	}
	if _, err := d.Fetch(context.Background(), srv.URL+"/loop", io.Discard); err == nil || loops.Load() > maxRedirects+2 {
		t.Fatalf("redirect loop: err %v after %d hops", err, loops.Load())
	}
	for _, path := range []string{"/file", "/creds", "/bad"} {
		if _, err := d.Fetch(context.Background(), srv.URL+path, io.Discard); err == nil {
			t.Errorf("%s: unsafe redirect followed", path)
		}
	}
}

func TestEveryRedirectIsRevalidatedUnderTheStrictPolicy(t *testing.T) {
	d := NewDownloader(Policy{}, 100)
	req := func(target string) *http.Request {
		u, _ := url.Parse(target)
		return &http.Request{URL: u}
	}

	if err := d.client.CheckRedirect(req("https://cdn.example.com/x"), make([]*http.Request, 1)); err != nil {
		t.Errorf("safe redirect refused: %v", err)
	}
	for _, target := range []string{"http://169.254.169.254/latest", "https://127.0.0.1/x", "https://localhost/x", "file:///etc/passwd", "https://u:p@cdn.example.com/x", "https://cdn.example.com:8443/x"} {
		if err := d.client.CheckRedirect(req(target), make([]*http.Request, 1)); !errors.Is(err, ErrURLBlocked) {
			t.Errorf("%s: err = %v", target, err)
		}
	}
	if err := d.client.CheckRedirect(req("https://cdn.example.com/x"), make([]*http.Request, maxRedirects+1)); !errors.Is(err, ErrURLBlocked) {
		t.Errorf("too many redirects: %v", err)
	}
}

func TestFetchHonoursCancellation(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-block }))
	defer srv.Close()
	defer close(block)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := testDownloader(100).Fetch(ctx, srv.URL, io.Discard); done <- err }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("fetch did not stop after cancellation")
	}
}
