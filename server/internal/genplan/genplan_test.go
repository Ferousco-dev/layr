package genplan

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/designir"
)

func nd(id, name, typ string, children ...designir.Node) designir.Node {
	return designir.Node{
		ID: "n_" + id, Source: designir.SourceRef{Provider: "figma", NodeID: id, Type: "FRAME"}, Name: name, Type: typ, Visible: true,
		Geometry: designir.Geometry{Width: 100, Height: 100}, Children: children,
	}
}

func inst(id, comp string, children ...designir.Node) designir.Node {
	n := nd(id, comp, designir.TypeInstance, children...)
	n.Instance = &designir.Instance{ComponentID: comp, ComponentSourceID: comp + "-src"}
	return n
}

func def(id, comp string, children ...designir.Node) designir.Node {
	n := nd(id, comp, designir.TypeComponent, children...)
	n.Component = &designir.ComponentRole{ComponentID: comp}
	return n
}

func img(id, asset string) designir.Node {
	n := nd(id, "image", designir.TypeImage)
	n.Asset = &designir.AssetUse{AssetID: asset, Role: "content"}
	return n
}

type screenSpec struct {
	id, name string
	children []designir.Node
}

func makeIR(comps []string, assets []string, specs ...screenSpec) *designir.DesignIR {
	ir := &designir.DesignIR{SchemaVersion: 1, Source: designir.Source{Provider: "figma", FileKey: "KEY", NodeIDs: []string{}}}
	for _, c := range comps {
		ir.Components = append(ir.Components, designir.Component{ID: c, SourceID: c + "-src", Name: c, ScreenIDs: []string{}})
	}
	for _, a := range assets {
		ir.Assets = append(ir.Assets, designir.Asset{ID: a, Kind: "image", Format: "png", MediaType: "image/png", Path: "assets/" + a + ".png", SizeBytes: 10, SHA256: "aa" + a})
	}
	for _, s := range specs {
		root := nd("r_"+s.id, s.name, designir.TypeFrame, s.children...)
		root.Layout = &designir.Layout{Mode: designir.LayoutVertical, Gap: ptr(8)}
		w := 100.0
		ir.Screens = append(ir.Screens, designir.Screen{
			ID: s.id, SourceNodeID: root.Source.NodeID, Name: s.name, Width: 100, Height: 100, Root: root,
			Reference: &designir.Reference{Path: "reference/" + s.id + ".png", Width: &w, Height: &w, SHA256: "ff" + s.id},
		})
	}
	ir.Finalize()
	return ir
}

func ptr(v float64) *float64 { return &v }

func sids(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("screen_%016x", i+1)
	}
	return out
}

func unitIDs(p *Plan, typ string) []string {
	var out []string
	for _, u := range p.Units {
		if u.Type == typ {
			out = append(out, u.ID)
		}
	}
	return out
}

func hasEdge(p *Plan, unit, on string) bool {
	for _, e := range p.Dependencies {
		if e.Unit == unit && e.DependsOn == on {
			return true
		}
	}
	return false
}

func mustBuild(t *testing.T, ir *designir.DesignIR, sel Selection) *Plan {
	t.Helper()
	p, err := Build(ir, Request{DesignVersion: "dv_1", Selection: sel}, Limits{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return p
}

// appDesign is Dashboard and Settings sharing AppShell, Sidebar and Avatar, plus unrelated Login using Button.
func appDesign() *designir.DesignIR {
	avatarDef := def("avdef", "Avatar", img("avimg", "avatar"))
	sidebarDef := def("sbdef", "Sidebar", inst("sb_av", "Avatar"))
	return makeIR(
		[]string{"AppShell", "Sidebar", "Avatar", "Navbar", "Button", "Chart"},
		[]string{"logo", "avatar", "chart", "hero", "unused"},
		screenSpec{"screen_a", "Components", []designir.Node{avatarDef, sidebarDef, def("shdef", "AppShell", inst("sh_sb", "Sidebar"), inst("sh_nav", "Navbar"))}},
		screenSpec{"screen_dash", "Dashboard", []designir.Node{inst("d_shell", "AppShell", inst("d_shell_sb", "Sidebar", inst("d_shell_av", "Avatar"))), inst("d_chart", "Chart", img("dchartimg", "chart")), img("dlogo", "logo")}},
		screenSpec{"screen_set", "Settings", []designir.Node{inst("s_shell", "AppShell", inst("s_shell_sb", "Sidebar", inst("s_shell_av", "Avatar"))), inst("s_btn", "Button")}},
		screenSpec{"screen_login", "Login", []designir.Node{inst("l_btn", "Button"), img("lhero", "hero")}},
	)
}

func TestOneScreenIncludesOnlyWhatItNeeds(t *testing.T) {
	ir := appDesign()
	p := mustBuild(t, ir, Selection{Mode: ModeOne, ScreenID: "screen_login"})
	if got := unitIDs(p, UnitScreen); !reflect.DeepEqual(got, []string{"screen_screen_login"}) {
		t.Fatalf("screens = %v", got)
	}
	if got := unitIDs(p, UnitComponent); !reflect.DeepEqual(got, []string{"component_Button"}) {
		t.Fatalf("components = %v", got)
	}
	if len(p.Assets) != 1 || p.Assets[0].ID != "hero" {
		t.Fatalf("assets = %+v", p.Assets)
	}
	for _, kind := range []string{UnitFoundation, UnitDesignFoundation, UnitIntegration, UnitVerification} {
		if len(unitIDs(p, kind)) != 1 {
			t.Fatalf("expected one %s unit", kind)
		}
	}
}

func TestSelectedScreensShareDeduplicatedDependencies(t *testing.T) {
	p := mustBuild(t, appDesign(), Selection{Mode: ModeSelected, ScreenIDs: []string{"screen_set", "screen_dash", "screen_dash"}})
	if got := p.Selection.ScreenIDs; !reflect.DeepEqual(got, []string{"screen_dash", "screen_set"}) {
		t.Fatalf("selection order = %v", got)
	}
	comps := unitIDs(p, UnitComponent)
	want := []string{"component_Avatar", "component_Sidebar", "component_AppShell", "component_Navbar", "component_Button", "component_Chart"}
	if len(comps) != len(want) {
		t.Fatalf("components = %v", comps)
	}
	seen := map[string]int{}
	for _, c := range comps {
		seen[c]++
	}
	for _, w := range want {
		if seen[w] != 1 {
			t.Fatalf("%s appears %d times in %v", w, seen[w], comps)
		}
	}
	for _, e := range [][2]string{
		{"component_Sidebar", "component_Avatar"}, {"component_AppShell", "component_Sidebar"}, {"component_AppShell", "component_Navbar"},
		{"screen_screen_dash", "component_AppShell"}, {"screen_screen_set", "component_AppShell"}, {"screen_screen_set", "component_Button"},
		{"component_Avatar", "design_foundation"}, {"integration", "screen_screen_dash"}, {"verification_screen_dash", "integration"},
	} {
		if !hasEdge(p, e[0], e[1]) {
			t.Errorf("missing edge %s -> %s", e[0], e[1])
		}
	}
	for _, a := range p.Assets {
		if a.ID == "hero" || a.ID == "unused" {
			t.Errorf("asset %s must not be included", a.ID)
		}
	}
	if len(p.Assets) != 3 {
		t.Errorf("assets = %+v, want logo, avatar and chart", p.Assets)
	}
}

func TestFlowResolvesItsScreens(t *testing.T) {
	ir := appDesign()
	ir.Sections = []designir.Section{{ID: "flow_main", SourceNodeID: "sec", Name: "Main", ScreenIDs: []string{"screen_set", "screen_dash"}}, {ID: "flow_auth", SourceNodeID: "sec2", Name: "Auth", ScreenIDs: []string{"screen_login"}}}
	p := mustBuild(t, ir, Selection{Mode: ModeFlow, FlowID: "flow_main"})
	if !reflect.DeepEqual(p.Selection.ScreenIDs, []string{"screen_dash", "screen_set"}) || p.Selection.FlowID != "flow_main" {
		t.Fatalf("selection = %+v", p.Selection)
	}
	if _, err := Build(ir, Request{DesignVersion: "v", Selection: Selection{Mode: ModeFlow, FlowID: "flow_missing"}}, Limits{}); CodeOf(err) != CodeFlowNotFound {
		t.Fatalf("err = %v", err)
	}
}

func TestSelectionRejections(t *testing.T) {
	ir := appDesign()
	cases := map[string]struct {
		sel  Selection
		code string
	}{
		"empty selected":     {Selection{Mode: ModeSelected}, CodeEmptySelection},
		"missing screen":     {Selection{Mode: ModeOne, ScreenID: "screen_zzz"}, CodeScreenNotFound},
		"other design":       {Selection{Mode: ModeSelected, ScreenIDs: []string{"screen_dash", "screen_from_another_design"}}, CodeScreenNotFound},
		"malformed":          {Selection{Mode: ModeOne, ScreenID: "../etc/passwd"}, CodeInvalidSelection},
		"unknown mode":       {Selection{Mode: "everything"}, CodeInvalidSelection},
		"mixed fields":       {Selection{Mode: ModeAll, ScreenID: "screen_dash"}, CodeInvalidSelection},
		"flow without id":    {Selection{Mode: ModeFlow}, CodeInvalidSelection},
		"one with a list":    {Selection{Mode: ModeOne, ScreenID: "screen_dash", ScreenIDs: []string{"x"}}, CodeInvalidSelection},
		"one without screen": {Selection{Mode: ModeOne}, CodeInvalidSelection},
	}
	for name, c := range cases {
		if _, err := Build(ir, Request{DesignVersion: "v", Selection: c.sel}, Limits{}); CodeOf(err) != c.code {
			t.Errorf("%s: err = %v, want %s", name, err, c.code)
		}
	}
}

func TestTargetMustBeNextTypeScript(t *testing.T) {
	ir := appDesign()
	for _, target := range []Target{{"react", "typescript"}, {"nextjs", "javascript"}, {"nextjs", ""}} {
		if _, err := Build(ir, Request{DesignVersion: "v", Selection: Selection{Mode: ModeAll}, Target: target}, Limits{}); CodeOf(err) != CodeTargetUnsupported {
			t.Errorf("%+v: err = %v", target, err)
		}
	}
	p, err := Build(ir, Request{DesignVersion: "v", Selection: Selection{Mode: ModeAll}}, Limits{})
	if err != nil || p.Target != (Target{FrameworkNextJS, LanguageTypeScript}) {
		t.Fatalf("default target: %v %+v", err, p)
	}
	if _, err := Build(ir, Request{Selection: Selection{Mode: ModeAll}}, Limits{}); CodeOf(err) != CodeInvalidDesignVersion {
		t.Fatalf("missing version: %v", err)
	}
}

func TestStagesExposeParallelWork(t *testing.T) {
	p := mustBuild(t, appDesign(), Selection{Mode: ModeSelected, ScreenIDs: []string{"screen_dash", "screen_set", "screen_login"}})
	if p.Stages[0][0] != "foundation" || p.Stages[1][0] != "design_foundation" {
		t.Fatalf("stages = %v", p.Stages)
	}
	last := p.Stages[len(p.Stages)-1]
	if len(last) != 3 || last[0] != "verification_screen_dash" {
		t.Fatalf("last stage = %v", last)
	}
	g, _ := p.Graph()
	ready := g.Ready(map[string]bool{"foundation": true, "design_foundation": true})
	if len(ready) < 3 {
		t.Fatalf("ready = %v", ready)
	}
	for _, id := range ready {
		for _, d := range g.DependsOn(id) {
			if d != "design_foundation" && d != "foundation" {
				t.Fatalf("%s is ready but waits for %s", id, d)
			}
		}
	}
	if got := g.Ready(nil); len(got) != 1 || got[0] != "foundation" {
		t.Fatalf("nothing done: %v", got)
	}
	if p.Summary.MaxParallel < 3 || p.Summary.Stages != len(p.Stages) {
		t.Fatalf("summary = %+v", p.Summary)
	}
}

func TestComponentCycleIsAControlledError(t *testing.T) {
	ir := makeIR([]string{"A", "B", "C"}, nil,
		screenSpec{"screen_one", "One", []designir.Node{
			def("da", "A", inst("ab", "B")), def("db", "B", inst("bc", "C")), def("dc", "C", inst("ca", "A")),
		}},
	)
	_, err := Build(ir, Request{DesignVersion: "v", Selection: Selection{Mode: ModeAll}}, Limits{})
	if CodeOf(err) != CodeDependencyCycle {
		t.Fatalf("err = %v", err)
	}
}

func TestComponentContainingItselfIsACycle(t *testing.T) {
	ir := makeIR([]string{"A"}, nil, screenSpec{"screen_one", "One", []designir.Node{def("da", "A", inst("aa", "A"))}})
	if _, err := Build(ir, Request{DesignVersion: "v", Selection: Selection{Mode: ModeAll}}, Limits{}); CodeOf(err) != CodeDependencyCycle {
		t.Fatalf("err = %v", err)
	}
}

func TestMissingReferencesFailSafely(t *testing.T) {
	ir := appDesign()
	ir.Screens[1].Root.Children = append(ir.Screens[1].Root.Children, inst("ghost", "Ghost"))
	if _, err := Build(ir, Request{DesignVersion: "v", Selection: Selection{Mode: ModeOne, ScreenID: "screen_dash"}}, Limits{}); CodeOf(err) != CodeDependencyInvalid {
		t.Fatalf("component: %v", err)
	}
	ir = appDesign()
	ir.Screens[3].Root.Children = append(ir.Screens[3].Root.Children, img("ghostimg", "no_such_asset"))
	if _, err := Build(ir, Request{DesignVersion: "v", Selection: Selection{Mode: ModeOne, ScreenID: "screen_login"}}, Limits{}); CodeOf(err) != CodeDependencyInvalid {
		t.Fatalf("asset: %v", err)
	}
}

func TestMalformedDesignDoesNotPanic(t *testing.T) {
	if _, err := Build(nil, Request{DesignVersion: "v", Selection: Selection{Mode: ModeAll}}, Limits{}); CodeOf(err) != CodePlanInvalid {
		t.Fatalf("nil: %v", err)
	}
	ir := appDesign()
	ir.Screens[1].Root.Children = nil
	ir.Screens = append(ir.Screens, ir.Screens[1])
	if _, err := Build(ir, Request{DesignVersion: "v", Selection: Selection{Mode: ModeAll}}, Limits{}); CodeOf(err) != CodePlanInvalid {
		t.Fatalf("duplicate screen: %v", err)
	}
}

func TestPlanIsDeterministic(t *testing.T) {
	sel := Selection{Mode: ModeAll}
	first := mustBuild(t, appDesign(), sel)
	for i := 0; i < 20; i++ {
		again := mustBuild(t, appDesign(), sel)
		if !reflect.DeepEqual(first, again) {
			t.Fatal("plans differ between runs")
		}
	}
	other := mustBuild(t, appDesign(), Selection{Mode: ModeOne, ScreenID: "screen_dash"})
	if other.Fingerprint == first.Fingerprint {
		t.Fatal("different selections share a fingerprint")
	}
	p2, _ := Build(appDesign(), Request{DesignVersion: "dv_2", Selection: sel}, Limits{})
	if p2.Fingerprint == first.Fingerprint {
		t.Fatal("different design versions share a fingerprint")
	}
}

func TestValidateRejectsTamperedPlans(t *testing.T) {
	good := mustBuild(t, appDesign(), Selection{Mode: ModeAll})
	edit := func(f func(p *Plan)) *Plan {
		p := mustBuild(t, appDesign(), Selection{Mode: ModeAll})
		f(p)
		return p
	}
	cases := map[string]*Plan{
		"schema":       edit(func(p *Plan) { p.SchemaVersion = 9 }),
		"version":      edit(func(p *Plan) { p.DesignVersion = "" }),
		"target":       edit(func(p *Plan) { p.Target.Framework = "vue" }),
		"no screens":   edit(func(p *Plan) { p.Selection.ScreenIDs = nil }),
		"self edge":    edit(func(p *Plan) { p.Dependencies = append(p.Dependencies, Edge{"integration", "integration"}) }),
		"missing edge": edit(func(p *Plan) { p.Dependencies = append(p.Dependencies, Edge{"integration", "nothing"}) }),
		"dup unit":     edit(func(p *Plan) { p.Units = append(p.Units, p.Units[0]) }),
		"bad type":     edit(func(p *Plan) { p.Units[0].Type = "magic" }),
		"stages":       edit(func(p *Plan) { p.Stages[0], p.Stages[1] = p.Stages[1], p.Stages[0] }),
		"fingerprint":  edit(func(p *Plan) { p.Fingerprint = "00" }),
		"asset":        edit(func(p *Plan) { p.Units[len(p.Units)-1].AssetIDs = []string{"gone"} }),
	}
	for name, p := range cases {
		if err := Validate(p, nil); err == nil {
			t.Errorf("%s: tampered plan passed validation", name)
		}
	}
	if err := Validate(good, appDesign()); err != nil {
		t.Fatalf("good plan: %v", err)
	}
	units := []Unit{{ID: "a", Type: UnitFoundation}, {ID: "b", Type: UnitComponent}, {ID: "c", Type: UnitScreen}}
	if _, err := NewGraph(units, []Edge{{"a", "b"}, {"b", "c"}, {"c", "a"}}); CodeOf(err) != CodeDependencyCycle {
		t.Fatalf("cycle: %v", err)
	}
	if _, err := NewGraph(units, []Edge{{"a", "b"}, {"a", "b"}}); CodeOf(err) != CodeDependencyInvalid {
		t.Fatalf("duplicate edge: %v", err)
	}
}

func bigDesign(n int) *designir.DesignIR {
	comps := []string{"Button", "Card", "Nav", "Avatar", "Icon"}
	var specs []screenSpec
	for i, id := range sids(n) {
		specs = append(specs, screenSpec{id, fmt.Sprintf("Screen %d", i+1), []designir.Node{
			inst(fmt.Sprintf("btn%d", i), "Button"), inst(fmt.Sprintf("nav%d", i), "Nav", inst(fmt.Sprintf("av%d", i), "Avatar")), inst(fmt.Sprintf("card%d", i), comps[1+i%2*0]),
		}})
	}
	return makeIR(comps, nil, specs...)
}

func TestThreeHundredScreens(t *testing.T) {
	ir := bigDesign(300)
	start := time.Now()
	p := mustBuild(t, ir, Selection{Mode: ModeAll})
	took := time.Since(start)
	if len(unitIDs(p, UnitScreen)) != 300 || len(unitIDs(p, UnitVerification)) != 300 || len(p.Selection.ScreenIDs) != 300 {
		t.Fatalf("screens=%d verifications=%d", len(unitIDs(p, UnitScreen)), len(unitIDs(p, UnitVerification)))
	}
	if got := len(unitIDs(p, UnitComponent)); got != 4 {
		t.Fatalf("components = %d, want 4 shared units", got)
	}
	if len(unitIDs(p, UnitFoundation)) != 1 || len(unitIDs(p, UnitIntegration)) != 1 {
		t.Fatal("foundation and integration must each exist once")
	}
	widest := 0
	for _, s := range p.Stages {
		if len(s) > widest {
			widest = len(s)
		}
	}
	if widest != 300 {
		t.Fatalf("widest stage = %d, want 300 independent screens", widest)
	}
	if took > 3*time.Second {
		t.Fatalf("planning took %s", took)
	}
	if again := mustBuild(t, ir, Selection{Mode: ModeAll}); again.Fingerprint != p.Fingerprint {
		t.Fatal("not deterministic")
	}
}

func TestPlanLimits(t *testing.T) {
	if _, err := Build(bigDesign(50), Request{DesignVersion: "v", Selection: Selection{Mode: ModeAll}}, Limits{MaxUnits: 20}); CodeOf(err) != CodePlanTooLarge {
		t.Fatalf("units: %v", err)
	}
	if _, err := Build(bigDesign(50), Request{DesignVersion: "v", Selection: Selection{Mode: ModeAll}}, Limits{MaxWork: 10}); CodeOf(err) != CodePlanTooLarge {
		t.Fatalf("work: %v", err)
	}
}

func TestConcurrentPlansShareOneDesignSafely(t *testing.T) {
	ir := bigDesign(60)
	before := fmt.Sprintf("%v", ir.Screens[0])
	want := mustBuild(t, ir, Selection{Mode: ModeAll}).Fingerprint
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			sel := Selection{Mode: ModeAll}
			if i%2 == 1 {
				sel = Selection{Mode: ModeSelected, ScreenIDs: sids(10)}
			}
			p, err := Build(ir, Request{DesignVersion: "dv_1", Selection: sel}, Limits{})
			if err != nil {
				t.Error(err)
				return
			}
			if i%2 == 0 && p.Fingerprint != want {
				t.Error("concurrent plan differs")
			}
		}(i)
	}
	wg.Wait()
	if fmt.Sprintf("%v", ir.Screens[0]) != before {
		t.Fatal("planning modified the design")
	}
}

func BenchmarkPlanThreeHundredScreens(b *testing.B) {
	ir := bigDesign(300)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := Build(ir, Request{DesignVersion: "v", Selection: Selection{Mode: ModeAll}}, Limits{}); err != nil {
			b.Fatal(err)
		}
	}
}
