package normalize

import (
	"encoding/json"
	"flag"
	"os"
	"testing"

	"github.com/ferousco-dev/layr/server/internal/assets"
	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
)

var update = flag.Bool("update", false, "rewrite golden files")

func readNode(t testing.TB, name string) figma.Node {
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

func readManifest(t testing.TB, name string) *assets.Manifest {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	var m assets.Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	return &m
}

func input(roots ...figma.Node) Input {
	in := Input{FileKey: "FILEKEY123456", FileName: "Landing", Version: "77", ImportID: "imp-1"}
	for _, r := range roots {
		in.Roots = append(in.Roots, ScreenInput{Root: r})
	}
	return in
}

func mustNormalize(t testing.TB, in Input) *designir.DesignIR {
	t.Helper()
	ir, err := Normalize(in, designir.DefaultLimits)
	if err != nil {
		t.Fatal(err)
	}
	return ir
}

func frameAt(id string, x, y, w, h float64, children ...figma.Node) figma.Node {
	return figma.Node{ID: id, Name: id, Type: figma.NodeFrame, AbsoluteBoundingBox: &figma.Rect{X: x, Y: y, Width: w, Height: h}, Children: children}
}

func leaf(id, typ string, x, y, w, h float64) figma.Node {
	return figma.Node{ID: id, Name: id, Type: typ, AbsoluteBoundingBox: &figma.Rect{X: x, Y: y, Width: w, Height: h}}
}

func solid(r, g, b, a float64) figma.Paint {
	return figma.Paint{Type: "SOLID", Color: &figma.Color{R: r, G: g, B: b, A: a}}
}

func findNode(n *designir.Node, sourceID string) *designir.Node {
	if n.Source.NodeID == sourceID {
		return n
	}
	for i := range n.Children {
		if f := findNode(&n.Children[i], sourceID); f != nil {
			return f
		}
	}
	return nil
}

func hasWarning(ir *designir.DesignIR, code, nodeID string) bool {
	for _, w := range ir.Warnings {
		if w.Code == code && (nodeID == "" || w.SourceNodeID == nodeID) {
			return true
		}
	}
	return false
}

func f64(v float64) *float64 { return &v }

var assetsManifest = assets.Manifest{
	SchemaVersion: 1,
	Assets: []assets.Asset{{
		ID: "asset_123", Kind: "image", Format: "png", MediaType: "image/png", Path: "assets/hero-abc123.png", SizeBytes: 10, SHA256: "aa",
		Sources: []assets.Source{{NodeID: "2:1", ImageRef: "abc", Role: "fill"}},
	}},
}
