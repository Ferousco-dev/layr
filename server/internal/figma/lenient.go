package figma

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
	"sync"
)

// Provider JSON is decoded leniently: odd fields are kept raw in Extra and named in Drift.

var errNotObject = errors.New("figma: value is not a JSON object")

type fieldIndex map[string][]int

var fieldCache sync.Map

// indexFor maps JSON names to struct field paths, following embedded structs.
func indexFor(t reflect.Type) fieldIndex {
	if cached, ok := fieldCache.Load(t); ok {
		return cached.(fieldIndex)
	}
	idx := fieldIndex{}
	var walk func(t reflect.Type, prefix []int)
	walk = func(t reflect.Type, prefix []int) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			path := append(append([]int(nil), prefix...), i)
			if f.Anonymous && f.Type.Kind() == reflect.Struct {
				walk(f.Type, path)
				continue
			}
			name := strings.Split(f.Tag.Get("json"), ",")[0]
			if !f.IsExported() || name == "-" || name == "" {
				continue
			}
			idx[name] = path
		}
	}
	walk(t, nil)
	fieldCache.Store(t, idx)
	return idx
}

// objectHooks customise how one struct is decoded.
type objectHooks struct {
	// special fields are decoded from the token stream by the hook instead of generically.
	special map[string]func(dec *json.Decoder) error
	// required fields must decode into their Go type or the whole object fails.
	required map[string]bool
}

// decodeObject fills dst (a pointer to struct) from a JSON object, tolerating drift.
func decodeObject(data []byte, dst any, hooks objectHooks) (map[string]json.RawMessage, []string, error) {
	if b := bytes.TrimSpace(data); len(b) == 0 || b[0] != '{' {
		return nil, nil, errNotObject
	}
	target := reflect.ValueOf(dst).Elem()
	idx := indexFor(target.Type())
	dec := json.NewDecoder(bytes.NewReader(data))
	if _, err := dec.Token(); err != nil {
		return nil, nil, err
	}

	var extra map[string]json.RawMessage
	var drift []string
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		key, _ := keyTok.(string)

		if hook, ok := hooks.special[key]; ok {
			if err := hook(dec); err != nil {
				return nil, nil, err
			}
			continue
		}
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			return nil, nil, err
		}
		path, known := idx[key]
		if !known {
			if extra == nil {
				extra = map[string]json.RawMessage{}
			}
			extra[key] = raw
			continue
		}

		field := target.FieldByIndex(path)
		if err := json.Unmarshal(raw, field.Addr().Interface()); err != nil {
			if hooks.required[key] {
				return nil, nil, fmt.Errorf("figma: required field %q has an unexpected shape", key)
			}
			field.Set(reflect.Zero(field.Type()))
			if extra == nil {
				extra = map[string]json.RawMessage{}
			}
			extra[key] = raw
			drift = append(drift, key)
		}
	}
	return extra, drift, nil
}

// skipValueRest discards the remainder of a value whose first token was already read.
func skipValueRest(dec *json.Decoder, first json.Token) error {
	d, ok := first.(json.Delim)
	if !ok || (d != '{' && d != '[') {
		return nil
	}
	depth := 1
	for depth > 0 {
		tok, err := dec.Token()
		if err != nil {
			return err
		}
		if d, ok := tok.(json.Delim); ok {
			if d == '{' || d == '[' {
				depth++
			} else {
				depth--
			}
		}
	}
	return nil
}

func (n *Node) UnmarshalJSON(data []byte) error {
	var children []Node
	var childDrift []string
	hooks := objectHooks{special: map[string]func(*json.Decoder) error{
		"children": func(dec *json.Decoder) error {
			first, err := dec.Token()
			if err != nil {
				return err
			}
			if d, ok := first.(json.Delim); !ok || d != '[' {
				childDrift = append(childDrift, "children")
				return skipValueRest(dec, first)
			}
			for i := 0; dec.More(); i++ {
				children = append(children, Node{})
				if err := dec.Decode(&children[len(children)-1]); err != nil {
					children = children[:len(children)-1]
					childDrift = append(childDrift, fmt.Sprintf("children[%d]", i))
				}
			}
			_, err = dec.Token()
			return err
		},
	}}

	*n = Node{}
	extra, drift, err := decodeObject(data, n, hooks)
	if err != nil {
		return err
	}
	n.Children, n.Extra, n.Drift = children, extra, append(drift, childDrift...)
	return nil
}

func (f *File) UnmarshalJSON(data []byte) error {
	*f = File{}
	_, _, err := decodeObject(data, f, objectHooks{required: map[string]bool{"document": true}})
	return err
}

func (e *NodeEntry) UnmarshalJSON(data []byte) error {
	*e = NodeEntry{}
	_, _, err := decodeObject(data, e, objectHooks{required: map[string]bool{"document": true}})
	return err
}

const maxJSONDepth = 2000

var errTooDeep = errors.New("response nesting is too deep")

// depthGuard fails before a hostile, deeply nested body can exhaust the stack.
type depthGuard struct {
	r        io.Reader
	depth    int
	inString bool
	escaped  bool
}

func (g *depthGuard) Read(p []byte) (int, error) {
	n, err := g.r.Read(p)
	for _, c := range p[:n] {
		switch {
		case g.inString:
			switch {
			case g.escaped:
				g.escaped = false
			case c == '\\':
				g.escaped = true
			case c == '"':
				g.inString = false
			}
		case c == '"':
			g.inString = true
		case c == '{' || c == '[':
			if g.depth++; g.depth > maxJSONDepth {
				// Hand back nothing so the decoder cannot finish the value from this chunk.
				return 0, errTooDeep
			}
		case c == '}' || c == ']':
			g.depth--
		}
	}
	return n, err
}
