package normalize

import (
	"bytes"
	"strconv"
	"sync"
	"testing"

	"github.com/ferousco-dev/layr/server/internal/assets"
	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
)

// screens builds a small multi-screen file: two screens share a Button component, one has a photo.
func multiScreenInput() Input {
	button := func(id string) figma.Node {
		n := leaf(id, figma.NodeInstance, 10, 10, 100, 40)
		n.ComponentID = "5:100"
		n.Fills = []figma.Paint{solid(0, 0.4, 1, 1)}
		return n
	}
	photo := leaf("21:2", figma.NodeRectangle, 0, 100, 300, 200)
	photo.Fills = []figma.Paint{{Type: "IMAGE", ImageRef: "photo", ScaleMode: "FILL"}}
	master := leaf("5:100", figma.NodeComponent, 0, 0, 100, 40)
	master.Fills = []figma.Paint{solid(0, 0.4, 1, 1)}

	home := frameAt("20:1", 0, 0, 1440, 900, button("20:2"), button("20:3"))
	home.Name = "Home"
	about := frameAt("21:1", 2000, 0, 1440, 900, button("21:3"), photo)
	about.Name = "About"
	kit := frameAt("22:1", 4000, 0, 400, 300, master)
	kit.Name = "Kit"
	// A section whose frames are screens of one flow.
	flowA, flowB := frameAt("30:1", 0, 2000, 390, 800), frameAt("30:2", 500, 2000, 390, 800)
	flowA.Name, flowB.Name = "Step 1", "Step 2"
	section := figma.Node{ID: "30:0", Name: "Checkout", Type: figma.NodeSection, Children: []figma.Node{flowA, flowB}}

	in := input()
	in.Roots = []ScreenInput{{Root: home, Page: "Marketing"}, {Root: about, Page: "Marketing"}, {Root: kit, Page: "Kit"}, {Root: section, Page: "Flows"}}
	in.Components = map[string]figma.ComponentMeta{"5:100": {Key: "k", Name: "Button"}}
	in.Manifest = &assets.Manifest{SchemaVersion: 1, Assets: []assets.Asset{
		{ID: "asset_photo", Kind: "image", Format: "png", MediaType: "image/png", Path: "assets/photo-aaaaaa.png", SizeBytes: 5, SHA256: "aa", Sources: []assets.Source{{NodeID: "21:2", ImageRef: "photo", Role: "fill"}}},
		{ID: "asset_unused", Kind: "image", Format: "png", MediaType: "image/png", Path: "assets/unused-bbbbbb.png", SizeBytes: 5, SHA256: "bb", Sources: []assets.Source{{NodeID: "99:9", ImageRef: "unused", Role: "fill"}}},
	}}
	return in
}

func TestEveryScreenGetsAStableIDAndSectionsOpenIntoScreens(t *testing.T) {
	ir := mustNormalize(t, multiScreenInput())

	names := []string{}
	for _, s := range ir.Screens {
		names = append(names, s.Name+"@"+s.Page)
		if s.ID == "" || s.SourceNodeID != s.Root.Source.NodeID {
			t.Fatalf("screen = %+v", s)
		}
	}
	want := []string{"Home@Marketing", "About@Marketing", "Kit@Kit", "Step 1@Flows", "Step 2@Flows"}
	if len(names) != len(want) {
		t.Fatalf("screens = %v", names)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("screens = %v, want %v", names, want)
		}
	}
	if len(ir.Sections) != 1 || ir.Sections[0].Name != "Checkout" || len(ir.Sections[0].ScreenIDs) != 2 || ir.Screens[3].SectionID != ir.Sections[0].ID {
		t.Fatalf("sections = %+v", ir.Sections)
	}
	if len(ir.Source.NodeIDs) != 5 || ir.Source.NodeIDs[3] != "30:1" || ir.Stats.Screens != 5 {
		t.Fatalf("source = %+v stats %+v", ir.Source, ir.Stats)
	}
	if again := mustNormalize(t, multiScreenInput()); again.Screens[0].ID != ir.Screens[0].ID {
		t.Fatal("screen IDs are not stable")
	}
}

func TestSharedComponentsAreOneEntryAcrossScreens(t *testing.T) {
	ir := mustNormalize(t, multiScreenInput())

	if len(ir.Components) != 1 {
		t.Fatalf("components = %+v", ir.Components)
	}
	c := ir.Components[0]
	if c.Name != "Button" || c.InstanceCount != 3 || len(c.ScreenIDs) != 3 {
		t.Fatalf("component = %+v", c)
	}
	kit := ir.Screens[2]
	if c.DefinitionScreenID != kit.ID || c.DefinitionNodeID != kit.Root.Children[0].ID {
		t.Fatalf("definition = %s/%s, want the master on the Kit screen", c.DefinitionScreenID, c.DefinitionNodeID)
	}
	for _, s := range ir.Screens[:3] {
		if len(s.ComponentIDs) != 1 || s.ComponentIDs[0] != c.ID {
			t.Fatalf("screen %s components = %v", s.Name, s.ComponentIDs)
		}
	}
	inst := findNode(&ir.Screens[0].Root, "20:2")
	if inst.Instance.ComponentID != c.ID || inst.Instance.ComponentSourceID != "5:100" {
		t.Fatalf("instance = %+v", inst.Instance)
	}
}

func TestAComponentWithoutAMasterIsKeptAsExternal(t *testing.T) {
	inst := leaf("2:1", figma.NodeInstance, 0, 0, 10, 10)
	inst.ComponentID = "77:7"
	ir := mustNormalize(t, input(frameAt("1:1", 0, 0, 100, 100, inst)))

	if len(ir.Components) != 1 || ir.Components[0].SourceID != "77:7" || ir.Components[0].DefinitionNodeID != "" || ir.Components[0].InstanceCount != 1 {
		t.Fatalf("components = %+v", ir.Components)
	}
}

func TestAssetsAndTokensAreScopedToTheScreensThatUseThem(t *testing.T) {
	ir := mustNormalize(t, multiScreenInput())

	if len(ir.Assets) != 1 || ir.Assets[0].ID != "asset_photo" || len(ir.Assets[0].ScreenIDs) != 1 || ir.Assets[0].ScreenIDs[0] != ir.Screens[1].ID {
		t.Fatalf("assets = %+v (unused assets must be dropped)", ir.Assets)
	}
	if len(ir.Screens[1].AssetIDs) != 1 || len(ir.Screens[0].AssetIDs) != 0 {
		t.Fatalf("screen asset lists: %v %v", ir.Screens[0].AssetIDs, ir.Screens[1].AssetIDs)
	}
	if len(ir.Tokens.Colors) != 1 || ir.Tokens.Colors[0].Value != (designir.Color{R: 0, G: 102, B: 255, A: 1}) || ir.Tokens.Colors[0].Count != 4 {
		t.Fatalf("color tokens = %+v", ir.Tokens.Colors)
	}
}

func TestSelectingScreensNeedsNoFigmaAndKeepsScreensIndependent(t *testing.T) {
	ir := mustNormalize(t, multiScreenInput())
	about := ir.Screens[1]

	subset, err := ir.Select([]string{about.ID}, designir.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}

	if len(subset.Screens) != 1 || subset.Screens[0].ID != about.ID || subset.Stats.Screens != 1 || len(subset.Source.NodeIDs) != 1 || subset.Source.NodeIDs[0] != "21:1" {
		t.Fatalf("subset = %+v", subset.Stats)
	}
	if len(subset.Components) != 1 || subset.Components[0].InstanceCount != 1 || subset.Components[0].DefinitionNodeID != "" || len(subset.Components[0].ScreenIDs) != 1 {
		t.Fatalf("a shared component must be recounted for the subset and lose its master: %+v", subset.Components)
	}
	if len(subset.Assets) != 1 || len(subset.Sections) != 0 || subset.Tokens.Colors[0].Count != 1 {
		t.Fatalf("subset indexes = assets %d sections %d colors %+v", len(subset.Assets), len(subset.Sections), subset.Tokens.Colors)
	}

	noPhoto, err := ir.Select([]string{ir.Screens[0].ID}, designir.DefaultLimits)
	if err != nil || len(noPhoto.Assets) != 0 {
		t.Fatalf("assets of unselected screens must be dropped: %v %d", err, len(noPhoto.Assets))
	}
	flow, err := ir.Select([]string{ir.Screens[3].ID, ir.Screens[4].ID}, designir.DefaultLimits)
	if err != nil || len(flow.Sections) != 1 || len(flow.Sections[0].ScreenIDs) != 2 || len(flow.Components) != 0 {
		t.Fatalf("a flow selection keeps its section only: %v %+v", err, flow.Sections)
	}
	if len(ir.Screens) != 5 || ir.Stats.Screens != 5 {
		t.Fatal("selecting must not change the original")
	}
	if _, err := ir.Select([]string{"screen_nope"}, designir.DefaultLimits); designir.CodeOf(err) != designir.CodeInvalidInput {
		t.Fatalf("unknown screen: %v", err)
	}
	if _, err := ir.Select(nil, designir.DefaultLimits); err == nil {
		t.Fatal("an empty selection must be rejected")
	}
}

func TestNormalizationIsByteForByteStable(t *testing.T) {
	first, err := designir.Marshal(mustNormalize(t, multiScreenInput()), designir.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		again, _ := designir.Marshal(mustNormalize(t, multiScreenInput()), designir.DefaultLimits)
		if !bytes.Equal(first, again) {
			t.Fatalf("run %d differs", i)
		}
	}
}

func TestSerializedDesignRoundTripsAndStaysPortable(t *testing.T) {
	ir := mustNormalize(t, multiScreenInput())

	data, err := designir.Marshal(ir, designir.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	back, err := designir.Unmarshal(data, designir.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := designir.Marshal(back, designir.DefaultLimits)
	if !bytes.Equal(data, again) {
		t.Fatal("round trip changed the design")
	}
	for _, secret := range []string{"http://", "https://", "/Users/", "/tmp/", "Bearer", "access_token", "refresh_token", "Signature"} {
		if bytes.Contains(data, []byte(secret)) {
			t.Fatalf("design contains %q", secret)
		}
	}
	if _, err := designir.Unmarshal(append(data[:len(data)-2:len(data)-2], []byte(`,"extra":1}`)...), designir.DefaultLimits); err == nil {
		t.Fatal("unknown fields must be rejected")
	}
}

func TestLimitsStopPathologicalTrees(t *testing.T) {
	deep := leaf("d:0", figma.NodeRectangle, 0, 0, 1, 1)
	for i := 1; i <= 300; i++ {
		deep = frameAt("d:"+strconv.Itoa(i), 0, 0, 1, 1, deep)
	}
	if _, err := Normalize(input(deep), designir.DefaultLimits); designir.CodeOf(err) != designir.CodeLimit {
		t.Fatalf("deep tree: %v", err)
	}

	wide := frameAt("w:0", 0, 0, 10, 10)
	for i := 1; i <= 50; i++ {
		wide.Children = append(wide.Children, leaf("w:"+strconv.Itoa(i), figma.NodeRectangle, 0, 0, 1, 1))
	}
	small := designir.Limits{MaxNodes: 20, MaxDepth: 10, MaxScreens: 10}
	if _, err := Normalize(input(wide), small); designir.CodeOf(err) != designir.CodeLimit {
		t.Fatalf("wide tree: %v", err)
	}

	in := input()
	for i := 0; i < 5; i++ {
		in.Roots = append(in.Roots, ScreenInput{Root: frameAt("s:"+strconv.Itoa(i), 0, 0, 1, 1)})
	}
	if _, err := Normalize(in, designir.Limits{MaxNodes: 100, MaxDepth: 10, MaxScreens: 3}); designir.CodeOf(err) != designir.CodeLimit {
		t.Fatalf("too many screens: %v", err)
	}
}

func TestBadInputIsRejectedCleanly(t *testing.T) {
	if _, err := Normalize(Input{FileKey: "K"}, designir.DefaultLimits); designir.CodeOf(err) != designir.CodeRootMissing {
		t.Fatalf("no roots: %v", err)
	}
	if _, err := Normalize(input(frameAt("1:1", 0, 0, 1, 1)), designir.DefaultLimits); err != nil {
		t.Fatal(err)
	}
	bad := input(frameAt("1:1", 0, 0, 1, 1))
	bad.FileKey = ""
	if _, err := Normalize(bad, designir.DefaultLimits); designir.CodeOf(err) != designir.CodeInvalidInput {
		t.Fatalf("no file key: %v", err)
	}
}

func TestConcurrentNormalizationsShareNothingUnsafe(t *testing.T) {
	in := multiScreenInput()
	var wg sync.WaitGroup
	results := make([][]byte, 8)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ir, err := Normalize(in, designir.DefaultLimits)
			if err != nil {
				t.Error(err)
				return
			}
			results[i], _ = designir.Marshal(ir, designir.DefaultLimits)
		}(i)
	}
	wg.Wait()
	for i := 1; i < len(results); i++ {
		if !bytes.Equal(results[0], results[i]) {
			t.Fatal("concurrent runs disagree")
		}
	}
}

func TestNamedStylesAreLinkedByReference(t *testing.T) {
	n := leaf("2:1", figma.NodeRectangle, 0, 0, 10, 10)
	n.Styles = map[string]string{"fill": "S:1", "effect": "S:2"}
	in := input(frameAt("1:1", 0, 0, 100, 100, n))
	in.Styles = map[string]figma.StyleMeta{"S:1": {Key: "k1", Name: "Brand/Blue", StyleType: "FILL"}}
	ir := mustNormalize(t, in)

	refs := findNode(&ir.Screens[0].Root, "2:1").StyleRefs
	if len(refs) != 2 || refs[0].Role != "effect" || refs[1].Role != "fill" {
		t.Fatalf("refs = %+v", refs)
	}
	named := 0
	for _, s := range ir.Styles {
		if s.Name == "Brand/Blue" && s.Type == "fill" {
			named++
		}
	}
	if named != 1 || len(ir.Styles) != 2 {
		t.Fatalf("styles = %+v", ir.Styles)
	}
}
