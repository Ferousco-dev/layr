package assets

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ferousco-dev/layr/server/internal/figma"
)

func loadFixture(t *testing.T, name string) figma.Node {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var n figma.Node
	if err := json.Unmarshal(b, &n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestDiscoverImagesDeduplicatesByRefAndKeepsEveryUse(t *testing.T) {
	work := Discover(loadFixture(t, "mixed_assets.json"))

	refs := []string{}
	for _, w := range work.Images {
		refs = append(refs, w.Ref)
	}
	if strings.Join(refs, ",") != "hero111,avatar22,gifref33" {
		t.Fatalf("refs = %v, want distinct refs in document order", refs)
	}
	hero := work.Images[0]
	if len(hero.Uses) != 2 || hero.Uses[0].NodeID != "2:1" || hero.Uses[0].ScaleMode != "FILL" ||
		hero.Uses[1].NodeID != "2:2" || hero.Uses[1].ScaleMode != "FIT" || hero.Uses[1].Rotation != 90 {
		t.Fatalf("hero uses = %#v", hero.Uses)
	}
	avatar := work.Images[1]
	if len(avatar.Uses) != 2 || !avatar.Uses[0].Transform || avatar.Uses[1].NodeID != "I2:7;9:4" {
		t.Fatalf("avatar uses (ellipse fill and one nested in an instance) = %#v", avatar.Uses)
	}
}

func TestDiscoverChoosesOneExportRootPerArtwork(t *testing.T) {
	work := Discover(loadFixture(t, "mixed_assets.json"))

	got := map[string]string{}
	for _, v := range work.Vectors {
		got[v.NodeID] = v.Reason
	}
	want := map[string]string{
		"3:3":      "vector",         // lone icon inside a styled card
		"2:4":      "vector-group",   // logo group: one file, not three
		"2:5":      "vector-group",   // illustration: one file, not five
		"I2:6;9:2": "vector",         // icon inside a filled instance; the fill stays CSS
		"2:12":     "export-setting", // the designer marked it for SVG export
		"13:1":     "vector",         // vector inside an unknown container type
	}
	if len(got) != len(want) {
		t.Fatalf("exports = %v, want %v", got, want)
	}
	for id, reason := range want {
		if got[id] != reason {
			t.Fatalf("%s: reason %q, want %q (all: %v)", id, got[id], reason, got)
		}
	}
	for _, skipped := range []string{"4:1", "4:2", "5:3", "5:5", "2:8", "8:1", "2:9", "3:4", "2:1"} {
		if _, ok := got[skipped]; ok {
			t.Errorf("%s should not be exported on its own", skipped)
		}
	}
}

func TestDiscoverReportsUnsupportedMediaWithoutFailing(t *testing.T) {
	work := Discover(loadFixture(t, "mixed_assets.json"))

	codes := map[string]string{}
	for _, w := range work.Warnings {
		codes[w.Code+"@"+w.NodeID] = w.Message
	}
	for _, key := range []string{
		WarnVideoUnsupported + "@2:10", WarnPatternUnsupported + "@2:11", WarnImageUnresolved + "@2:14", WarnAnimatedImage + "@2:15",
	} {
		if codes[key] == "" {
			t.Errorf("missing warning %s in %v", key, codes)
		}
	}
}

func TestDiscoverIsDeterministicAndSkipsHiddenNodes(t *testing.T) {
	root := loadFixture(t, "mixed_assets.json")

	a, b := Discover(root), Discover(root)

	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	if string(ja) != string(jb) {
		t.Fatal("two runs disagree")
	}
	for _, v := range a.Vectors {
		if v.NodeID == "2:9" {
			t.Fatal("hidden node exported")
		}
	}
}

func TestRootIsNeverExportedAsOneAsset(t *testing.T) {
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame, Children: []figma.Node{{ID: "1:2", Type: figma.NodeVector}}}

	work := Discover(root)

	if len(work.Vectors) != 1 || work.Vectors[0].NodeID != "1:2" {
		t.Fatalf("vectors = %#v", work.Vectors)
	}
}

func TestTextAndImagesPreventGroupExport(t *testing.T) {
	root := figma.Node{ID: "1:1", Type: figma.NodeFrame, Children: []figma.Node{
		{ID: "2:1", Type: figma.NodeGroup, Children: []figma.Node{{ID: "2:2", Type: figma.NodeVector}, {ID: "2:3", Type: figma.NodeText}}},
	}}

	work := Discover(root)

	if len(work.Vectors) != 1 || work.Vectors[0].NodeID != "2:2" {
		t.Fatalf("only the vector, not the mixed group, should export: %#v", work.Vectors)
	}
}
