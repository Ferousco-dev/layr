package imports

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ferousco-dev/layr/server/internal/assets"
	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

const svgLogo = `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24"><script>steal()</script><path d="M0 0L24 24"/></svg>`

func box(id, name, typ string, x, y, w, h float64, kids ...figma.Node) figma.Node {
	return figma.Node{ID: id, Name: name, Type: typ, AbsoluteBoundingBox: &figma.Rect{X: x, Y: y, Width: w, Height: h}, Children: kids}
}

func textNode(id, s string, x, y float64) figma.Node {
	n := box(id, s, "TEXT", x, y, 200, 24)
	n.Characters = s
	n.Fills = []figma.Paint{{Type: "SOLID", Color: &figma.Color{R: 0.1, G: 0.1, B: 0.1, A: 1}}}
	return n
}

func instance(id, name, component string, x, y float64) figma.Node {
	n := box(id, name, "INSTANCE", x, y, 120, 40)
	n.ComponentID = component
	return n
}

// saasScreen is one product screen: a gradient frame with shadow, text, shared components, a shared photo and a logo vector.
func saasScreen(id, name string, x float64, shared ...figma.Node) figma.Node {
	kids := []figma.Node{textNode(id+"-t", name, 24, 24)}
	photo := box(id+"-p", "Background", "RECTANGLE", 0, 0, 400, 300)
	photo.Fills = []figma.Paint{{Type: "IMAGE", ImageRef: "bgref", ScaleMode: "FILL"}}
	logo := box(id+"-l", "Logo", "VECTOR", 300, 8, 24, 24)
	kids = append(kids, photo, logo)
	kids = append(kids, shared...)
	f := box(id, name, "FRAME", x, 0, 400, 300, kids...)
	f.Fills = []figma.Paint{{Type: "GRADIENT_LINEAR", GradientStops: []figma.ColorStop{
		{Position: 0, Color: figma.Color{R: 1, A: 1}}, {Position: 1, Color: figma.Color{B: 1, A: 1}},
	}, GradientHandlePositions: []figma.Vector{{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 0, Y: 1}}}}
	f.Effects = []figma.Effect{{Type: "DROP_SHADOW", Visible: &yes, Radius: 8, Color: &figma.Color{A: 0.25}, Offset: &figma.Vector{X: 0, Y: 4}}}
	f.LayoutMode = "VERTICAL"
	f.ItemSpacing = 8
	return f
}

var yes = true

func saasFixture(cdn string) *fakeFigma {
	btn := func(id string) figma.Node { return instance(id, "Button", "5:100", 24, 200) }
	inp := func(id string) figma.Node { return instance(id, "Input", "5:101", 24, 150) }
	side := func(id string) figma.Node { return instance(id, "Sidebar", "5:102", 0, 0) }
	nav := func(id string) figma.Node { return instance(id, "Navbar", "5:103", 0, 0) }

	login := saasScreen("10:1", "Login", 0, inp("10:1a"), btn("10:1b"))
	signup := saasScreen("10:2", "Signup", 450, inp("10:2a"), inp("10:2b"), btn("10:2c"))
	forgot := saasScreen("10:3", "Forgot Password", 900, inp("10:3a"), btn("10:3b"))
	auth := box("10:0", "Authentication", "SECTION", -50, -50, 1400, 400, login, signup, forgot)

	overview := saasScreen("20:1", "Overview", 0, side("20:1a"), nav("20:1b"))
	analytics := saasScreen("20:2", "Analytics", 450, side("20:2a"), nav("20:2b"))
	contacts := saasScreen("20:3", "Contacts", 900, side("20:3a"), nav("20:3b"), btn("20:3c"))
	settings := saasScreen("20:4", "Settings", 1350, side("20:4a"), nav("20:4b"), inp("20:4c"), btn("20:4d"))

	masters := []figma.Node{
		box("5:100", "Button", "COMPONENT", 0, 900, 120, 40),
		box("5:101", "Input", "COMPONENT", 200, 900, 240, 40),
		box("5:102", "Sidebar", "COMPONENT", 500, 900, 200, 800),
		box("5:103", "Navbar", "COMPONENT", 800, 900, 1200, 64),
	}
	file := &figma.File{
		FileInfo: figma.FileInfo{Name: "SaaS Product", Version: "1001"},
		Document: figma.Node{ID: "0:0", Type: figma.NodeDocument, Children: []figma.Node{
			{ID: "0:1", Name: "Authentication", Type: figma.NodeCanvas, Children: []figma.Node{auth}},
			{ID: "0:2", Name: "Dashboard", Type: figma.NodeCanvas, Children: []figma.Node{overview, analytics, contacts, settings}},
			{ID: "0:3", Name: "Shared Components", Type: figma.NodeCanvas, Children: masters},
		}},
	}
	nodes := map[string]figma.Node{"10:0": auth}
	for _, n := range []figma.Node{login, signup, forgot, overview, analytics, contacts, settings} {
		nodes[n.ID] = n
	}
	for _, m := range masters {
		nodes[m.ID] = m
	}
	comps := map[string]figma.ComponentMeta{
		"5:100": {Key: "k1", Name: "Button"}, "5:101": {Key: "k2", Name: "Input"},
		"5:102": {Key: "k3", Name: "Sidebar"}, "5:103": {Key: "k4", Name: "Navbar"},
	}
	renders := map[string]string{}
	for id := range nodes {
		if strings.HasSuffix(id, "-l") {
			renders[id] = cdn + "/logo.svg"
		}
	}
	for _, s := range []string{"10:1", "10:2", "10:3", "20:1", "20:2", "20:3", "20:4"} {
		renders[s+"-l"] = cdn + "/logo.svg"
	}
	return &fakeFigma{
		file: file, nodes: nodes, components: comps,
		renderURL: cdn + "/reference.png", renderMap: renders,
		fills: map[string]string{"bgref": cdn + "/bg.png"},
	}
}

func saasCDN() *httptest.Server {
	png := pngFixture()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/reference.png", "/bg.png":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		case "/logo.svg":
			w.Header().Set("Content-Type", "image/svg+xml")
			_, _ = w.Write([]byte(svgLogo))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

// runSaaS imports the whole fixture through the real asset pipeline and returns the stored artifacts.
func runSaaS(t *testing.T) (r *rig, api *fakeFigma, imp Import, manifest, irBytes []byte, ir *designir.DesignIR) {
	t.Helper()
	cdn := saasCDN()
	t.Cleanup(cdn.Close)
	api = saasFixture(cdn.URL)
	r = newRig(t, api)
	r.svc.SetAssets(assets.NewPipeline(api, assets.NewDownloader(assets.Policy{AllowInsecure: true}, 1<<20), nil,
		assets.Config{MaxAssetBytes: 1 << 20, MaxImportBytes: 4 << 20, Concurrency: 4, MaxAssets: 100}))
	r.svc.SetDesign(DesignConfig{MaxNodes: 10000, MaxBytes: 4 << 20})

	started := start(t, r, ownerA, projectA, fileURL)
	waiting := r.settle(t, started.ID)
	if waiting.Status != StatusAwaitingSelection {
		t.Fatalf("want a selection step for a multi-screen file, got %+v", waiting)
	}
	if _, err := r.svc.Select(context.Background(), ownerA, projectA, started.ID, Selection{All: true}); err != nil {
		t.Fatal(err)
	}
	imp = r.settle(t, started.ID)
	if imp.Status != StatusCompleted {
		t.Fatalf("import = %+v", imp)
	}
	dir, err := r.mgr.Create(imp.ID)
	if err != nil {
		t.Fatal(err)
	}
	if manifest, err = dir.ReadFile(workspace.Assets, "manifest.json"); err != nil {
		t.Fatal(err)
	}
	if irBytes, err = dir.ReadFile(workspace.Design, "design-ir.json"); err != nil {
		t.Fatal(err)
	}
	if ir, err = designir.Unmarshal(irBytes, designir.DefaultLimits); err != nil {
		t.Fatal(err)
	}
	return
}

func TestSaaSProductImportsEndToEndWithEveryInvariant(t *testing.T) {
	r, api, imp, manifestBytes, irBytes, ir := runSaaS(t)

	if imp.ScreenCount != 11 || len(ir.Screens) != 11 || len(ir.Sections) != 1 || ir.Sections[0].Name != "Authentication" || len(ir.Sections[0].ScreenIDs) != 3 {
		t.Fatalf("screens = %d sections = %+v", len(ir.Screens), ir.Sections)
	}
	want := map[string]int{"Button": 5, "Input": 5, "Sidebar": 4, "Navbar": 4}
	if len(ir.Components) != len(want) {
		t.Fatalf("components = %d", len(ir.Components))
	}
	for _, c := range ir.Components {
		if want[c.Name] != c.InstanceCount {
			t.Errorf("%s instances = %d, want %d", c.Name, c.InstanceCount, want[c.Name])
		}
	}

	dir, _ := r.mgr.Create(imp.ID)
	m, err := assets.LoadManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(m.References) != 11 {
		t.Fatalf("reference renders = %d, want one per screen", len(m.References))
	}
	images, vectors := 0, 0
	for _, a := range m.Assets {
		if a.Kind == "image" {
			images++
		} else {
			vectors++
		}
		data, err := dir.ReadFile(workspace.Assets, strings.TrimPrefix(a.Path, "assets/"))
		if err != nil {
			t.Fatalf("manifest entry %s points at a missing file: %v", a.ID, err)
		}
		sum := sha256.Sum256(data)
		if hex.EncodeToString(sum[:]) != a.SHA256 {
			t.Fatalf("asset %s checksum does not match its file", a.ID)
		}
		if strings.Contains(string(data), "<script") {
			t.Fatalf("asset %s kept active content", a.ID)
		}
	}
	if images != 1 {
		t.Fatalf("the shared background must be one asset, got %d images", images)
	}
	if vectors < 1 {
		t.Fatalf("logo vectors were not stored")
	}

	ids := map[string]bool{}
	for _, a := range ir.Assets {
		ids[a.ID] = true
		if _, ok := m.AssetByID(a.ID); !ok {
			t.Fatalf("IR asset %s is not in the manifest", a.ID)
		}
	}
	for _, s := range ir.Screens {
		for _, id := range s.AssetIDs {
			if !ids[id] {
				t.Fatalf("screen %s references unknown asset %s", s.Name, id)
			}
		}
	}
	for _, blob := range [][]byte{manifestBytes, irBytes} {
		text := string(blob)
		for _, bad := range []string{"http://", "https://", r.root, "SECRET", "access_token"} {
			if strings.Contains(text, bad) {
				t.Fatalf("stored artifact contains %q", bad)
			}
		}
	}

	if api.renders.Load() > 8 {
		t.Fatalf("Figma render requests = %d; references must be batched (N+1 check)", api.renders.Load())
	}
	if api.calls.Load() > 10 {
		t.Fatalf("Figma calls = %d for 11 screens; the design must be fetched once and worked on locally", api.calls.Load())
	}

	analytics := ""
	for _, sc := range ir.Screens {
		if sc.Name == "Analytics" {
			analytics = sc.ID
		}
	}
	one, err := ir.Select([]string{analytics}, designir.DefaultLimits)
	if err != nil || len(one.Screens) != 1 || one.Screens[0].Name != "Analytics" {
		t.Fatalf("selecting one screen: %v", err)
	}
}

func TestSaaSProductOutputIsReproducibleAcrossRuns(t *testing.T) {
	_, _, _, m1, _, ir1 := runSaaS(t)
	_, _, _, m2, _, ir2 := runSaaS(t)

	if sha(m1) != sha(m2) {
		t.Fatal("asset manifest differs between identical runs")
	}
	ir1.Source.ImportID, ir2.Source.ImportID = "", ""
	a, err := designir.Marshal(ir1, designir.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	b, err := designir.Marshal(ir2, designir.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	if sha(a) != sha(b) {
		t.Fatal("Design IR differs between identical runs (apart from the import id)")
	}
}

func sha(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
