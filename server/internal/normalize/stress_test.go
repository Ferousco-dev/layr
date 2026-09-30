package normalize

import (
	"bytes"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/assets"
	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
)

// bigScreen builds one screen with rows*cols text and image leaves, reusing a few image refs and one component.
func bigScreen(id string, rows, cols int) figma.Node {
	kids := make([]figma.Node, 0, rows)
	for r := 0; r < rows; r++ {
		row := make([]figma.Node, 0, cols)
		for c := 0; c < cols; c++ {
			cell := fmt.Sprintf("%s:%d:%d", id, r, c)
			var n figma.Node
			switch c % 3 {
			case 0:
				n = leaf(cell, figma.NodeText, float64(c*40), float64(r*20), 40, 20)
				n.Characters = "Label " + cell
			case 1:
				n = leaf(cell, figma.NodeRectangle, float64(c*40), float64(r*20), 40, 20)
				n.Fills = []figma.Paint{{Type: "IMAGE", ImageRef: fmt.Sprintf("img%d", (r+c)%8), ScaleMode: "FILL"}}
			default:
				n = leaf(cell, figma.NodeInstance, float64(c*40), float64(r*20), 40, 20)
				n.ComponentID = "5:1"
			}
			row = append(row, n)
		}
		kids = append(kids, frameAt(fmt.Sprintf("%s:r%d", id, r), 0, float64(r*20), float64(cols*40), 20, row...))
	}
	return frameAt(id, 0, 0, float64(cols*40), float64(rows*20), kids...)
}

func stressManifest() *assets.Manifest {
	m := &assets.Manifest{SchemaVersion: 1}
	for i := 0; i < 8; i++ {
		m.Assets = append(m.Assets, assets.Asset{
			ID: fmt.Sprintf("asset_%02d", i), Kind: "image", Format: "png", MediaType: "image/png",
			Path: fmt.Sprintf("assets/img%d-aaaaaa.png", i), SizeBytes: 5, SHA256: "aa",
			Sources: []assets.Source{{ImageRef: fmt.Sprintf("img%d", i), Role: "fill"}},
		})
	}
	return m
}

func stressInput(screens, rows, cols int) Input {
	in := input()
	for i := 0; i < screens; i++ {
		in.Roots = append(in.Roots, ScreenInput{Root: bigScreen(fmt.Sprintf("%d:1", 100+i), rows, cols), Page: "Page " + fmt.Sprint(i/50)})
	}
	in.Components = map[string]figma.ComponentMeta{"5:1": {Key: "k", Name: "Chip"}}
	in.Manifest = stressManifest()
	return in
}

func stressLimits(screens int) designir.Limits {
	return designir.Limits{MaxNodes: 5_000_000, MaxDepth: designir.DefaultLimits.MaxDepth, MaxScreens: screens}
}

func heapMB() float64 {
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)
	return float64(m.HeapAlloc) / (1 << 20)
}

func TestThreeHundredScreensNormalizeValidateAndStayDeterministic(t *testing.T) {
	in := stressInput(300, 8, 6)
	lim := stressLimits(300)

	start := time.Now()
	first, err := Normalize(in, lim)
	if err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	if len(first.Screens) != 300 || first.Stats.Screens != 300 {
		t.Fatalf("screens = %d stats = %+v", len(first.Screens), first.Stats)
	}
	seen := map[string]bool{}
	for _, s := range first.Screens {
		if seen[s.ID] {
			t.Fatalf("duplicate screen id %s", s.ID)
		}
		seen[s.ID] = true
	}
	a, err := designir.Marshal(first, lim)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Normalize(in, lim)
	if err != nil {
		t.Fatal(err)
	}
	b, err := designir.Marshal(second, lim)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("two runs over the same 300 screens produced different bytes")
	}
	t.Logf("300 screens: %d nodes, %d bytes of IR, normalize %s, heap after %.1f MB", first.Stats.Nodes, len(a), took.Round(time.Millisecond), heapMB())
}

func TestScreenLimitBoundaryFailsWithATypedError(t *testing.T) {
	in := stressInput(11, 1, 3)
	if _, err := Normalize(in, designir.Limits{MaxNodes: 1_000_000, MaxDepth: 64, MaxScreens: 11}); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	_, err := Normalize(in, designir.Limits{MaxNodes: 1_000_000, MaxDepth: 64, MaxScreens: 10})
	var ie *designir.Error
	if !errors.As(err, &ie) || ie.Code != designir.CodeLimit {
		t.Fatalf("above the limit: %v", err)
	}
}

func TestNodeLimitBoundaryFailsWithATypedError(t *testing.T) {
	in := stressInput(1, 10, 6)
	ir, err := Normalize(in, stressLimits(10))
	if err != nil {
		t.Fatal(err)
	}
	n := ir.Stats.Nodes
	if _, err := Normalize(in, designir.Limits{MaxNodes: n, MaxDepth: 64, MaxScreens: 10}); err != nil {
		t.Fatalf("exactly at the node limit: %v", err)
	}
	_, err = Normalize(in, designir.Limits{MaxNodes: n - 1, MaxDepth: 64, MaxScreens: 10})
	var ie *designir.Error
	if !errors.As(err, &ie) || ie.Code != designir.CodeLimit {
		t.Fatalf("one over the node limit: %v", err)
	}
}

func TestDeepNestingIsBoundedNotFatal(t *testing.T) {
	deep := leaf("9:0", figma.NodeRectangle, 0, 0, 10, 10)
	for i := 1; i <= 5000; i++ {
		deep = frameAt(fmt.Sprintf("9:%d", i), 0, 0, 10, 10, deep)
	}
	_, err := Normalize(input(deep), designir.DefaultLimits)
	var ie *designir.Error
	if !errors.As(err, &ie) {
		t.Fatalf("5000 levels: err = %v, want a typed Design IR error", err)
	}
}

func TestThousandsOfNodesInOneScreen(t *testing.T) {
	in := stressInput(1, 400, 9)
	ir, err := Normalize(in, stressLimits(1))
	if err != nil {
		t.Fatal(err)
	}
	if ir.Stats.Nodes < 4000 {
		t.Fatalf("nodes = %d, want thousands", ir.Stats.Nodes)
	}
}

func BenchmarkNormalize100Screens(b *testing.B) {
	in := stressInput(100, 8, 6)
	lim := stressLimits(100)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Normalize(in, lim); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkNormalizeLargeTree(b *testing.B) {
	in := stressInput(1, 400, 9)
	lim := stressLimits(1)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Normalize(in, lim); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkValidateDesignIR(b *testing.B) {
	ir, err := Normalize(stressInput(100, 8, 6), stressLimits(100))
	if err != nil {
		b.Fatal(err)
	}
	lim := stressLimits(100)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := designir.Validate(ir, lim); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalAndUnmarshalDesignIR(b *testing.B) {
	lim := stressLimits(100)
	ir, err := Normalize(stressInput(100, 8, 6), lim)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		data, err := designir.Marshal(ir, lim)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := designir.Unmarshal(data, lim); err != nil {
			b.Fatal(err)
		}
	}
}
