package figma

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// mutations are the wrong shapes a future Figma version could send for any value.
var mutations = []any{nil, "surprise", 12345.5, true, []any{}, map[string]any{"new": "shape"}}

// paths lists every location in a JSON tree.
func paths(v any, prefix []any, out *[][]any) {
	*out = append(*out, prefix)
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			paths(child, append(append([]any(nil), prefix...), k), out)
		}
	case []any:
		for i, child := range t {
			paths(child, append(append([]any(nil), prefix...), i), out)
		}
	}
}

func setAt(root any, path []any, value any) any {
	if len(path) == 0 {
		return value
	}
	switch t := root.(type) {
	case map[string]any:
		t[path[0].(string)] = setAt(t[path[0].(string)], path[1:], value)
	case []any:
		t[path[0].(int)] = setAt(t[path[0].(int)], path[1:], value)
	}
	return root
}

func clone(v any) any {
	b, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(b, &out)
	return out
}

// TestEverySingleValueMayChangeShapeWithoutBreakingTheCall corrupts each fixture value in turn.
func TestEverySingleValueMayChangeShapeWithoutBreakingTheCall(t *testing.T) {
	fixtures := map[string]func(*testing.T, []byte) error{
		"simple_file.json": func(t *testing.T, body []byte) error {
			r := newRig(t, serve(body))
			_, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})
			return err
		},
		"autolayout_text.json": nodesCall("1:2"),
		"strange_things.json":  nodesCall("5:1"),
	}
	for name, call := range fixtures {
		var base any
		if err := json.Unmarshal(fixture(t, name), &base); err != nil {
			t.Fatal(err)
		}
		var all [][]any
		paths(base, nil, &all)

		failures := 0
		for _, path := range all {
			if len(path) == 0 {
				continue
			}
			for _, m := range mutations {
				mutated, _ := json.Marshal(setAt(clone(base), path, m))
				if err := call(t, mutated); err != nil && !structural(path) {
					failures++
					t.Errorf("%s: replacing %v with %v broke the call: %v", name, path, m, err)
				}
			}
		}
		if failures > 0 {
			t.Fatalf("%s: %d tolerable mutations failed", name, failures)
		}
	}
}

func nodesCall(id string) func(*testing.T, []byte) error {
	return func(t *testing.T, body []byte) error {
		r := newRig(t, serve(body))
		_, err := r.api.GetFileNodes(context.Background(), "u", "K", []string{id}, NodesOptions{})
		return err
	}
}

// structural paths are the only ones where no usable data can remain.
func structural(path []any) bool {
	return len(path) == 1 && (path[0] == "document" || path[0] == "nodes")
}

func TestMismatchedFieldIsKeptAsExtraAndFlagged(t *testing.T) {
	body := `{"name":"n","nodes":{"1:1":{"document":{"id":"1:1","name":"A","type":"FRAME","cornerRadius":{"x":1},"itemSpacing":"wide","children":[{"id":"1:2","name":"B","type":"TEXT"}]},"schemaVersion":0}}}`
	r := newRig(t, serve([]byte(body)))

	res, err := r.api.GetFileNodes(context.Background(), "u", "K", []string{"1:1"}, NodesOptions{})
	if err != nil {
		t.Fatal(err)
	}

	n := res.Nodes["1:1"].Document
	if n.CornerRadius != 0 || n.ItemSpacing != 0 || len(n.Children) != 1 {
		t.Fatalf("good data was lost or bad data leaked in: %#v", n)
	}
	if string(n.Extra["cornerRadius"]) != `{"x":1}` || string(n.Extra["itemSpacing"]) != `"wide"` || len(n.Drift) != 2 {
		t.Fatalf("extra = %v drift = %v", n.Extra, n.Drift)
	}
}

func TestBrokenChildrenAreSkippedNotFatal(t *testing.T) {
	body := `{"nodes":{"1:1":{"document":{"id":"1:1","type":"FRAME","children":["oops",null,7,{"id":"1:2","type":"TEXT"}]}}}}`
	r := newRig(t, serve([]byte(body)))

	res, err := r.api.GetFileNodes(context.Background(), "u", "K", []string{"1:1"}, NodesOptions{})
	if err != nil {
		t.Fatal(err)
	}

	n := res.Nodes["1:1"].Document
	if len(n.Children) != 1 || n.Children[0].ID != "1:2" || len(n.Drift) != 3 {
		t.Fatalf("children = %#v drift = %v", n.Children, n.Drift)
	}

	notArray := `{"nodes":{"1:1":{"document":{"id":"1:1","type":"FRAME","children":{"a":[1,2,{"b":3}]},"name":"still here"}}}}`
	r2 := newRig(t, serve([]byte(notArray)))
	res2, err := r2.api.GetFileNodes(context.Background(), "u", "K", []string{"1:1"}, NodesOptions{})
	if err != nil || res2.Nodes["1:1"].Document.Name != "still here" {
		t.Fatalf("err = %v", err)
	}
}

func TestOneMalformedNodeEntryDoesNotSpoilTheOthers(t *testing.T) {
	body := `{"nodes":{"1:1":"garbage","1:2":{"document":{"id":"1:2","type":"FRAME"}},"1:3":{"document":"nope"},"1:4":null}}`
	r := newRig(t, serve([]byte(body)))

	res, err := r.api.GetFileNodes(context.Background(), "u", "K", []string{"1:1", "1:2", "1:3", "1:4", "1:5"}, NodesOptions{})
	if err != nil {
		t.Fatal(err)
	}

	if _, ok := res.Nodes["1:2"]; !ok || len(res.Nodes) != 1 {
		t.Fatalf("nodes = %v", res.Nodes)
	}
	if strings.Join(res.Malformed, ",") != "1:1,1:3" || strings.Join(res.Missing, ",") != "1:4,1:5" {
		t.Fatalf("malformed = %v missing = %v", res.Malformed, res.Missing)
	}
}

func TestImageMapsTolerateOddValues(t *testing.T) {
	fills := newRig(t, serve([]byte(`{"images":{"a":"https://x/a","b":null,"c":7,"d":{"x":1}}}`)))
	got, err := fills.api.GetImageFills(context.Background(), "u", "K")
	if err != nil || len(got) != 1 || got["a"] != "https://x/a" {
		t.Fatalf("fills = %v err = %v", got, err)
	}

	renders := newRig(t, serve([]byte(`{"err":null,"images":{"1:1":"https://x/1","1:2":5,"1:3":[]}}`)))
	res, err := renders.api.RenderNodes(context.Background(), "u", "K", []string{"1:1", "1:2", "1:3"}, RenderOptions{})
	if err != nil || len(res.URLs) != 1 || len(res.Failed) != 2 {
		t.Fatalf("res = %#v err = %v", res, err)
	}
}

func TestDriftReportNamesWhatIsNew(t *testing.T) {
	r := newRig(t, serve(fixture(t, "strange_things.json")))

	res, err := r.api.GetFileNodes(context.Background(), "u", "K", []string{"5:1"}, NodesOptions{})
	if err != nil {
		t.Fatal(err)
	}

	report := Analyze(res.Roots()...)
	if strings.Join(report.UnknownNodeTypes, ",") != "CONNECTOR,STICKY,TABLE,TABLE_CELL,TEXT_PATH" {
		t.Fatalf("unknown types = %v", report.UnknownNodeTypes)
	}
	if len(report.ExtraFields) != 1 || report.ExtraFields[0] != "someBrandNewProperty" {
		t.Fatalf("extra = %v", report.ExtraFields)
	}
	if !strings.Contains(r.logs.String(), "figma.schema_drift") || !strings.Contains(r.logs.String(), "someBrandNewProperty") {
		t.Fatalf("drift not logged: %s", r.logs.String())
	}
}

func TestPlainKnownFilesReportNoDrift(t *testing.T) {
	r := newRig(t, serve(fixture(t, "autolayout_text.json")))

	res, _ := r.api.GetFileNodes(context.Background(), "u", "K", []string{"1:2"}, NodesOptions{})

	if !Analyze(res.Roots()...).Empty() || strings.Contains(r.logs.String(), "schema_drift") {
		t.Fatal("false drift reported for a fully modelled file")
	}
}

func TestHostileNestingIsRejectedNotCrashed(t *testing.T) {
	depth := maxJSONDepth + 10
	body := `{"document":` + strings.Repeat(`{"type":"FRAME","children":[`, depth/2) + strings.Repeat(`]}`, depth/2) + `}`
	r := newRig(t, serve([]byte(body)))

	_, err := r.api.GetFile(context.Background(), "u", "K", FileOptions{})

	if KindOf(err) != KindBadResponse {
		t.Fatalf("err = %v", err)
	}

	deepButFine := `{"document":` + strings.Repeat(`{"type":"FRAME","children":[`, 200) + strings.Repeat(`]}`, 200) + `}`
	ok := newRig(t, serve([]byte(deepButFine)))
	if _, err := ok.api.GetFile(context.Background(), "u", "K", FileOptions{}); err != nil {
		t.Fatalf("legitimately deep tree rejected: %v", err)
	}
}

func FuzzDecodeNeverPanics(f *testing.F) {
	for _, name := range []string{"simple_file.json", "autolayout_text.json", "strange_things.json", "image_fills.json"} {
		b, err := readFixture(name)
		if err == nil {
			f.Add(b)
		}
	}
	f.Add([]byte(`{"document":{"type":"FRAME","children":[null,{"children":7}]}}`))
	f.Fuzz(func(t *testing.T, data []byte) {
		var file File
		_ = json.Unmarshal(data, &file)
		var entry NodeEntry
		_ = json.Unmarshal(data, &entry)
		var node Node
		_ = json.Unmarshal(data, &node)
		_ = Analyze(node, file.Document, entry.Document)
	})
}

func readFixture(name string) ([]byte, error) {
	return os.ReadFile("testdata/" + name)
}

// BenchmarkDecodeLargeTree guards against the lenient decoder becoming slow on big files.
func BenchmarkDecodeLargeTree(b *testing.B) {
	var sb strings.Builder
	sb.WriteString(`{"name":"big","document":{"id":"0:0","type":"DOCUMENT","children":[`)
	for i := 0; i < 20000; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"id":"1:1","name":"Frame","type":"FRAME","layoutMode":"VERTICAL","itemSpacing":8,"fills":[{"type":"SOLID","color":{"r":1,"g":0,"b":0,"a":1}}],"children":[{"id":"1:2","name":"Text","type":"TEXT","characters":"hello","style":{"fontFamily":"Inter","fontSize":14}}]}`)
	}
	sb.WriteString(`]}}`)
	data := []byte(sb.String())
	b.SetBytes(int64(len(data)))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var f File
		if err := json.Unmarshal(data, &f); err != nil {
			b.Fatal(err)
		}
	}
}
