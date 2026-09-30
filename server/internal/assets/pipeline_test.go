package assets

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

const importID = "11111111-1111-4111-8111-111111111111"

// route is one canned CDN response.
type route struct {
	status int
	ctype  string
	body   []byte
	hits   atomic.Int32
	block  chan struct{}
}

type cdn struct {
	srv    *httptest.Server
	mu     sync.Mutex
	routes map[string]*route
}

func newCDN(t *testing.T) *cdn {
	c := &cdn{routes: map[string]*route{}}
	c.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		rt := c.routes[r.URL.Path]
		c.mu.Unlock()
		if rt == nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		rt.hits.Add(1)
		if rt.block != nil {
			select {
			case <-rt.block:
			case <-r.Context().Done():
				return
			}
		}
		if rt.ctype != "" {
			w.Header().Set("Content-Type", rt.ctype)
		}
		if rt.status != 0 {
			w.WriteHeader(rt.status)
		}
		_, _ = w.Write(rt.body)
	}))
	t.Cleanup(c.srv.Close)
	return c
}

func (c *cdn) serve(path, ctype string, body []byte) *route {
	rt := &route{ctype: ctype, body: body}
	c.mu.Lock()
	c.routes[path] = rt
	c.mu.Unlock()
	return rt
}

func (c *cdn) url(path string) string { return c.srv.URL + path }

type fakeAPI struct {
	fills       map[string]string
	renders     map[string]string
	fillCalls   atomic.Int32
	renderCalls atomic.Int32
	fillsErr    error
	renderErr   error
	refreshed   map[string]string
}

func (f *fakeAPI) GetImageFills(context.Context, string, string) (map[string]string, error) {
	n := f.fillCalls.Add(1)
	if f.fillsErr != nil {
		return nil, f.fillsErr
	}
	if n > 1 && f.refreshed != nil {
		return f.refreshed, nil
	}
	return f.fills, nil
}

func (f *fakeAPI) RenderNodes(_ context.Context, _, _ string, ids []string, opts figma.RenderOptions) (*figma.Renders, error) {
	n := f.renderCalls.Add(1)
	if f.renderErr != nil {
		return nil, f.renderErr
	}
	src := f.renders
	if n > 1 && f.refreshed != nil {
		src = f.refreshed
	}
	out := &figma.Renders{URLs: map[string]string{}}
	for _, id := range ids {
		if u := src[id]; u != "" {
			out.URLs[id] = u
		} else {
			out.Failed = append(out.Failed, id)
		}
	}
	return out, nil
}

type rig struct {
	t     *testing.T
	cdn   *cdn
	api   *fakeAPI
	mgr   *workspace.Manager
	dir   *workspace.Dir
	root  string
	pipe  *Pipeline
	sleep []time.Duration
}

func newRig(t *testing.T, mutate ...func(*Config)) *rig {
	t.Helper()
	root := t.TempDir()
	mgr, err := workspace.NewManager(root, 8<<20, 32<<20)
	if err != nil {
		t.Fatal(err)
	}
	dir, err := mgr.Create(importID)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{MaxAssetBytes: 1 << 20, MaxImportBytes: 8 << 20, Concurrency: 4, MaxAssets: 300}
	for _, m := range mutate {
		m(&cfg)
	}
	r := &rig{t: t, cdn: newCDN(t), api: &fakeAPI{fills: map[string]string{}, renders: map[string]string{}}, mgr: mgr, dir: dir, root: root}
	r.pipe = NewPipeline(r.api, NewDownloader(Policy{AllowInsecure: true}, cfg.MaxAssetBytes), nil, cfg)
	r.pipe.sleep = func(ctx context.Context, d time.Duration) error { r.sleep = append(r.sleep, d); return ctx.Err() }
	return r
}

func (r *rig) input(root figma.Node) Input {
	return Input{ImportID: importID, UserID: "user-1", FileKey: "FILEKEY123456", NodeIDs: []string{"1:1"}, Root: root, Dir: r.dir}
}

func (r *rig) manifest() *Manifest {
	r.t.Helper()
	m, err := LoadManifest(r.dir)
	if err != nil {
		r.t.Fatal(err)
	}
	return m
}

func (r *rig) assetFiles() []string {
	names, _ := r.dir.Names(workspace.Assets)
	return names
}

func svgOf(marker string) []byte {
	return []byte(`<svg width="24" height="24" viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg"><path d="M` + marker + `" fill="#000"/></svg>`)
}

// fullDesign serves every asset of the mixed_assets fixture.
func fullDesign(t *testing.T, r *rig) figma.Node {
	t.Helper()
	root := loadFixture(t, "mixed_assets.json")
	r.cdn.serve("/hero.png", "image/png", pngBytes(t, 4, 2))
	r.cdn.serve("/avatar.jpg", "image/jpeg", jpegBytes(t, 8, 6))
	r.cdn.serve("/gif.gif", "image/gif", gifBytes(t, 3, 3))
	r.api.fills = map[string]string{"hero111": r.cdn.url("/hero.png"), "avatar22": r.cdn.url("/avatar.jpg"), "gifref33": r.cdn.url("/gif.gif")}
	same := svgOf("0 0")
	r.cdn.serve("/arrow.svg", "image/svg+xml", same)
	r.cdn.serve("/plus.svg", "image/svg+xml", same)
	r.cdn.serve("/logo.svg", "image/svg+xml", svgOf("1 1"))
	r.cdn.serve("/illustration.svg", "image/svg+xml", svgOf("2 2"))
	r.cdn.serve("/marked.svg", "image/svg+xml", svgOf("3 3"))
	r.cdn.serve("/future.svg", "image/svg+xml", svgOf("4 4"))
	r.api.renders = map[string]string{
		"3:3": r.cdn.url("/arrow.svg"), "I2:6;9:2": r.cdn.url("/plus.svg"), "2:4": r.cdn.url("/logo.svg"),
		"2:5": r.cdn.url("/illustration.svg"), "2:12": r.cdn.url("/marked.svg"), "13:1": r.cdn.url("/future.svg"),
	}
	return root
}

func TestRunStoresImagesAndSVGsWithAManifest(t *testing.T) {
	r := newRig(t)
	root := fullDesign(t, r)

	sum, err := r.pipe.Run(context.Background(), r.input(root))
	if err != nil {
		t.Fatal(err)
	}

	m := r.manifest()
	if m.SchemaVersion != 1 || m.ImportID != importID || m.Source.FileKey != "FILEKEY123456" || sum.Assets != len(m.Assets) {
		t.Fatalf("manifest header = %+v summary %+v", m, sum)
	}
	// 3 raster images + 5 distinct SVGs (arrow and plus have identical bytes and share one file).
	if len(m.Assets) != 8 {
		t.Fatalf("assets = %d: %+v", len(m.Assets), m.Assets)
	}

	hero, ok := m.AssetForImageRef("hero111")
	if !ok || hero.Kind != "image" || hero.Format != "png" || hero.MediaType != "image/png" || !strings.HasPrefix(hero.Path, "assets/hero-image-") ||
		!strings.HasSuffix(hero.Path, ".png") || *hero.Width != 4 || *hero.Height != 2 {
		t.Fatalf("hero = %+v", hero)
	}
	if len(hero.Sources) != 2 || hero.Sources[0].NodeID != "2:1" || hero.Sources[0].ScaleMode != "FILL" || hero.Sources[1].NodeID != "2:2" || hero.Sources[1].ScaleMode != "FIT" || hero.Sources[1].Rotation != 90 {
		t.Fatalf("usage lost: %+v", hero.Sources)
	}
	if rt := r.cdn.routes["/hero.png"]; rt.hits.Load() != 1 {
		t.Fatalf("hero image downloaded %d times for two uses", rt.hits.Load())
	}

	avatar, _ := m.AssetForImageRef("avatar22")
	if avatar.Format != "jpeg" || !strings.HasSuffix(avatar.Path, ".jpg") || len(avatar.Sources) != 2 {
		t.Fatalf("avatar = %+v", avatar)
	}

	arrow := m.AssetsForNode("3:3")
	plus := m.AssetsForNode("I2:6;9:2")
	if len(arrow) != 1 || len(plus) != 1 || arrow[0].ID != plus[0].ID || len(arrow[0].Sources) != 2 {
		t.Fatalf("identical svgs must share one asset while keeping both sources: %+v %+v", arrow, plus)
	}
	if arrow[0].Kind != "svg" || arrow[0].MediaType != "image/svg+xml" || *arrow[0].Width != 24 {
		t.Fatalf("svg = %+v", arrow[0])
	}

	for _, a := range m.Assets {
		content, err := r.dir.ReadFile(workspace.Assets, strings.TrimPrefix(a.Path, "assets/"))
		if err != nil {
			t.Fatalf("%s: %v", a.Path, err)
		}
		sum := sha256.Sum256(content)
		if hex.EncodeToString(sum[:]) != a.SHA256 || int64(len(content)) != a.SizeBytes || a.ID != assetID(a.SHA256) {
			t.Fatalf("%s: metadata does not match the stored bytes", a.Path)
		}
		if got, ok := m.AssetByID(a.ID); !ok || got.Path != a.Path {
			t.Fatalf("lookup by id failed for %s", a.Path)
		}
	}
	if len(r.assetFiles()) != 9 { // 8 assets + manifest
		t.Fatalf("files = %v", r.assetFiles())
	}
}

func TestManifestHoldsNothingSensitiveAndIsSorted(t *testing.T) {
	r := newRig(t)
	root := fullDesign(t, r)
	_, _ = r.pipe.Run(context.Background(), r.input(root))

	raw, _ := r.dir.ReadFile(workspace.Assets, "manifest.json")
	text := string(raw)
	for _, forbidden := range []string{"http://", "https://", r.cdn.srv.URL, r.root, "127.0.0.1", "Bearer", "token", "Signature"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("manifest contains %q", forbidden)
		}
	}
	var m Manifest
	_ = json.Unmarshal(raw, &m)
	for i := 1; i < len(m.Assets); i++ {
		if m.Assets[i-1].Path >= m.Assets[i].Path {
			t.Fatalf("assets not sorted by path: %s >= %s", m.Assets[i-1].Path, m.Assets[i].Path)
		}
	}
	for _, a := range m.Assets {
		if !strings.HasPrefix(a.Path, "assets/") || strings.Contains(a.Path, "..") || filepath.IsAbs(a.Path) {
			t.Fatalf("path %q is not a portable relative path", a.Path)
		}
	}
}

func TestOutputDoesNotDependOnConcurrency(t *testing.T) {
	var outputs []string
	for _, workers := range []int{1, 8} {
		r := newRig(t, func(c *Config) { c.Concurrency = workers })
		root := fullDesign(t, r)
		if _, err := r.pipe.Run(context.Background(), r.input(root)); err != nil {
			t.Fatal(err)
		}
		raw, _ := r.dir.ReadFile(workspace.Assets, "manifest.json")
		outputs = append(outputs, string(raw))
	}
	if outputs[0] != outputs[1] {
		t.Fatal("manifest differs between 1 and 8 workers")
	}
}

func TestWarningsForUnsupportedAndMissingMedia(t *testing.T) {
	r := newRig(t)
	root := fullDesign(t, r)
	delete(r.api.fills, "gifref33")
	delete(r.api.renders, "2:5")

	if _, err := r.pipe.Run(context.Background(), r.input(root)); err != nil {
		t.Fatalf("missing optional assets must not fail the import: %v", err)
	}

	codes := map[string]bool{}
	for _, w := range r.manifest().Warnings {
		codes[w.Code+"@"+w.NodeID] = true
	}
	for _, want := range []string{
		WarnVideoUnsupported + "@2:10", WarnPatternUnsupported + "@2:11", WarnImageUnresolved + "@2:15", WarnVectorUnresolved + "@2:5",
	} {
		if !codes[want] {
			t.Errorf("missing warning %s in %v", want, codes)
		}
	}
}

func TestSecondRunReusesVerifiedAssetsAndRepairsCorruptOnes(t *testing.T) {
	r := newRig(t)
	root := fullDesign(t, r)
	if _, err := r.pipe.Run(context.Background(), r.input(root)); err != nil {
		t.Fatal(err)
	}
	first := r.manifest()
	fillCalls, renderCalls := r.api.fillCalls.Load(), r.api.renderCalls.Load()
	heroHits := r.cdn.routes["/hero.png"].hits.Load()

	if _, err := r.pipe.Run(context.Background(), r.input(root)); err != nil {
		t.Fatal(err)
	}
	if r.api.fillCalls.Load() != fillCalls || r.api.renderCalls.Load() != renderCalls || r.cdn.routes["/hero.png"].hits.Load() != heroHits {
		t.Fatal("a rerun fetched assets that were already stored and intact")
	}

	hero, _ := first.AssetForImageRef("hero111")
	path := filepath.Join(r.dir.Path(), hero.Path)
	if err := os.WriteFile(path, []byte("corrupted"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := r.pipe.Run(context.Background(), r.input(root)); err != nil {
		t.Fatal(err)
	}
	if r.cdn.routes["/hero.png"].hits.Load() != heroHits+1 {
		t.Fatal("corrupt asset was not downloaded again")
	}
	repaired := r.manifest()
	got, _ := repaired.AssetForImageRef("hero111")
	if got.SHA256 != hero.SHA256 || !verifyStored(r.dir, got) || len(repaired.Assets) != len(first.Assets) {
		t.Fatalf("repaired asset differs: %+v", got)
	}
}

func TestStaleFilesAreRemovedAndNothingDuplicates(t *testing.T) {
	r := newRig(t)
	root := fullDesign(t, r)
	_ = r.dir.WriteBytes(workspace.Assets, "leftover-abc123.png", []byte("old"))

	if _, err := r.pipe.Run(context.Background(), r.input(root)); err != nil {
		t.Fatal(err)
	}

	for _, n := range r.assetFiles() {
		if n == "leftover-abc123.png" {
			t.Fatal("stale file survived")
		}
	}
}

func TestSameNameDifferentContentGetsDistinctFiles(t *testing.T) {
	r := newRig(t)
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame, Children: []figma.Node{
		{ID: "2:1", Name: "Icon", Type: figma.NodeVector},
		{ID: "2:2", Name: "Icon", Type: figma.NodeVector},
		{ID: "2:3", Name: "Icon", Type: figma.NodeVector},
	}}
	for i, id := range []string{"2:1", "2:2", "2:3"} {
		path := "/icon" + id + ".svg"
		r.cdn.serve(path, "image/svg+xml", svgOf(strings.Repeat("1", i+1)))
		r.api.renders[id] = r.cdn.url(path)
	}

	if _, err := r.pipe.Run(context.Background(), r.input(root)); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	for _, a := range r.manifest().Assets {
		if !strings.HasPrefix(a.Path, "assets/icon-") || seen[a.Path] {
			t.Fatalf("bad or duplicate path %q", a.Path)
		}
		seen[a.Path] = true
	}
	if len(seen) != 3 {
		t.Fatalf("paths = %v", seen)
	}
}

func TestContentValidation(t *testing.T) {
	cases := []struct {
		name  string
		ctype string
		body  []byte
		ok    bool
		want  string
	}{
		{"correct type and bytes", "image/png", pngBytes(t, 2, 2), true, "png"},
		{"wrong image type, real bytes win", "image/jpeg", pngBytes(t, 2, 2), true, "png"},
		{"missing type", "", pngBytes(t, 2, 2), true, "png"},
		{"generic type", "application/octet-stream", jpegBytes(t, 2, 2), true, "jpeg"},
		{"png type, html bytes", "image/png", []byte("<html><script>alert(1)</script></html>"), false, ""},
		{"png type, php bytes", "image/png", []byte("<?php echo 1;"), false, ""},
		{"html type, png bytes", "text/html", pngBytes(t, 2, 2), false, ""},
		{"json type", "application/json", []byte(`{}`), false, ""},
		{"truncated png", "image/png", pngBytes(t, 2, 2)[:20], false, ""},
		{"empty", "image/png", nil, false, ""},
		{"svg type, png bytes", "image/svg+xml", pngBytes(t, 2, 2), true, "png"},
		{"svg bytes", "image/svg+xml", svgOf("5 5"), true, "svg"},
		{"svg type, html bytes", "image/svg+xml", []byte("<html></html>"), false, ""},
		{"svg without content type", "", svgOf("6 6"), true, "svg"},
		{"webp", "image/webp", webpVP8X(10, 20), true, "webp"},
	}
	for _, tc := range cases {
		r := newRig(t)
		r.cdn.serve("/a", tc.ctype, tc.body)
		r.api.fills = map[string]string{"ref": r.cdn.url("/a")}
		root := figma.Node{ID: "1:1", Type: figma.NodeFrame, Children: []figma.Node{
			{ID: "2:1", Name: "Pic", Type: figma.NodeRectangle, Fills: []figma.Paint{{Type: "IMAGE", ImageRef: "ref"}}},
		}}

		_, err := r.pipe.Run(context.Background(), r.input(root))

		if tc.ok {
			if err != nil {
				t.Errorf("%s: unexpected error %v", tc.name, err)
				continue
			}
			m := r.manifest()
			if err != nil || len(m.Assets) != 1 || m.Assets[0].Format != tc.want || !strings.HasSuffix(m.Assets[0].Path, "."+ext(tc.want)) {
				t.Errorf("%s: err %v manifest %+v", tc.name, err, m)
			}
			continue
		}
		if CodeOf(err) != CodeInvalidContent {
			t.Errorf("%s: err = %v, want %s", tc.name, err, CodeInvalidContent)
		}
		if names := r.assetFiles(); len(names) != 0 {
			t.Errorf("%s: files left behind: %v", tc.name, names)
		}
	}
}

func ext(format string) string {
	if format == "jpeg" {
		return "jpg"
	}
	return format
}

func TestMaliciousSVGIsSanitizedBeforeStorage(t *testing.T) {
	r := newRig(t)
	evil := `<svg xmlns="http://www.w3.org/2000/svg" onload="steal()"><script>alert(1)</script><image href="https://evil.example/x.png"/><path d="M9 9"/></svg>`
	r.cdn.serve("/evil.svg", "image/svg+xml", []byte(evil))
	r.api.renders = map[string]string{"2:1": r.cdn.url("/evil.svg")}
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame, Children: []figma.Node{{ID: "2:1", Name: "Logo", Type: figma.NodeVector}}}

	if _, err := r.pipe.Run(context.Background(), r.input(root)); err != nil {
		t.Fatal(err)
	}

	m := r.manifest()
	stored, _ := r.dir.ReadFile(workspace.Assets, strings.TrimPrefix(m.Assets[0].Path, "assets/"))
	for _, bad := range []string{"script", "onload", "evil.example", "alert"} {
		if strings.Contains(string(stored), bad) {
			t.Fatalf("stored svg still contains %q: %s", bad, stored)
		}
	}
	if !m.Assets[0].Sanitized || !strings.Contains(string(stored), `d="M9 9"`) || m.Assets[0].SHA256 != sumHex(stored) {
		t.Fatalf("asset = %+v stored %s", m.Assets[0], stored)
	}
	found := false
	for _, w := range m.Warnings {
		found = found || w.Code == WarnSVGSanitized
	}
	if !found {
		t.Fatal("sanitization not reported")
	}
}

func TestSizeLimitAbortsAndLeavesNothing(t *testing.T) {
	r := newRig(t, func(c *Config) { c.MaxAssetBytes = 4096 })
	r.cdn.serve("/big", "image/png", append(pngBytes(t, 2, 2), make([]byte, 100000)...))
	r.api.fills = map[string]string{"ref": r.cdn.url("/big")}
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame, Children: []figma.Node{
		{ID: "2:1", Type: figma.NodeRectangle, Fills: []figma.Paint{{Type: "IMAGE", ImageRef: "ref"}}},
	}}

	_, err := r.pipe.Run(context.Background(), r.input(root))

	if CodeOf(err) != CodeTooLarge || len(r.assetFiles()) != 0 {
		t.Fatalf("err = %v files = %v", err, r.assetFiles())
	}
	entries, _ := os.ReadDir(filepath.Join(r.dir.Path(), workspace.Assets))
	if len(entries) != 0 {
		t.Fatalf("temporary file left behind: %d entries", len(entries))
	}
	if _, err := LoadManifest(r.dir); err == nil {
		t.Fatal("a manifest was written for a failed run")
	}
}

func TestImportBudgetStopsRunawayTotals(t *testing.T) {
	r := newRig(t, func(c *Config) { c.MaxImportBytes = 3000 })
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame}
	for i := 0; i < 4; i++ {
		id := string(rune('a' + i))
		body := append(pngBytes(t, 2+i, 2), make([]byte, 1200)...)
		r.cdn.serve("/"+id, "image/png", body)
		r.api.fills[id] = r.cdn.url("/" + id)
		root.Children = append(root.Children, figma.Node{ID: "2:" + id, Type: figma.NodeRectangle, Fills: []figma.Paint{{Type: "IMAGE", ImageRef: id}}})
	}

	_, err := r.pipe.Run(context.Background(), r.input(root))

	if CodeOf(err) != CodeBudgetExceeded {
		t.Fatalf("err = %v", err)
	}
}

func TestTransientFailuresRetryAndPermanentOnesDoNot(t *testing.T) {
	r := newRig(t)
	var n atomic.Int32
	png := pngBytes(t, 2, 2)
	r.cdn.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/flaky":
			if n.Add(1) < 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		case "/bad":
			n.Add(1)
			w.WriteHeader(http.StatusBadRequest)
		}
	})
	r.api.fills = map[string]string{"flaky": r.cdn.url("/flaky")}
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame, Children: []figma.Node{
		{ID: "2:1", Type: figma.NodeRectangle, Fills: []figma.Paint{{Type: "IMAGE", ImageRef: "flaky"}}},
	}}

	if _, err := r.pipe.Run(context.Background(), r.input(root)); err != nil {
		t.Fatalf("flaky download: %v", err)
	}
	if n.Load() != 3 || len(r.sleep) != 2 || r.sleep[1] <= r.sleep[0] {
		t.Fatalf("attempts %d backoff %v", n.Load(), r.sleep)
	}

	n.Store(0)
	r2 := newRig(t)
	r2.cdn.srv.Config.Handler = r.cdn.srv.Config.Handler
	r2.api.fills = map[string]string{"bad": r.cdn.url("/bad")}
	root.Children[0].Fills[0].ImageRef = "bad"
	_, err := r2.pipe.Run(context.Background(), r2.input(root))
	if CodeOf(err) != CodeDownloadFailed || n.Load() != 1 {
		t.Fatalf("err %v, attempts %d (a 400 must not be retried)", err, n.Load())
	}
}

func TestExpiredURLIsReResolvedOnceAndBounded(t *testing.T) {
	r := newRig(t)
	r.cdn.serve("/old", "", nil).status = http.StatusForbidden
	r.cdn.serve("/new", "image/png", pngBytes(t, 3, 3))
	r.api.fills = map[string]string{"ref": r.cdn.url("/old")}
	r.api.refreshed = map[string]string{"ref": r.cdn.url("/new")}
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame, Children: []figma.Node{
		{ID: "2:1", Type: figma.NodeRectangle, Fills: []figma.Paint{{Type: "IMAGE", ImageRef: "ref"}}},
	}}

	if _, err := r.pipe.Run(context.Background(), r.input(root)); err != nil {
		t.Fatal(err)
	}
	if r.api.fillCalls.Load() != 2 || r.cdn.routes["/new"].hits.Load() != 1 {
		t.Fatalf("fill lookups %d", r.api.fillCalls.Load())
	}

	still := newRig(t)
	still.cdn.serve("/old", "", nil).status = http.StatusForbidden
	still.api.fills = map[string]string{"ref": still.cdn.url("/old")}
	still.api.refreshed = map[string]string{"ref": still.cdn.url("/old")}
	_, err := still.pipe.Run(context.Background(), still.input(root))
	if CodeOf(err) != CodeDownloadFailed || still.api.fillCalls.Load() != 2 || still.cdn.routes["/old"].hits.Load() != 2 {
		t.Fatalf("err %v lookups %d hits %d: re-resolution must happen once, not loop", err, still.api.fillCalls.Load(), still.cdn.routes["/old"].hits.Load())
	}
}

func TestUnsafeURLFromTheProviderIsRefused(t *testing.T) {
	r := newRig(t)
	r.pipe.dl = NewDownloader(Policy{}, 1<<20)
	r.api.fills = map[string]string{"ref": "https://169.254.169.254/latest/meta-data"}
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame, Children: []figma.Node{
		{ID: "2:1", Type: figma.NodeRectangle, Fills: []figma.Paint{{Type: "IMAGE", ImageRef: "ref"}}},
	}}

	_, err := r.pipe.Run(context.Background(), r.input(root))

	if CodeOf(err) != CodeURLBlocked {
		t.Fatalf("err = %v", err)
	}
}

func TestOneFatalFailureStopsTheSiblings(t *testing.T) {
	r := newRig(t, func(c *Config) { c.Concurrency = 2 })
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame}
	r.cdn.serve("/broken", "image/png", []byte("not an image"))
	slow := r.cdn.serve("/slow", "image/png", pngBytes(t, 2, 2))
	slow.block = make(chan struct{})
	defer close(slow.block)
	r.api.fills = map[string]string{"broken": r.cdn.url("/broken"), "slow": r.cdn.url("/slow")}
	for _, ref := range []string{"broken", "slow"} {
		root.Children = append(root.Children, figma.Node{ID: "2:" + ref, Type: figma.NodeRectangle, Fills: []figma.Paint{{Type: "IMAGE", ImageRef: ref}}})
	}

	done := make(chan error, 1)
	go func() { _, err := r.pipe.Run(context.Background(), r.input(root)); done <- err }()

	select {
	case err := <-done:
		if CodeOf(err) != CodeInvalidContent {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("the pipeline waited for a sibling after a fatal error")
	}
	if len(r.assetFiles()) != 0 {
		t.Fatalf("files = %v", r.assetFiles())
	}
}

func TestCancellationStopsDownloadsAndCleansUp(t *testing.T) {
	r := newRig(t)
	rt := r.cdn.serve("/slow", "image/png", pngBytes(t, 2, 2))
	rt.block = make(chan struct{})
	defer close(rt.block)
	r.api.fills = map[string]string{"ref": r.cdn.url("/slow")}
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame, Children: []figma.Node{
		{ID: "2:1", Type: figma.NodeRectangle, Fills: []figma.Paint{{Type: "IMAGE", ImageRef: "ref"}}},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := r.pipe.Run(ctx, r.input(root)); done <- err }()
	waitFor(t, func() bool { return rt.hits.Load() > 0 })

	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("pipeline did not stop after cancellation")
	}
	entries, _ := os.ReadDir(filepath.Join(r.dir.Path(), workspace.Assets))
	if len(entries) != 0 {
		t.Fatalf("leftovers after cancellation: %d entries", len(entries))
	}
}

func TestFigmaFailuresPropagateUntouched(t *testing.T) {
	r := newRig(t)
	r.api.fillsErr = &figma.Error{Kind: figma.KindRateLimited, RetryAfter: time.Minute}
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame, Children: []figma.Node{
		{ID: "2:1", Type: figma.NodeRectangle, Fills: []figma.Paint{{Type: "IMAGE", ImageRef: "ref"}}},
	}}

	_, err := r.pipe.Run(context.Background(), r.input(root))

	if figma.KindOf(err) != figma.KindRateLimited {
		t.Fatalf("err = %v", err)
	}
}

func TestAssetLimitKeepsImagesFirstAndWarns(t *testing.T) {
	r := newRig(t, func(c *Config) { c.MaxAssets = 2 })
	root := fullDesign(t, r)

	if _, err := r.pipe.Run(context.Background(), r.input(root)); err != nil {
		t.Fatal(err)
	}

	m := r.manifest()
	warned := false
	for _, w := range m.Warnings {
		warned = warned || w.Code == WarnTooManyAssets
	}
	if len(m.Assets) > 2 || !warned {
		t.Fatalf("assets %d warned %v", len(m.Assets), warned)
	}
	if _, ok := m.AssetForImageRef("hero111"); !ok {
		t.Fatal("real images must be kept before vectors")
	}
}

func TestReferenceRendersAreSavedApartFromAssets(t *testing.T) {
	r := newRig(t)
	one, two := pngBytes(t, 10, 6), pngBytes(t, 20, 12)
	r.cdn.serve("/one.png", "image/png", one)
	r.cdn.serve("/two.png", "image/png", two)

	refs, err := r.pipe.SaveReferences(context.Background(), r.dir, "u", "FILEKEY123456", map[string]string{"2:2": r.cdn.url("/two.png"), "1:1": r.cdn.url("/one.png")})
	if err != nil {
		t.Fatal(err)
	}

	if len(refs) != 2 || refs[0].NodeID != "1:1" || refs[1].NodeID != "2:2" {
		t.Fatalf("refs = %+v", refs)
	}
	for i, want := range [][]byte{one, two} {
		name := strings.TrimPrefix(refs[i].Path, "reference/")
		stored, err := r.dir.ReadFile(workspace.Reference, name)
		if err != nil || !bytes.Equal(stored, want) || refs[i].SHA256 != sumHex(want) || refs[i].SizeBytes != int64(len(want)) {
			t.Fatalf("reference %d not stored correctly: %v", i, err)
		}
		if !strings.HasPrefix(refs[i].Path, "reference/") || strings.Contains(refs[i].Path, ":") {
			t.Fatalf("unsafe reference path %q", refs[i].Path)
		}
	}
	if *refs[0].Width != 10 || *refs[1].Height != 12 {
		t.Fatalf("dimensions: %+v", refs)
	}
	if len(r.assetFiles()) != 0 {
		t.Fatal("a reference image leaked into assets/")
	}

	in := r.input(figma.Node{ID: "1:1", Type: figma.NodeFrame})
	in.NodeIDs = []string{"1:1", "2:2"}
	in.References = refs
	if _, err := r.pipe.Run(context.Background(), in); err != nil {
		t.Fatal(err)
	}
	m := r.manifest()
	if len(m.References) != 2 || len(m.Assets) != 0 || m.Source.NodeID != "1:1" || len(m.Source.NodeIDs) != 2 {
		t.Fatalf("manifest = %+v", m)
	}
}

func TestReferenceFilesAreUniqueAndSafe(t *testing.T) {
	seen := map[string]string{}
	for _, id := range []string{"1:2", "1-2", "12:3", "I5:1;2:3", "../../etc", "", strings.Repeat("9:", 80)} {
		name := referenceFile(id)
		if !safeName.MatchString(name) || !strings.HasSuffix(name, ".png") {
			t.Errorf("%q -> unsafe %q", id, name)
		}
		if other, dup := seen[name]; dup {
			t.Errorf("%q and %q share %q", id, other, name)
		}
		seen[name] = id
	}
}

func TestReferenceMustBeAPNGAndReResolvesExpiredURLs(t *testing.T) {
	r := newRig(t)
	r.cdn.serve("/jpeg", "image/jpeg", jpegBytes(t, 4, 4))
	if _, err := r.pipe.SaveReferences(context.Background(), r.dir, "u", "K", map[string]string{"1:1": r.cdn.url("/jpeg")}); CodeOf(err) != CodeInvalidContent {
		t.Fatalf("non-png reference: %v", err)
	}
	if names, _ := r.dir.Names(workspace.Reference); len(names) != 0 {
		t.Fatalf("rejected reference was stored: %v", names)
	}

	r.cdn.serve("/expired", "", nil).status = http.StatusForbidden
	r.cdn.serve("/fresh", "image/png", pngBytes(t, 2, 2))
	r.api.refreshed = map[string]string{"1:1": r.cdn.url("/fresh")}
	r.api.renders = map[string]string{"1:1": r.cdn.url("/fresh")}
	r.api.renderCalls.Store(1)
	if _, err := r.pipe.SaveReferences(context.Background(), r.dir, "u", "K", map[string]string{"1:1": r.cdn.url("/expired")}); err != nil {
		t.Fatalf("expired reference URL: %v", err)
	}
}

func TestConcurrentRunsUnderTheRaceDetector(t *testing.T) {
	r := newRig(t, func(c *Config) { c.Concurrency = 8 })
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame}
	for i := 0; i < 40; i++ {
		id := strings.Repeat("x", 1) + string(rune('A'+i%26)) + string(rune('a'+i/26))
		r.cdn.serve("/"+id, "image/png", pngBytes(t, 2+i, 3))
		r.api.fills[id] = r.cdn.url("/" + id)
		root.Children = append(root.Children, figma.Node{ID: "2:" + id, Type: figma.NodeRectangle, Fills: []figma.Paint{{Type: "IMAGE", ImageRef: id}}})
	}

	if _, err := r.pipe.Run(context.Background(), r.input(root)); err != nil {
		t.Fatal(err)
	}

	m := r.manifest()
	if len(m.Assets) != 40 || len(r.assetFiles()) != 41 {
		t.Fatalf("assets %d files %d", len(m.Assets), len(r.assetFiles()))
	}
	for _, a := range m.Assets {
		if !verifyStored(r.dir, a) {
			t.Fatalf("%s failed verification", a.Path)
		}
	}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not reached")
}

var _ = io.Discard
