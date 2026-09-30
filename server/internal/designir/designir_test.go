package designir

import (
	"math"
	"strings"
	"testing"
)

func node(id string, typ string, children ...Node) Node {
	return Node{ID: id, Source: SourceRef{Provider: "figma", NodeID: "src-" + id, Type: "FRAME"}, Name: id, Type: typ, Visible: true, Children: children}
}

func valid() *DesignIR {
	root := node("n_root", TypeFrame, node("n_text", TypeText))
	root.Children[0].Text = &Text{Characters: "hello"}
	ir := &DesignIR{
		SchemaVersion: SchemaVersion,
		Source:        Source{Provider: "figma", FileKey: "K", NodeIDs: []string{"src-n_root"}},
		Screens:       []Screen{{ID: "screen_1", SourceNodeID: "src-n_root", Name: "Home", Width: 100, Height: 100, Root: root}},
	}
	ir.Finalize()
	return ir
}

func expectCode(t *testing.T, name string, ir *DesignIR, code string) {
	t.Helper()
	err := Validate(ir, DefaultLimits)
	if CodeOf(err) != code {
		t.Errorf("%s: err = %v, want %s", name, err, code)
	}
}

func TestAValidDesignPasses(t *testing.T) {
	if err := Validate(valid(), DefaultLimits); err != nil {
		t.Fatal(err)
	}
}

func TestValidatorRejectsBrokenDesigns(t *testing.T) {
	mutations := map[string]func(*DesignIR){
		"wrong version":          func(ir *DesignIR) { ir.SchemaVersion = 2 },
		"no provider":            func(ir *DesignIR) { ir.Source.Provider = "" },
		"no screens":             func(ir *DesignIR) { ir.Screens = nil },
		"missing root":           func(ir *DesignIR) { ir.Screens[0].Root = Node{} },
		"root source mismatch":   func(ir *DesignIR) { ir.Screens[0].SourceNodeID = "other" },
		"duplicate node id":      func(ir *DesignIR) { ir.Screens[0].Root.Children[0].ID = "n_root" },
		"duplicate screen":       func(ir *DesignIR) { ir.Screens = append(ir.Screens, ir.Screens[0]) },
		"unknown node type":      func(ir *DesignIR) { ir.Screens[0].Root.Type = "widget" },
		"text with children":     func(ir *DesignIR) { ir.Screens[0].Root.Children[0].Children = []Node{node("n_x", TypeFrame)} },
		"text node without text": func(ir *DesignIR) { ir.Screens[0].Root.Children[0].Text = nil },
		"text on a frame":        func(ir *DesignIR) { ir.Screens[0].Root.Text = &Text{Characters: "x"} },
		"missing source":         func(ir *DesignIR) { ir.Screens[0].Root.Source.NodeID = "" },
		"negative size":          func(ir *DesignIR) { ir.Screens[0].Root.Geometry.Width = -1 },
		"nan":                    func(ir *DesignIR) { ir.Screens[0].Root.Geometry.X = math.NaN() },
		"infinity":               func(ir *DesignIR) { ir.Screens[0].Root.Geometry.Height = math.Inf(1) },
		"broken asset":           func(ir *DesignIR) { ir.Screens[0].Root.Asset = &AssetUse{AssetID: "ghost", Role: "export"} },
		"broken image asset": func(ir *DesignIR) {
			ir.Screens[0].Root.Appearance = &Appearance{Fills: []Paint{{Type: PaintImage, Visible: true, Image: &ImagePaint{AssetID: "ghost"}}}}
		},
		"broken component": func(ir *DesignIR) {
			ir.Screens[0].Root.Instance = &Instance{ComponentID: "comp_ghost", ComponentSourceID: "1"}
		},
		"broken style": func(ir *DesignIR) { ir.Screens[0].Root.StyleRefs = []StyleRef{{Role: "fill", StyleID: "ghost"}} },
		"bad color": func(ir *DesignIR) {
			ir.Screens[0].Root.Appearance = &Appearance{Fills: []Paint{{Type: PaintSolid, Visible: true, Color: &Color{R: 300, A: 1}}}}
		},
		"bad opacity": func(ir *DesignIR) { o := 2.0; ir.Screens[0].Root.Appearance = &Appearance{Opacity: &o} },
		"bad stop": func(ir *DesignIR) {
			ir.Screens[0].Root.Appearance = &Appearance{Fills: []Paint{{Type: PaintLinearGradient, Visible: true, Gradient: &Gradient{Stops: []Stop{{Position: 1.5}}}}}}
		},
		"text run out of range": func(ir *DesignIR) { ir.Screens[0].Root.Children[0].Text.Runs = []Run{{Start: 2, End: 99}} },
		"overlapping runs": func(ir *DesignIR) {
			ir.Screens[0].Root.Children[0].Text.Runs = []Run{{Start: 0, End: 3}, {Start: 2, End: 4}}
		},
		"bad section": func(ir *DesignIR) { ir.Screens[0].SectionID = "ghost" },
		"absolute asset path": func(ir *DesignIR) {
			ir.Assets = []Asset{{ID: "a", Path: "/Users/me/assets/x.png", SHA256: "x"}}
		},
		"url asset path": func(ir *DesignIR) {
			ir.Assets = []Asset{{ID: "a", Path: "https://cdn.example/x.png", SHA256: "x"}}
		},
		"traversal asset path":   func(ir *DesignIR) { ir.Assets = []Asset{{ID: "a", Path: "assets/../../x.png", SHA256: "x"}} },
		"asset without checksum": func(ir *DesignIR) { ir.Assets = []Asset{{ID: "a", Path: "assets/x.png"}} },
		"bad reference path":     func(ir *DesignIR) { ir.Screens[0].Reference = &Reference{Path: "/etc/passwd"} },
	}
	for name, mutate := range mutations {
		ir := valid()
		mutate(ir)
		if err := Validate(ir, DefaultLimits); CodeOf(err) != CodeValidation {
			t.Errorf("%s: err = %v, want %s", name, err, CodeValidation)
		}
	}
	expectCode(t, "nil design", nil, CodeRootMissing)
}

func TestValidatorEnforcesResourceLimits(t *testing.T) {
	deep := node("n_leaf", TypeFrame)
	for i := 0; i < 20; i++ {
		deep = node("n_"+strings.Repeat("a", i+1), TypeFrame, deep)
	}
	ir := valid()
	ir.Screens[0].Root = deep
	ir.Screens[0].SourceNodeID = deep.Source.NodeID
	if err := Validate(ir, Limits{MaxNodes: 1000, MaxDepth: 10, MaxScreens: 5}); CodeOf(err) != CodeLimit {
		t.Errorf("depth: %v", err)
	}
	if err := Validate(valid(), Limits{MaxNodes: 1, MaxDepth: 10, MaxScreens: 5}); CodeOf(err) != CodeLimit {
		t.Errorf("nodes: %v", err)
	}
	if err := Validate(valid(), Limits{MaxNodes: 100, MaxDepth: 10, MaxScreens: 0}); CodeOf(err) != CodeLimit {
		t.Errorf("screens: %v", err)
	}
}

func TestMarshalRefusesInvalidDesigns(t *testing.T) {
	ir := valid()
	ir.Screens[0].Root.Geometry.Width = math.NaN()

	if _, err := Marshal(ir, DefaultLimits); CodeOf(err) != CodeValidation {
		t.Fatalf("err = %v", err)
	}
}

func TestUnmarshalIsStrict(t *testing.T) {
	good, _ := Marshal(valid(), DefaultLimits)
	for name, data := range map[string][]byte{
		"garbage":       []byte("not json"),
		"empty":         nil,
		"trailing data": append(append([]byte{}, good...), []byte(`{"x":1}`)...),
		"unknown field": []byte(strings.Replace(string(good), `"schema_version":1`, `"schema_version":1,"surprise":true`, 1)),
		"wrong version": []byte(strings.Replace(string(good), `"schema_version":1`, `"schema_version":9`, 1)),
	} {
		if _, err := Unmarshal(data, DefaultLimits); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if _, err := Unmarshal(good, DefaultLimits); err != nil {
		t.Fatal(err)
	}
}

func TestFinalizeIsIdempotentAndDeterministic(t *testing.T) {
	ir := valid()
	first, _ := Marshal(ir, DefaultLimits)

	ir.Finalize()
	ir.Finalize()

	second, _ := Marshal(ir, DefaultLimits)
	if string(first) != string(second) {
		t.Fatal("Finalize is not idempotent")
	}
	if strings.Contains(string(first), "null") {
		t.Fatalf("arrays must serialise as [] not null:\n%s", first)
	}
}

func TestTokensCountRecurringValues(t *testing.T) {
	ir := valid()
	blue := Color{R: 0, G: 102, B: 255, A: 1}
	gap := 8.0
	for i := 0; i < 3; i++ {
		n := node("n_box"+strings.Repeat("x", i), TypeFrame)
		n.Appearance = &Appearance{Fills: []Paint{{Type: PaintSolid, Visible: true, Color: &blue}}, Radius: &Radius{TopLeft: 4, TopRight: 4, BottomRight: 4, BottomLeft: 4},
			Effects: []Effect{{Type: EffectDropShadow, Visible: true, Radius: 8, OffsetY: 2, Color: &Color{A: 0.2}}}}
		n.Layout = &Layout{Mode: LayoutVertical, Gap: &gap, Padding: &Padding{Top: 8, Left: 16}}
		ir.Screens[0].Root.Children = append(ir.Screens[0].Root.Children, n)
	}
	ir.Finalize()

	if len(ir.Tokens.Colors) != 1 || ir.Tokens.Colors[0].Count != 3 {
		t.Fatalf("colors = %+v", ir.Tokens.Colors)
	}
	if len(ir.Tokens.Radii) != 1 || ir.Tokens.Radii[0].Count != 12 || len(ir.Tokens.Shadows) != 1 || ir.Tokens.Shadows[0].Count != 3 {
		t.Fatalf("radii %+v shadows %+v", ir.Tokens.Radii, ir.Tokens.Shadows)
	}
	if ir.Tokens.Spacing[0].Value != 8 || ir.Tokens.Spacing[0].Count != 6 || ir.Stats.Nodes != 5 {
		t.Fatalf("spacing %+v stats %+v", ir.Tokens.Spacing, ir.Stats)
	}
}
