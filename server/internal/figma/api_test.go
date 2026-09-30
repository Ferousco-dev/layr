package figma

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const secretToken = "SUPER_SECRET_TEST_TOKEN"

type fakeTokens struct {
	token        string
	accessErr    error
	refreshErr   error
	refreshed    atomic.Int32
	rejectedSeen string
}

func (f *fakeTokens) AccessToken(context.Context, string) (string, error) {
	return f.token, f.accessErr
}

func (f *fakeTokens) RefreshRejected(_ context.Context, _, rejected string) (string, error) {
	f.refreshed.Add(1)
	f.rejectedSeen = rejected
	return "REFRESHED_TEST_TOKEN", f.refreshErr
}

type rig struct {
	api    *API
	tokens *fakeTokens
	logs   *bytes.Buffer
	sleeps []time.Duration
	hits   atomic.Int32
}

func newRig(t *testing.T, handler http.HandlerFunc) *rig {
	t.Helper()
	r := &rig{tokens: &fakeTokens{token: secretToken}, logs: &bytes.Buffer{}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.hits.Add(1)
		handler(w, req)
	}))
	t.Cleanup(srv.Close)
	r.api = NewAPI(APIConfig{
		Tokens:  r.tokens,
		Log:     slog.New(slog.NewTextHandler(r.logs, nil)),
		BaseURL: srv.URL,
		Sleep: func(ctx context.Context, d time.Duration) error {
			r.sleeps = append(r.sleeps, d)
			return ctx.Err()
		},
		Jitter: func() float64 { return 1 },
	})
	return r
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func serve(body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
}

func status(code int) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(code) }
}

func TestGetFileDecodesTypedTreeAndSendsCorrectRequest(t *testing.T) {
	var got *http.Request
	r := newRig(t, func(w http.ResponseWriter, req *http.Request) {
		got = req
		serve(fixture(t, "simple_file.json"))(w, req)
	})

	file, err := r.api.GetFile(context.Background(), "u1", "FILEKEY", FileOptions{Depth: 2, Version: "4242", Geometry: true})
	if err != nil {
		t.Fatal(err)
	}

	if got.URL.Path != "/v1/files/FILEKEY" || got.URL.Query().Get("depth") != "2" ||
		got.URL.Query().Get("version") != "4242" || got.URL.Query().Get("geometry") != "paths" {
		t.Fatalf("request = %s", got.URL.String())
	}
	if got.Header.Get("Authorization") != "Bearer "+secretToken || got.Header.Get("User-Agent") != "Layr/0.1" {
		t.Fatal("auth or user agent header missing")
	}
	if file.Name != "Landing Page" || file.Version != "4242" || file.Document.Type != NodeDocument {
		t.Fatalf("file = %#v", file.FileInfo)
	}
	hero := file.Document.Children[0].Children[0]
	if hero.Name != "Hero" || !hero.ClipsContent || hero.Constraints.Horizontal != "LEFT_RIGHT" || hero.AbsoluteBoundingBox.Width != 1440 {
		t.Fatalf("hero = %#v", hero)
	}
	if file.Components["2:1"].Name != "Button" || file.ComponentSets["2:0"].Key != "set1" || file.Styles["S:1"].StyleType != "TEXT" {
		t.Fatal("component or style metadata missing")
	}
}

func TestUnknownNodeTypeAndFieldsAreTolerated(t *testing.T) {
	r := newRig(t, serve(fixture(t, "simple_file.json")))

	file, err := r.api.GetFile(context.Background(), "u1", "K", FileOptions{})
	if err != nil {
		t.Fatal(err)
	}

	mystery := file.Document.Children[0].Children[0].Children[0]
	if mystery.Type != "FUTURE_SUPER_NODE" || mystery.Known() || mystery.ID != "1:3" {
		t.Fatalf("mystery = %#v", mystery)
	}
	if !file.Document.Children[0].Children[0].Known() {
		t.Fatal("FRAME should be known")
	}
}

func TestGetFileNodesPreservesAutoLayoutTextPaintsAndComponents(t *testing.T) {
	var query string
	r := newRig(t, func(w http.ResponseWriter, req *http.Request) {
		query = req.URL.Query().Get("ids")
		serve(fixture(t, "autolayout_text.json"))(w, req)
	})

	res, err := r.api.GetFileNodes(context.Background(), "u1", "K", []string{"9:9", "1:2", "1:2"}, NodesOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if query != "1:2,9:9" {
		t.Fatalf("ids = %q, want sorted and de-duplicated", query)
	}
	if len(res.Missing) != 1 || res.Missing[0] != "9:9" {
		t.Fatalf("missing = %v", res.Missing)
	}
	card := res.Nodes["1:2"].Document
	if card.LayoutMode != "VERTICAL" || card.PrimaryAxisAlignItems != "SPACE_BETWEEN" || card.PaddingLeft != 16 ||
		card.ItemSpacing != 12 || card.LayoutSizingHorizontal != "FILL" || len(card.RectangleCornerRadii) != 4 {
		t.Fatalf("auto layout lost: %#v", card)
	}
	if len(card.Fills) != 3 || card.Fills[0].Color.G != 0.2 || card.Fills[1].GradientStops[1].Color.B != 1 || card.Fills[2].ImageRef != "abc123ref" {
		t.Fatalf("paints lost: %#v", card.Fills)
	}
	if !card.Effects[0].IsVisible() || card.Effects[0].Offset.Y != 2 || card.Effects[1].IsVisible() {
		t.Fatalf("effects lost: %#v", card.Effects)
	}
	title := card.Children[0]
	if title.Characters != "Hello world" || title.Style.FontFamily != "Inter" || title.Style.FontWeight != 700 || title.Style.LetterSpacing != -0.5 ||
		title.Style.TextCase != "UPPER" || !title.Style.Italic || len(title.CharacterStyleOverrides) != 5 || title.StyleOverrideTable["1"].FontWeight != 400 {
		t.Fatalf("text lost: %#v", title)
	}
	if card.Children[1].FillGeometry[0].Path != "M0 0L10 10" {
		t.Fatal("vector geometry lost")
	}
	inst := card.Children[2]
	if inst.ComponentID != "2:1" || inst.ComponentProperties["Label#1:0"].Value != "Buy" || res.Nodes["1:2"].Components["2:1"].Name != "Button" {
		t.Fatalf("instance lost: %#v", inst)
	}
}

func TestGetImageFillsHandlesBothResponseShapes(t *testing.T) {
	for name, body := range map[string][]byte{
		"meta shape":      fixture(t, "image_fills.json"),
		"top level shape": []byte(`{"images": {"abc123ref": "https://example.test/a.png", "def456ref": "https://example.test/b.png"}}`),
	} {
		r := newRig(t, serve(body))

		fills, err := r.api.GetImageFills(context.Background(), "u1", "K")

		if err != nil || len(fills) != 2 || fills["abc123ref"] != "https://example.test/a.png" {
			t.Fatalf("%s: fills = %v, err = %v", name, fills, err)
		}
	}
	empty := newRig(t, serve([]byte(`{"meta":{"images":{}}}`)))
	if fills, err := empty.api.GetImageFills(context.Background(), "u1", "K"); err != nil || fills == nil || len(fills) != 0 {
		t.Fatalf("empty fills = %v, err = %v", fills, err)
	}
}

func TestRenderNodesPNGAndSVG(t *testing.T) {
	var last *http.Request
	r := newRig(t, func(w http.ResponseWriter, req *http.Request) {
		last = req
		serve([]byte(`{"err":null,"images":{"1:2":"https://example.test/one.svg","1:3":null}}`))(w, req)
	})
	off := false

	res, err := r.api.RenderNodes(context.Background(), "u1", "K", []string{"1:3", "1:2"}, RenderOptions{
		Format: FormatSVG, Scale: 2, SVGOutlineText: &off, SVGIncludeID: true, UseAbsoluteBounds: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	q := last.URL.Query()
	if last.URL.Path != "/v1/images/K" || q.Get("format") != "svg" || q.Get("scale") != "2" || q.Get("svg_outline_text") != "false" ||
		q.Get("svg_include_id") != "true" || q.Get("use_absolute_bounds") != "true" || q.Get("ids") != "1:2,1:3" {
		t.Fatalf("query = %s", last.URL.RawQuery)
	}
	if res.URLs["1:2"] != "https://example.test/one.svg" || len(res.Failed) != 1 || res.Failed[0] != "1:3" {
		t.Fatalf("res = %#v", res)
	}

	png, err := r.api.RenderNodes(context.Background(), "u1", "K", []string{"1:2"}, RenderOptions{})
	if err != nil || last.URL.Query().Get("format") != "png" || last.URL.Query().Has("svg_include_id") || png.URLs["1:2"] == "" {
		t.Fatalf("default png request: %s err %v", last.URL.RawQuery, err)
	}
}

func TestRenderNodesReportsMissingNodeAndProviderError(t *testing.T) {
	r := newRig(t, serve([]byte(`{"err":null,"images":{}}`)))
	res, err := r.api.RenderNodes(context.Background(), "u1", "K", []string{"1:2"}, RenderOptions{})
	if err != nil || len(res.Failed) != 1 {
		t.Fatalf("res = %#v, err = %v", res, err)
	}

	bad := newRig(t, serve([]byte(`{"err":"Not found","images":null}`)))
	if _, err := bad.api.RenderNodes(context.Background(), "u1", "K", []string{"1:2"}, RenderOptions{}); KindOf(err) != KindBadResponse {
		t.Fatalf("err = %v", err)
	}
}

func TestInputValidationRejectsBeforeAnyNetworkCall(t *testing.T) {
	r := newRig(t, status(200))
	ctx := context.Background()
	tooMany := make([]string, maxNodeIDs+1)
	for i := range tooMany {
		tooMany[i] = "1:" + string(rune('a'+i%26)) + string(rune('a'+i/26))
	}

	checks := map[string]error{
		"empty key":         second(r.api.GetFile(ctx, "u", "", FileOptions{})),
		"slash in key":      second(r.api.GetFile(ctx, "u", "a/b", FileOptions{})),
		"query in key":      second(r.api.GetFile(ctx, "u", "a?x=1", FileOptions{})),
		"negative depth":    second(r.api.GetFile(ctx, "u", "K", FileOptions{Depth: -1})),
		"bad version":       second(r.api.GetFile(ctx, "u", "K", FileOptions{Version: "1 2"})),
		"no node ids":       second(r.api.GetFileNodes(ctx, "u", "K", nil, NodesOptions{})),
		"empty node id":     second(r.api.GetFileNodes(ctx, "u", "K", []string{""}, NodesOptions{})),
		"comma in node id":  second(r.api.GetFileNodes(ctx, "u", "K", []string{"1:2,3:4"}, NodesOptions{})),
		"too many node ids": second(r.api.GetFileNodes(ctx, "u", "K", tooMany, NodesOptions{})),
		"bad format":        second(r.api.RenderNodes(ctx, "u", "K", []string{"1:2"}, RenderOptions{Format: "gif"})),
		"scale too big":     second(r.api.RenderNodes(ctx, "u", "K", []string{"1:2"}, RenderOptions{Scale: 5})),
		"scale negative":    second(r.api.RenderNodes(ctx, "u", "K", []string{"1:2"}, RenderOptions{Scale: -1})),
		"fills empty key":   second(r.api.GetImageFills(ctx, "u", "")),
	}
	for name, err := range checks {
		if !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
	if r.hits.Load() != 0 {
		t.Fatalf("network was called %d times for invalid input", r.hits.Load())
	}
}

func second[T any](_ T, err error) error { return err }

func TestQueryValuesAreEncoded(t *testing.T) {
	var raw string
	r := newRig(t, func(w http.ResponseWriter, req *http.Request) {
		raw = req.URL.RawQuery
		serve([]byte(`{"images":{}}`))(w, req)
	})

	_, _ = r.api.RenderNodes(context.Background(), "u", "K", []string{"I5:1;2:3"}, RenderOptions{})

	if strings.Contains(raw, ";") || !strings.Contains(raw, "ids=I5%3A1%3B2%3A3") {
		t.Fatalf("query not encoded: %s", raw)
	}
}

func TestStatusMapping(t *testing.T) {
	cases := []struct {
		code      int
		kind      Kind
		retryable bool
		requests  int32
	}{
		{400, KindBadRequest, false, 1},
		{403, KindPermissionDenied, false, 1},
		{404, KindNotFound, false, 1},
		{500, KindUnavailable, true, 1},
		{418, KindBadResponse, false, 1},
		{502, KindUnavailable, true, 3},
		{503, KindUnavailable, true, 3},
		{504, KindUnavailable, true, 3},
	}
	for _, tc := range cases {
		r := newRig(t, status(tc.code))

		_, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})

		var fe *Error
		if !errors.As(err, &fe) || fe.Kind != tc.kind || fe.Retryable != tc.retryable || fe.Status != tc.code {
			t.Fatalf("%d: err = %#v", tc.code, err)
		}
		if r.hits.Load() != tc.requests {
			t.Fatalf("%d: requests = %d, want %d", tc.code, r.hits.Load(), tc.requests)
		}
	}
}

func TestRetriesThenSucceeds(t *testing.T) {
	var n atomic.Int32
	r := newRig(t, func(w http.ResponseWriter, req *http.Request) {
		if n.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		serve([]byte(`{"name":"ok","document":{"id":"0:0","type":"DOCUMENT","name":"d"}}`))(w, req)
	})

	file, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})

	if err != nil || file.Name != "ok" || r.hits.Load() != 3 {
		t.Fatalf("err = %v, hits = %d", err, r.hits.Load())
	}
	if len(r.sleeps) != 2 || r.sleeps[0] != 500*time.Millisecond || r.sleeps[1] != time.Second {
		t.Fatalf("backoff = %v, want 500ms then 1s", r.sleeps)
	}
}

func TestRateLimitHonoursRetryAfterThenReturnsTypedError(t *testing.T) {
	r := newRig(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "3")
		w.Header().Set("X-Figma-Plan-Tier", "pro")
		w.Header().Set("X-Figma-Rate-Limit-Type", "low")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})

	var fe *Error
	if !errors.As(err, &fe) || fe.Kind != KindRateLimited || !fe.Retryable || fe.RetryAfter != 3*time.Second || fe.PlanTier != "pro" || fe.RateLimitType != "low" {
		t.Fatalf("err = %#v", err)
	}
	if r.hits.Load() != 3 || len(r.sleeps) != 2 || r.sleeps[0] != 3*time.Second {
		t.Fatalf("hits %d sleeps %v: attempts must be bounded and Retry-After honoured", r.hits.Load(), r.sleeps)
	}
}

func TestLongRetryAfterIsReturnedToCallerWithoutWaiting(t *testing.T) {
	r := newRig(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	_, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})

	var fe *Error
	if !errors.As(err, &fe) || fe.RetryAfter != time.Hour || r.hits.Load() != 1 || len(r.sleeps) != 0 {
		t.Fatalf("err = %#v hits %d sleeps %v", err, r.hits.Load(), r.sleeps)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"":     0,
		"5":    5 * time.Second,
		"-5":   0,
		"soon": 0,
		now.Add(90 * time.Second).UTC().Format(http.TimeFormat): 90 * time.Second,
		now.Add(-time.Hour).UTC().Format(http.TimeFormat):       0,
	}
	for in, want := range cases {
		if got := parseRetryAfter(in, now); got != want {
			t.Fatalf("%q: got %v want %v", in, got, want)
		}
	}
}

func TestCancellationDuringBackoffStopsRetries(t *testing.T) {
	r := newRig(t, status(503))
	ctx, cancel := context.WithCancel(context.Background())
	r.api.cfg.Sleep = func(ctx context.Context, _ time.Duration) error {
		cancel()
		return ctx.Err()
	}

	_, err := r.api.GetFile(ctx, "u", "K", FileOptions{})

	if KindOf(err) != KindCancelled || !errors.Is(err, context.Canceled) || r.hits.Load() != 1 {
		t.Fatalf("err = %v hits %d", err, r.hits.Load())
	}
}

func TestCancelledContextStopsRequest(t *testing.T) {
	r := newRig(t, serve([]byte(`{}`)))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := r.api.GetFile(ctx, "u", "K", FileOptions{})

	if KindOf(err) != KindCancelled {
		t.Fatalf("err = %v", err)
	}
}

func TestClientTimeoutIsTypedAndNotRetried(t *testing.T) {
	block := make(chan struct{})
	r := newRig(t, func(http.ResponseWriter, *http.Request) { <-block })
	defer close(block)
	r.api.cfg.HTTP = &http.Client{Timeout: 50 * time.Millisecond}

	_, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})

	if KindOf(err) != KindTimeout || r.hits.Load() != 1 {
		t.Fatalf("err = %v hits %d", err, r.hits.Load())
	}
}

func TestUnauthorizedRefreshesOnceThenRetries(t *testing.T) {
	r := newRig(t, func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") == "Bearer REFRESHED_TEST_TOKEN" {
			serve([]byte(`{"name":"ok","document":{"id":"0:0","type":"DOCUMENT","name":"d"}}`))(w, req)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	})

	file, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})

	if err != nil || file.Name != "ok" || r.tokens.refreshed.Load() != 1 || r.tokens.rejectedSeen != secretToken || r.hits.Load() != 2 {
		t.Fatalf("err = %v refreshed %d hits %d", err, r.tokens.refreshed.Load(), r.hits.Load())
	}
}

func TestUnauthorizedTwiceIsBoundedAndTyped(t *testing.T) {
	r := newRig(t, status(401))

	_, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})

	if KindOf(err) != KindTokenExpired || r.tokens.refreshed.Load() != 1 || r.hits.Load() != 2 {
		t.Fatalf("err = %v refreshed %d hits %d", err, r.tokens.refreshed.Load(), r.hits.Load())
	}
}

func TestCredentialFailuresPropagate(t *testing.T) {
	r := newRig(t, status(200))
	r.tokens.accessErr = ErrReconnectRequired

	_, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})
	if KindOf(err) != KindAuthRequired || !errors.Is(err, ErrReconnectRequired) || r.hits.Load() != 0 {
		t.Fatalf("reconnect: err = %v hits %d", err, r.hits.Load())
	}

	r.tokens.accessErr = errors.New("redis down")
	_, err = r.api.GetFile(context.Background(), "u", "K", FileOptions{})
	if fe := (*Error)(nil); !errors.As(err, &fe) || fe.Kind != KindUnavailable || !fe.Retryable {
		t.Fatalf("transient: err = %#v", err)
	}

	r.tokens.accessErr = nil
	r.tokens.refreshErr = ErrReconnectRequired
	r2 := newRig(t, status(401))
	r2.tokens.refreshErr = ErrReconnectRequired
	if _, err := r2.api.GetFile(context.Background(), "u", "K", FileOptions{}); KindOf(err) != KindAuthRequired {
		t.Fatalf("refresh rejected: err = %v", err)
	}
}

func TestBadBodiesAreBadResponses(t *testing.T) {
	for name, body := range map[string]string{"malformed": `{"name":`, "empty": ``, "not json": `<html>oops</html>`} {
		r := newRig(t, serve([]byte(body)))

		_, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})

		if KindOf(err) != KindBadResponse {
			t.Fatalf("%s: err = %v", name, err)
		}
	}
}

func TestOversizedResponseIsRejected(t *testing.T) {
	r := newRig(t, serve([]byte(`{"name":"`+strings.Repeat("a", 5000)+`"}`)))
	r.api.cfg.MaxResponseBytes = 1000

	_, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})

	if KindOf(err) != KindBadResponse || !errors.Is(err, errTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	var elsewhere atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		elsewhere.Add(1)
	}))
	defer other.Close()
	r := newRig(t, func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, other.URL, http.StatusFound)
	})
	r.api = NewAPI(APIConfig{Tokens: r.tokens, BaseURL: r.api.cfg.BaseURL, Sleep: r.api.cfg.Sleep})

	_, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})

	if KindOf(err) != KindBadResponse || elsewhere.Load() != 0 {
		t.Fatalf("err = %v, redirect target hit %d times", err, elsewhere.Load())
	}
}

func TestTokenNeverAppearsInErrorsOrLogs(t *testing.T) {
	for _, code := range []int{400, 401, 403, 404, 429, 500, 503} {
		r := newRig(t, status(code))

		_, err := r.api.GetFile(context.Background(), "user-1", "K", FileOptions{})

		if err == nil || strings.Contains(err.Error(), secretToken) || strings.Contains(r.logs.String(), secretToken) || strings.Contains(r.logs.String(), "REFRESHED_TEST_TOKEN") {
			t.Fatalf("%d: token leaked. err=%v logs=%s", code, err, r.logs.String())
		}
	}
}

func TestSuccessLogCarriesUsefulFieldsOnly(t *testing.T) {
	r := newRig(t, serve([]byte(`{"nodes":{}}`)))

	_, _ = r.api.GetFileNodes(context.Background(), "user-1", "FILEKEY", []string{"1:2", "1:3"}, NodesOptions{})

	logs := r.logs.String()
	for _, want := range []string{"figma_operation=get_file_nodes", "user_id=user-1", "file_key=FILEKEY", "node_count=2", "status=200", "attempts=1"} {
		if !strings.Contains(logs, want) {
			t.Fatalf("log missing %q: %s", want, logs)
		}
	}
	if strings.Contains(logs, "document") {
		t.Fatal("log must not contain payload")
	}
}

func TestBaseURLIsFixedToFigmaByDefault(t *testing.T) {
	api := NewAPI(APIConfig{Tokens: &fakeTokens{}})

	if api.cfg.BaseURL != "https://api.figma.com" {
		t.Fatalf("base = %q", api.cfg.BaseURL)
	}
}

func TestStrangeFilesDecodeWithoutLosingKnownData(t *testing.T) {
	r := newRig(t, serve(fixture(t, "strange_things.json")))

	res, err := r.api.GetFileNodes(context.Background(), "u", "K", []string{"5:1"}, NodesOptions{})
	if err != nil {
		t.Fatal(err)
	}

	board := res.Nodes["5:1"].Document
	if board.Type != NodeSection || board.IsVisible() || board.Opacity == nil || *board.Opacity != 0 {
		t.Fatalf("explicit hidden or zero opacity lost: %#v", board)
	}
	if board.LayoutGrids[0].Count != 12 || board.Styles["fill"] != "S:10" || !strings.Contains(string(board.BoundVariables), "VariableID:1:2") {
		t.Fatalf("grids, style refs or variable bindings lost: %#v", board)
	}
	if board.Fills[0].Type != "FUTURE_PAINT" || board.Fills[1].Color == nil || !strings.Contains(string(board.Fills[1].BoundVariables), "VariableID:9:9") {
		t.Fatalf("paints lost: %#v", board.Fills)
	}
	if board.Effects[0].Type != "NOISE" || board.Effects[1].Type != "FUTURE_EFFECT" || !board.Effects[1].IsVisible() {
		t.Fatalf("effects lost: %#v", board.Effects)
	}

	kids := board.Children
	for i, want := range []string{"TABLE", "STICKY", "CONNECTOR", "TEXT_PATH", "ELLIPSE", "TEXT", "INSTANCE"} {
		if kids[i].Type != want {
			t.Fatalf("child %d type = %q, want %q", i, kids[i].Type, want)
		}
	}
	if kids[0].Known() || kids[1].Known() || kids[2].Known() || kids[3].Known() || !kids[4].Known() {
		t.Fatal("Known() misreports modelled types")
	}
	if kids[0].Children[0].Type != "TABLE_CELL" || kids[1].Characters != "日本語 مرحبا 👋" {
		t.Fatal("nested or unicode data lost")
	}
	if kids[4].ArcData.EndingAngle != 3.14 || kids[4].StrokeCap != "ROUND" || kids[4].StrokeMiterAngle != 28.96 {
		t.Fatalf("shape data lost: %#v", kids[4])
	}
	if kids[5].Style.Hyperlink.URL != "https://example.test" || kids[5].Style.OpentypeFlags["LIGA"] != 0 || len(kids[5].LineTypes) != 1 {
		t.Fatalf("text extras lost: %#v", kids[5].Style)
	}
	if kids[6].Overrides[0].Fields[1] != "characters" || kids[6].AbsoluteBoundingBox.X != -100.5 || kids[6].AbsoluteBoundingBox.Y != 1e6 {
		t.Fatalf("instance data lost: %#v", kids[6])
	}
}

func TestSnapshotReceivesTheExactRawBody(t *testing.T) {
	raw := fixture(t, "strange_things.json")
	r := newRig(t, serve(raw))
	var snap bytes.Buffer

	_, err := r.api.GetFileNodes(context.Background(), "u", "K", []string{"5:1"}, NodesOptions{Snapshot: &snap})

	if err != nil || !bytes.Contains(snap.Bytes(), []byte("someBrandNewProperty")) || !bytes.Contains(snap.Bytes(), []byte("FUTURE_PAINT")) {
		t.Fatalf("snapshot missing unmodelled data (err %v)", err)
	}
	if len(bytes.TrimSpace(snap.Bytes())) != len(bytes.TrimSpace(raw)) {
		t.Fatalf("snapshot is %d bytes, body was %d", snap.Len(), len(raw))
	}
}

func TestSnapshotIgnoresRejectedAttempts(t *testing.T) {
	var n atomic.Int32
	r := newRig(t, func(w http.ResponseWriter, req *http.Request) {
		if n.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"status":401,"err":"Invalid token"}`))
			return
		}
		serve([]byte(`{"name":"ok","document":{"id":"0:0","type":"DOCUMENT","name":"d"}}`))(w, req)
	})
	var snap bytes.Buffer

	_, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{Snapshot: &snap})

	if err != nil || strings.Contains(snap.String(), "Invalid token") || !strings.Contains(snap.String(), `"name":"ok"`) {
		t.Fatalf("err = %v snapshot = %q", err, snap.String())
	}

	failing := newRig(t, status(404))
	var empty bytes.Buffer
	_, _ = failing.api.GetFile(context.Background(), "u", "K", FileOptions{Snapshot: &empty})
	if empty.Len() != 0 {
		t.Fatal("snapshot written for a failed request")
	}
}
