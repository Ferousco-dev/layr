package normalize

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
)

func TestHeroMatchesTheGoldenFile(t *testing.T) {
	in := input(readNode(t, "hero.json"))
	in.Manifest = readManifest(t, "hero.assets.json")
	in.Components = map[string]figma.ComponentMeta{"5:100": {Key: "abc", Name: "Button/Primary", ComponentSetID: "5:90"}}
	in.ComponentSets = map[string]figma.ComponentSetMeta{"5:90": {Key: "setkey", Name: "Button"}}

	data, err := designir.Marshal(mustNormalize(t, in), designir.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}

	const golden = "testdata/hero.golden.json"
	if *update {
		if err := os.WriteFile(golden, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, want) {
		t.Fatalf("Design IR changed; review the difference and rerun with -update if intended.\n%s", diffHint(want, data))
	}
}

func diffHint(want, got []byte) string {
	w, g := strings.Split(string(want), "\n"), strings.Split(string(got), "\n")
	for i := 0; i < len(w) && i < len(g); i++ {
		if w[i] != g[i] {
			return "first difference at line " + itoa(i+1) + ":\n  want: " + w[i] + "\n  got:  " + g[i]
		}
	}
	return "different lengths"
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func TestAutoLayoutIsPreservedAsDesignFacts(t *testing.T) {
	ir := mustNormalize(t, func() Input {
		in := input(readNode(t, "hero.json"))
		in.Manifest = readManifest(t, "hero.assets.json")
		return in
	}())

	root := ir.Screens[0].Root
	l := root.Layout
	if l.Mode != "horizontal" || l.Justify != "space_between" || l.Align != "center" || l.Wrap {
		t.Fatalf("layout = %+v", l)
	}
	if l.Gap != nil {
		t.Fatalf("space-between must not invent a numeric gap, got %v", *l.Gap)
	}
	if *l.Padding != (designir.Padding{Top: 64, Right: 120, Bottom: 64, Left: 120}) {
		t.Fatalf("padding = %+v", l.Padding)
	}
	if root.Sizing.Horizontal.Mode != "fixed" || root.Sizing.Vertical.Mode != "hug" || !root.ClipChildren {
		t.Fatalf("sizing = %+v clip %v", root.Sizing, root.ClipChildren)
	}
	names := []string{}
	for _, c := range root.Children {
		names = append(names, c.Name)
	}
	if strings.Join(names, ",") != "Text Group,Illustration,Badge" {
		t.Fatalf("child order = %v", names)
	}
	group := root.Children[0]
	if group.Layout.Mode != "vertical" || *group.Layout.Gap != 24 || group.Position.Mode != "flow" {
		t.Fatalf("nested layout = %+v position %+v", group.Layout, group.Position)
	}
}

func TestGeometryIsRelativeAndFreeOfFloatNoise(t *testing.T) {
	ir := mustNormalize(t, func() Input {
		in := input(readNode(t, "hero.json"))
		in.Manifest = readManifest(t, "hero.assets.json")
		return in
	}())

	root := ir.Screens[0].Root
	group := root.Children[0]
	if root.Geometry.X != 0 || root.Geometry.Y != 0 || root.Geometry.Absolute.X != 100 || root.Geometry.Absolute.Y != 200 || ir.Screens[0].Width != 1440 {
		t.Fatalf("root = %+v", root.Geometry)
	}
	if group.Geometry.X != 120 || group.Geometry.Y != 64 {
		t.Fatalf("y 263.999996 - 200 must become 64: got %+v", group.Geometry)
	}
	badge := root.Children[2]
	if badge.Position.Mode != "absolute" || badge.Geometry.X != 1200 || badge.Geometry.Y != 16 {
		t.Fatalf("absolute child in auto layout = %+v %+v", badge.Position, badge.Geometry)
	}
	if badge.Position.Constraints.Horizontal != "right" || badge.Position.Constraints.Vertical != "top" {
		t.Fatalf("constraints = %+v", badge.Position.Constraints)
	}
}

func TestSnapKeepsRealFractionsAndRemovesNoise(t *testing.T) {
	for in, want := range map[float64]float64{63.999996: 64, 0.0000004: 0, 12.5: 12.5, 0.3333: 0.3333, 1.00049: 1, 2.0051: 2.0051, -0.0000001: 0, 100.12345678: 100.1235} {
		if got := snap(in); got != want {
			t.Errorf("snap(%v) = %v, want %v", in, got, want)
		}
	}
}

func TestRotationSolvesTheUnrotatedSizeAroundTheCenter(t *testing.T) {
	// A 100x40 box rotated 90 degrees has 40x100 bounds around the same center.
	r90 := leaf("2:1", figma.NodeRectangle, 130, 50, 40, 100)
	r90.Rotation = 1.5707963267948966
	// The same box rotated 30 degrees.
	r30 := leaf("2:2", figma.NodeRectangle, 0, 0, 100*0.8660254+40*0.5, 100*0.5+40*0.8660254)
	r30.Rotation = 0.5235987755982988
	ir := mustNormalize(t, input(frameAt("1:1", 0, 0, 400, 400, r90, r30)))

	n90, n30 := findNode(&ir.Screens[0].Root, "2:1"), findNode(&ir.Screens[0].Root, "2:2")
	if n90.Geometry.Width != 100 || n90.Geometry.Height != 40 || n90.Geometry.Rotation != 90 || n90.Geometry.X != 100 || n90.Geometry.Y != 80 {
		t.Fatalf("90 degrees = %+v", n90.Geometry)
	}
	if n30.Geometry.Width != 100 || n30.Geometry.Height != 40 || n30.Geometry.Rotation != 30 {
		t.Fatalf("30 degrees = %+v", n30.Geometry)
	}

	r45 := leaf("2:3", figma.NodeRectangle, 0, 0, 100, 100)
	r45.Rotation = 0.7853981633974483
	if ir := mustNormalize(t, input(frameAt("1:1", 0, 0, 400, 400, r45))); !hasWarning(ir, "ROTATED_BOUNDS_APPROXIMATE", "2:3") {
		t.Fatal("45 degrees has no unique solution and must be flagged")
	}
}

func TestHeadingKeepsTextExactlyAndItsStyleRuns(t *testing.T) {
	ir := mustNormalize(t, func() Input {
		in := input(readNode(t, "hero.json"))
		in.Manifest = readManifest(t, "hero.assets.json")
		return in
	}())

	h := findNode(&ir.Screens[0].Root, "12:36")
	if h.Type != "text" || h.Text.Characters != "Build faster with Layr\nToday" {
		t.Fatalf("text = %q", h.Text.Characters)
	}
	s := h.Text.Style
	if s.FontFamily != "Inter" || *s.FontWeight != 600 || *s.FontSize != 64 || s.LineHeight.Mode != "pixels" || *s.LineHeight.Value != 72 ||
		*s.LetterSpacing != -1.28 || s.Align != "left" || s.VerticalAlign != "top" || s.Case != "original" || s.Decoration != "none" {
		t.Fatalf("style = %+v", s)
	}
	if len(h.Text.Runs) != 1 || h.Text.Runs[0].Start != 5 || h.Text.Runs[0].End != 22 || *h.Text.Runs[0].Style.FontWeight != 300 {
		t.Fatalf("runs = %+v", h.Text.Runs)
	}
	if h.Text.Runs[0].Style.Fills[0].Color.R != 128 {
		t.Fatalf("run color = %+v", h.Text.Runs[0].Style.Fills[0].Color)
	}
	if h.Position.Align != "stretch" {
		t.Fatalf("stretch lost: %+v", h.Position)
	}
	if hasWarning(ir, "MIXED_STYLE_PARTIAL", "") {
		t.Fatal("a consistent run list must not warn")
	}
	label := findNode(&ir.Screens[0].Root, "I12:37;5:101")
	if label.Text.Style.LineHeight.Mode != "auto" || label.Text.Style.LineHeight.Value != nil || label.Text.Style.LetterSpacing != nil {
		t.Fatalf("auto line height or zero spacing mishandled: %+v", label.Text.Style)
	}
}

func TestTextIsNeverTrimmedOrRewritten(t *testing.T) {
	cases := []string{"  leading and trailing  ", "tabs\tand\nnewlines\r\n", "日本語 مرحبا 👋", "double  space", "", " nbsp"}
	for _, chars := range cases {
		node := leaf("2:1", figma.NodeText, 0, 0, 10, 10)
		node.Characters = chars
		ir := mustNormalize(t, input(frameAt("1:1", 0, 0, 100, 100, node)))
		if got := findNode(&ir.Screens[0].Root, "2:1").Text.Characters; got != chars {
			t.Errorf("text %q became %q", chars, got)
		}
	}
}

func TestStyleRunsHandleEmojiAndBadLengths(t *testing.T) {
	node := leaf("2:1", figma.NodeText, 0, 0, 10, 10)
	node.Characters = "a😀b"
	// UTF-16: a=1 unit, emoji=2 units, b=1 unit.
	node.CharacterStyleOverrides = []int{0, 1, 1, 0}
	node.StyleOverrideTable = map[string]figma.TypeStyle{"1": {FontWeight: 700}}
	ir := mustNormalize(t, input(frameAt("1:1", 0, 0, 100, 100, node)))
	runs := findNode(&ir.Screens[0].Root, "2:1").Text.Runs
	if len(runs) != 1 || runs[0].Start != 1 || runs[0].End != 2 {
		t.Fatalf("the emoji is one code point: runs = %+v", runs)
	}

	node.CharacterStyleOverrides = []int{0, 1}
	ir = mustNormalize(t, input(frameAt("1:1", 0, 0, 100, 100, node)))
	if !hasWarning(ir, "MIXED_STYLE_PARTIAL", "2:1") || len(findNode(&ir.Screens[0].Root, "2:1").Text.Runs) != 0 {
		t.Fatal("a mismatched override list must warn and apply nothing")
	}
}

func TestColorsConvertPrecisely(t *testing.T) {
	cases := []struct {
		in   figma.Color
		want designir.Color
	}{
		{figma.Color{R: 0, G: 0, B: 0, A: 1}, designir.Color{R: 0, G: 0, B: 0, A: 1}},
		{figma.Color{R: 1, G: 1, B: 1, A: 1}, designir.Color{R: 255, G: 255, B: 255, A: 1}},
		{figma.Color{R: 0.5, G: 0.5, B: 0.5, A: 0.5}, designir.Color{R: 128, G: 128, B: 128, A: 0.5}},
		{figma.Color{R: 0.0353, G: 0.0353, B: 0.0431, A: 1}, designir.Color{R: 9, G: 9, B: 11, A: 1}},
		{figma.Color{R: 0.2, G: 0.4, B: 0.6, A: 0.333333}, designir.Color{R: 51, G: 102, B: 153, A: 0.3333}},
		{figma.Color{R: 1.0000001, G: -0.0000001, B: 0.9999999, A: 1}, designir.Color{R: 255, G: 0, B: 255, A: 1}},
	}
	for _, tc := range cases {
		node := leaf("2:1", figma.NodeRectangle, 0, 0, 10, 10)
		node.Fills = []figma.Paint{{Type: "SOLID", Color: &tc.in}}
		ir := mustNormalize(t, input(frameAt("1:1", 0, 0, 100, 100, node)))
		if got := *findNode(&ir.Screens[0].Root, "2:1").Appearance.Fills[0].Color; got != tc.want {
			t.Errorf("%+v -> %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func TestGradientsKeepTypeStopsAndOrientation(t *testing.T) {
	hero := mustNormalize(t, func() Input {
		in := input(readNode(t, "hero.json"))
		in.Manifest = readManifest(t, "hero.assets.json")
		return in
	}())
	g := hero.Screens[0].Root.Appearance.Fills[0]
	if g.Type != "linear_gradient" || len(g.Gradient.Stops) != 2 || len(g.Gradient.Handles) != 3 || g.Gradient.Stops[1].Color.A != 0.5 || g.Gradient.Handles[0].Y != 0.5 {
		t.Fatalf("gradient = %+v", g)
	}

	for source, want := range map[string]string{"GRADIENT_RADIAL": "radial_gradient", "GRADIENT_ANGULAR": "angular_gradient", "GRADIENT_DIAMOND": "diamond_gradient"} {
		node := leaf("2:1", figma.NodeRectangle, 0, 0, 10, 10)
		node.Fills = []figma.Paint{{Type: source, GradientStops: []figma.ColorStop{
			{Position: 0, Color: figma.Color{R: 1, A: 1}}, {Position: 0.4, Color: figma.Color{G: 1, A: 1}}, {Position: 1, Color: figma.Color{B: 1, A: 0.2}},
		}, GradientHandlePositions: []figma.Vector{{X: 0.5, Y: 0.5}, {X: 1, Y: 0.5}, {X: 0.5, Y: 1}}}}
		ir := mustNormalize(t, input(frameAt("1:1", 0, 0, 100, 100, node)))
		p := findNode(&ir.Screens[0].Root, "2:1").Appearance.Fills[0]
		if p.Type != want || len(p.Gradient.Stops) != 3 || p.Gradient.Stops[1].Position != 0.4 || p.Gradient.Stops[2].Color.A != 0.2 {
			t.Errorf("%s -> %+v", source, p)
		}
	}
}

func TestStrokeRadiusAndEffects(t *testing.T) {
	ir := mustNormalize(t, func() Input {
		in := input(readNode(t, "hero.json"))
		in.Manifest = readManifest(t, "hero.assets.json")
		return in
	}())

	badge := findNode(&ir.Screens[0].Root, "12:43")
	st := badge.Appearance.Stroke
	if st.Weight != 1 || st.Align != "inside" || len(st.Dashes) != 2 || st.Sides.Left != 2 || st.Paints[0].Color.R != 255 {
		t.Fatalf("stroke = %+v", st)
	}
	if *badge.Appearance.Opacity != 0.8 || badge.Appearance.BlendMode != "multiply" || badge.Appearance.Radius.TopLeft != 16 {
		t.Fatalf("appearance = %+v", badge.Appearance)
	}

	cta := findNode(&ir.Screens[0].Root, "12:37")
	fx := cta.Appearance.Effects
	if len(fx) != 3 || fx[0].Type != "drop_shadow" || fx[0].Radius != 12 || fx[0].Spread != 2 || fx[0].OffsetY != 4 || fx[0].Color.A != 0.25 ||
		fx[1].Radius != 2 || fx[2].Type != "layer_blur" || fx[2].Visible {
		t.Fatalf("effects must keep order, visibility and values: %+v", fx)
	}
}

func TestRadiusVariants(t *testing.T) {
	cases := []struct {
		name string
		node figma.Node
		want *designir.Radius
	}{
		{"uniform", figma.Node{CornerRadius: 8}, &designir.Radius{TopLeft: 8, TopRight: 8, BottomRight: 8, BottomLeft: 8}},
		{"per corner", figma.Node{CornerRadius: 4, RectangleCornerRadii: []float64{4, 8, 0, 12}}, &designir.Radius{TopLeft: 4, TopRight: 8, BottomRight: 0, BottomLeft: 12}},
		{"zero", figma.Node{}, nil},
		{"all zero corners", figma.Node{RectangleCornerRadii: []float64{0, 0, 0, 0}}, nil},
		{"fractional", figma.Node{CornerRadius: 2.5}, &designir.Radius{TopLeft: 2.5, TopRight: 2.5, BottomRight: 2.5, BottomLeft: 2.5}},
	}
	for _, tc := range cases {
		got := radius(tc.node)
		if (got == nil) != (tc.want == nil) || (got != nil && *got != *tc.want) {
			t.Errorf("%s: got %+v, want %+v", tc.name, got, tc.want)
		}
	}
}

func TestImageFillsReferenceAssetsNotUrls(t *testing.T) {
	node := leaf("2:1", figma.NodeRectangle, 0, 0, 200, 100)
	node.Fills = []figma.Paint{{Type: "IMAGE", ImageRef: "abc", ScaleMode: "FILL", Rotation: 90, ImageTransform: [][]float64{{1, 0, 0.5}, {0, 1, 0}},
		Filters: map[string]float64{"exposure": 0.2, "contrast": -0.1}}}
	in := input(frameAt("1:1", 0, 0, 400, 400, node))
	in.Manifest = &assetsManifest
	ir := mustNormalize(t, in)

	n := findNode(&ir.Screens[0].Root, "2:1")
	img := n.Appearance.Fills[0].Image
	if n.Type != "image" || img.AssetID != "asset_123" || img.Missing || img.ScaleMode != "fill" || img.Rotation != 90 || img.Transform[0][2] != 0.5 ||
		len(img.Filters) != 2 || img.Filters[0].Name != "contrast" {
		t.Fatalf("image = %+v type %s", img, n.Type)
	}
	if len(ir.Assets) != 1 || ir.Assets[0].Path != "assets/hero-abc123.png" || len(ir.Assets[0].ScreenIDs) != 1 {
		t.Fatalf("assets = %+v", ir.Assets)
	}
	data, _ := designir.Marshal(ir, designir.DefaultLimits)
	for _, bad := range []string{"http://", "https://", "figma-alpha", "Signature="} {
		if strings.Contains(string(data), bad) {
			t.Fatalf("design contains %q", bad)
		}
	}
}

func TestMissingAssetsWarnInsteadOfVanishing(t *testing.T) {
	image := leaf("2:1", figma.NodeRectangle, 0, 0, 10, 10)
	image.Fills = []figma.Paint{{Type: "IMAGE", ImageRef: "gone"}}
	vec := leaf("2:2", figma.NodeVector, 0, 0, 10, 10)
	ir := mustNormalize(t, input(frameAt("1:1", 0, 0, 100, 100, image, vec)))

	img := findNode(&ir.Screens[0].Root, "2:1")
	v := findNode(&ir.Screens[0].Root, "2:2")
	if !img.Appearance.Fills[0].Image.Missing || !v.Asset.Missing {
		t.Fatal("a missing asset must stay visible in the design as missing")
	}
	if !hasWarning(ir, "MISSING_ASSET", "2:1") || !hasWarning(ir, "MISSING_ASSET", "2:2") {
		t.Fatalf("warnings = %+v", ir.Warnings)
	}
}

func TestExportedVectorsBecomeOneAssetReference(t *testing.T) {
	ir := mustNormalize(t, func() Input {
		in := input(readNode(t, "hero.json"))
		in.Manifest = readManifest(t, "hero.assets.json")
		return in
	}())

	art := findNode(&ir.Screens[0].Root, "12:40")
	if art.Type != "vector" || art.Asset.AssetID != "asset_0123456789abcdef" || art.Asset.Role != "export" || art.Asset.Missing {
		t.Fatalf("art = %+v", art.Asset)
	}
	if art.CollapsedChildren != 2 || len(art.Children) != 0 || art.Source.Type != "GROUP" {
		t.Fatalf("children must be folded into the asset: %+v", art)
	}
	if hasWarning(ir, "MISSING_ASSET", "12:41") {
		t.Fatal("shapes inside an exported group are covered by its SVG")
	}
	if ir.Screens[0].Reference == nil || ir.Screens[0].Reference.Path != "reference/12-34-aabbcc.png" {
		t.Fatalf("reference = %+v", ir.Screens[0].Reference)
	}
}

func TestUnknownNodesMasksAndPaintsAreKeptWithWarnings(t *testing.T) {
	future := frameAt("2:1", 0, 0, 50, 50, leaf("2:2", figma.NodeRectangle, 0, 0, 10, 10))
	future.Type = "FUTURE_SUPER_NODE"
	mask := leaf("2:3", figma.NodeRectangle, 0, 0, 10, 10)
	mask.IsMask = true
	paints := leaf("2:4", figma.NodeRectangle, 0, 0, 10, 10)
	paints.Fills = []figma.Paint{{Type: "VIDEO"}, {Type: "PATTERN"}, {Type: "FUTURE_PAINT"}}
	paints.Effects = []figma.Effect{{Type: "NOISE"}}
	paints.BlendMode = "FUTURE_BLEND"
	ir := mustNormalize(t, input(frameAt("1:1", 0, 0, 100, 100, future, mask, paints)))

	u := findNode(&ir.Screens[0].Root, "2:1")
	if u.Type != "unknown" || u.Source.Type != "FUTURE_SUPER_NODE" || len(u.Children) != 1 || u.Children[0].Shape != "rectangle" {
		t.Fatalf("unknown node = %+v", u)
	}
	if !findNode(&ir.Screens[0].Root, "2:3").Mask {
		t.Fatal("mask flag lost")
	}
	p := findNode(&ir.Screens[0].Root, "2:4")
	if p.Appearance.Fills[0].Type != "video" || p.Appearance.Fills[1].Type != "pattern" || p.Appearance.Fills[2].Type != "unknown" || p.Appearance.Fills[2].SourceType != "FUTURE_PAINT" ||
		p.Appearance.Effects[0].Type != "unknown" || p.Appearance.BlendMode != "future_blend" {
		t.Fatalf("appearance = %+v", p.Appearance)
	}
	for _, code := range []string{"UNKNOWN_NODE_TYPE", "UNSUPPORTED_MASK", "UNSUPPORTED_PAINT", "UNSUPPORTED_EFFECT", "UNKNOWN_BLEND_MODE"} {
		if !hasWarning(ir, code, "") {
			t.Errorf("missing warning %s", code)
		}
	}
}

func TestHiddenNodesAreKept(t *testing.T) {
	hidden := leaf("2:1", figma.NodeRectangle, 0, 0, 10, 10)
	no := false
	hidden.Visible = &no

	ir := mustNormalize(t, input(frameAt("1:1", 0, 0, 100, 100, hidden)))

	if n := findNode(&ir.Screens[0].Root, "2:1"); n == nil || n.Visible {
		t.Fatalf("hidden node = %+v", n)
	}
}

func TestSizingPrefersExplicitFieldsAndFallsBackForOlderFiles(t *testing.T) {
	old := frameAt("2:1", 0, 0, 100, 50)
	old.LayoutMode, old.PrimaryAxisSizingMode, old.CounterAxisSizingMode = "HORIZONTAL", "AUTO", "FIXED"
	child := leaf("2:2", figma.NodeRectangle, 0, 0, 10, 10)
	child.LayoutAlign, child.LayoutGrow = "STRETCH", 1
	child.MinWidth, child.MaxWidth = f64(20), f64(300)
	old.Children = []figma.Node{child}
	ir := mustNormalize(t, input(frameAt("1:1", 0, 0, 400, 400, old)))

	o := findNode(&ir.Screens[0].Root, "2:1")
	c := findNode(&ir.Screens[0].Root, "2:2")
	if o.Sizing.Horizontal.Mode != "hug" || o.Sizing.Vertical.Mode != "fixed" {
		t.Fatalf("derived container sizing = %+v", o.Sizing)
	}
	if c.Sizing.Horizontal.Mode != "fill" || c.Sizing.Vertical.Mode != "fill" || *c.Sizing.Horizontal.Min != 20 || *c.Sizing.Horizontal.Max != 300 || c.Position.Grow != 1 {
		t.Fatalf("child sizing = %+v position %+v", c.Sizing, c.Position)
	}
	if plain := findNode(&ir.Screens[0].Root, "1:1"); plain.Sizing != nil {
		t.Fatalf("fixed/fixed without limits is omitted, got %+v", plain.Sizing)
	}
}

func TestWrapGridAndFreeformFrames(t *testing.T) {
	wrap := frameAt("2:1", 0, 0, 100, 100)
	wrap.LayoutMode, wrap.LayoutWrap, wrap.ItemSpacing, wrap.CounterAxisSpacing, wrap.CounterAxisAlignContent = "HORIZONTAL", "WRAP", 8, 16, "SPACE_BETWEEN"
	grid := frameAt("2:2", 0, 0, 100, 100)
	grid.LayoutMode = "GRID"
	free := frameAt("2:3", 0, 0, 100, 100, leaf("2:4", figma.NodeRectangle, 10, 20, 5, 5))
	ir := mustNormalize(t, input(frameAt("1:1", 0, 0, 400, 400, wrap, grid, free)))

	w := findNode(&ir.Screens[0].Root, "2:1").Layout
	if !w.Wrap || *w.Gap != 8 || *w.CrossGap != 16 || w.WrapAlign != "space_between" {
		t.Fatalf("wrap = %+v", w)
	}
	if findNode(&ir.Screens[0].Root, "2:2").Layout.Mode != "grid" || !hasWarning(ir, "UNSUPPORTED_LAYOUT_GRID", "2:2") {
		t.Fatal("grid layout must be recorded and flagged")
	}
	f := findNode(&ir.Screens[0].Root, "2:3")
	c := findNode(&ir.Screens[0].Root, "2:4")
	if f.Layout.Mode != "none" || c.Position.Mode != "absolute" || c.Geometry.X != 10 || c.Geometry.Y != 20 {
		t.Fatalf("free-form frame = %+v child %+v %+v", f.Layout, c.Position, c.Geometry)
	}
}
