package designir

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"unicode/utf8"
)

// Limits bound how large a design the pipeline will process.
type Limits struct {
	MaxNodes   int
	MaxDepth   int
	MaxScreens int
}

// DefaultLimits are generous for professional files and stop pathological input.
var DefaultLimits = Limits{MaxNodes: 200000, MaxDepth: 256, MaxScreens: 500}

var nodeTypes = map[string]bool{
	TypeDocument: true, TypeCanvas: true, TypeSection: true, TypeFrame: true, TypeGroup: true, TypeText: true,
	TypeImage: true, TypeVector: true, TypeShape: true, TypeComponent: true, TypeComponentSet: true,
	TypeInstance: true, TypeUnknown: true,
}

// leafTypes cannot contain children.
var leafTypes = map[string]bool{TypeText: true, TypeImage: true, TypeVector: true, TypeShape: true}

const maxProblems = 25

type checker struct {
	ir       *DesignIR
	lim      Limits
	problems []string
	limit    bool
	nodes    int
	ids      map[string]bool
	assets   map[string]bool
	comps    map[string]bool
	sets     map[string]bool
	styles   map[string]bool
	path     []string
}

func (c *checker) fail(format string, args ...any) {
	if len(c.problems) < maxProblems {
		c.problems = append(c.problems, fmt.Sprintf(format, args...))
	}
}

// Validate checks that ir is a well-formed Design IR v1 with resolvable references.
func Validate(ir *DesignIR, lim Limits) error {
	if ir == nil {
		return &Error{Code: CodeRootMissing, Problems: []string{"design is nil"}}
	}
	c := &checker{
		ir: ir, lim: lim, ids: map[string]bool{}, assets: map[string]bool{}, comps: map[string]bool{},
		sets: map[string]bool{}, styles: map[string]bool{},
	}
	c.header()
	c.registries()
	c.screens()
	c.finite(reflect.ValueOf(ir), 0)

	switch {
	case c.limit:
		return &Error{Code: CodeLimit, Problems: c.problems}
	case len(c.problems) > 0:
		return &Error{Code: CodeValidation, Problems: c.problems}
	}
	return nil
}

func (c *checker) header() {
	if c.ir.SchemaVersion != SchemaVersion {
		c.fail("schema_version %d is not supported", c.ir.SchemaVersion)
	}
	if c.ir.Source.Provider == "" || c.ir.Source.FileKey == "" {
		c.fail("source provider and file key are required")
	}
	for _, id := range c.ir.Source.NodeIDs {
		if id == "" {
			c.fail("source node id is empty")
		}
	}
}

func (c *checker) registries() {
	for _, a := range c.ir.Assets {
		if a.ID == "" || c.assets[a.ID] {
			c.fail("asset id %q is empty or duplicated", a.ID)
		}
		c.assets[a.ID] = true
		if !safeRelativePath(a.Path) {
			c.fail("asset %s has a path that is not a safe relative path", a.ID)
		}
		if a.SizeBytes < 0 || a.SHA256 == "" {
			c.fail("asset %s has no checksum or a negative size", a.ID)
		}
	}
	for _, s := range c.ir.ComponentSets {
		if s.ID == "" || c.sets[s.ID] {
			c.fail("component set id %q is empty or duplicated", s.ID)
		}
		c.sets[s.ID] = true
	}
	for _, k := range c.ir.Components {
		if k.ID == "" || c.comps[k.ID] {
			c.fail("component id %q is empty or duplicated", k.ID)
		}
		c.comps[k.ID] = true
		if k.SetID != "" && !c.sets[k.SetID] {
			c.fail("component %s references a missing set", k.ID)
		}
	}
	for _, s := range c.ir.Styles {
		if s.ID == "" || c.styles[s.ID] {
			c.fail("style id %q is empty or duplicated", s.ID)
		}
		c.styles[s.ID] = true
	}
}

// safeRelativePath accepts only plain relative workspace paths.
func safeRelativePath(p string) bool {
	if p == "" || strings.HasPrefix(p, "/") || strings.Contains(p, "\\") || strings.Contains(p, "://") || strings.Contains(p, "..") {
		return false
	}
	return !strings.ContainsAny(p, "\x00\n\r")
}

func (c *checker) screens() {
	if len(c.ir.Screens) == 0 {
		c.fail("the design has no screens")
		return
	}
	if len(c.ir.Screens) > c.lim.MaxScreens {
		c.limit = true
		c.fail("%d screens exceed the limit of %d", len(c.ir.Screens), c.lim.MaxScreens)
		return
	}
	sections := map[string]bool{}
	for _, s := range c.ir.Sections {
		if s.ID == "" || sections[s.ID] {
			c.fail("section id %q is empty or duplicated", s.ID)
		}
		sections[s.ID] = true
	}
	seen := map[string]bool{}
	for i := range c.ir.Screens {
		s := &c.ir.Screens[i]
		if s.ID == "" || seen[s.ID] {
			c.fail("screen id %q is empty or duplicated", s.ID)
		}
		seen[s.ID] = true
		if s.Root.ID == "" {
			c.fail("screen %s has no root node", s.ID)
			continue
		}
		if s.SourceNodeID != s.Root.Source.NodeID {
			c.fail("screen %s source node does not match its root", s.ID)
		}
		if s.SectionID != "" && !sections[s.SectionID] {
			c.fail("screen %s references a missing section", s.ID)
		}
		if s.Reference != nil && !safeRelativePath(s.Reference.Path) {
			c.fail("screen %s reference path is not a safe relative path", s.ID)
		}
		c.node(&s.Root, 1)
		if c.limit {
			return
		}
	}
	for _, sec := range c.ir.Sections {
		for _, id := range sec.ScreenIDs {
			if !seen[id] {
				c.fail("section %s references a missing screen", sec.ID)
			}
		}
	}
}

func (c *checker) node(n *Node, depth int) {
	if c.limit {
		return
	}
	if depth > c.lim.MaxDepth {
		c.limit = true
		c.fail("nesting deeper than %d levels", c.lim.MaxDepth)
		return
	}
	if c.nodes++; c.nodes > c.lim.MaxNodes {
		c.limit = true
		c.fail("more than %d nodes", c.lim.MaxNodes)
		return
	}
	if n.ID == "" || c.ids[n.ID] {
		c.fail("node id %q is empty or duplicated", n.ID)
	}
	c.ids[n.ID] = true
	if n.Source.NodeID == "" || n.Source.Provider == "" {
		c.fail("node %s has no source reference", n.ID)
	}
	if !nodeTypes[n.Type] {
		c.fail("node %s has unknown type %q", n.ID, n.Type)
	}
	if n.Geometry.Width < 0 || n.Geometry.Height < 0 {
		c.fail("node %s has a negative size", n.ID)
	}
	if leafTypes[n.Type] && len(n.Children) > 0 {
		c.fail("%s node %s cannot have children", n.Type, n.ID)
	}
	if (n.Type == TypeText) != (n.Text != nil) {
		c.fail("node %s: text content must be present exactly on text nodes", n.ID)
	}
	c.references(n)
	if n.Text != nil {
		c.text(n)
	}
	if n.Appearance != nil {
		c.appearance(n)
	}
	for i := range n.Children {
		c.node(&n.Children[i], depth+1)
	}
}

func (c *checker) references(n *Node) {
	if n.Asset != nil && !n.Asset.Missing && !c.assets[n.Asset.AssetID] {
		c.fail("node %s references a missing asset", n.ID)
	}
	if n.Instance != nil && n.Instance.ComponentID != "" && !c.comps[n.Instance.ComponentID] {
		c.fail("node %s references a missing component", n.ID)
	}
	if n.Component != nil {
		if n.Component.ComponentID != "" && !c.comps[n.Component.ComponentID] {
			c.fail("node %s references a missing component", n.ID)
		}
		if n.Component.SetID != "" && !c.sets[n.Component.SetID] {
			c.fail("node %s references a missing component set", n.ID)
		}
	}
	for _, r := range n.StyleRefs {
		if !c.styles[r.StyleID] {
			c.fail("node %s references a missing style", n.ID)
		}
	}
}

func (c *checker) text(n *Node) {
	length := utf8.RuneCountInString(n.Text.Characters)
	prev := 0
	for _, r := range n.Text.Runs {
		if r.Start < prev || r.End <= r.Start || r.End > length {
			c.fail("node %s has an invalid text run [%d,%d)", n.ID, r.Start, r.End)
		}
		prev = r.End
		c.paints(n, r.Style.Fills)
	}
	c.paints(n, n.Text.Style.Fills)
}

func (c *checker) appearance(n *Node) {
	a := n.Appearance
	if a.Opacity != nil && (*a.Opacity < 0 || *a.Opacity > 1) {
		c.fail("node %s opacity is outside 0..1", n.ID)
	}
	c.paints(n, a.Fills)
	if a.Stroke != nil {
		c.paints(n, a.Stroke.Paints)
		if a.Stroke.Weight < 0 {
			c.fail("node %s has a negative stroke weight", n.ID)
		}
	}
	for _, e := range a.Effects {
		if e.Color != nil {
			c.color(n, *e.Color)
		}
	}
}

func (c *checker) paints(n *Node, paints []Paint) {
	for _, p := range paints {
		if p.Color != nil {
			c.color(n, *p.Color)
		}
		if p.Gradient != nil {
			for _, s := range p.Gradient.Stops {
				c.color(n, s.Color)
				if s.Position < 0 || s.Position > 1 {
					c.fail("node %s has a gradient stop outside 0..1", n.ID)
				}
			}
		}
		if p.Image != nil && !p.Image.Missing && !c.assets[p.Image.AssetID] {
			c.fail("node %s paints an image with a missing asset", n.ID)
		}
	}
}

func (c *checker) color(n *Node, col Color) {
	if col.R < 0 || col.R > 255 || col.G < 0 || col.G > 255 || col.B < 0 || col.B > 255 || col.A < 0 || col.A > 1 {
		c.fail("node %s has a color channel out of range", n.ID)
	}
}

// finite rejects NaN and infinity anywhere in the document; the path is built only when a value fails.
func (c *checker) finite(v reflect.Value, depth int) {
	if depth > c.lim.MaxDepth*4+16 || len(c.problems) >= maxProblems {
		return
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface:
		if !v.IsNil() {
			c.finite(v.Elem(), depth+1)
		}
	case reflect.Float32, reflect.Float64:
		if f := v.Float(); math.IsNaN(f) || math.IsInf(f, 0) {
			c.fail("%s is not a finite number", "design."+strings.Join(c.path, "."))
		}
	case reflect.Struct:
		t := v.Type()
		for i := 0; i < v.NumField(); i++ {
			c.path = append(c.path, t.Field(i).Name)
			c.finite(v.Field(i), depth+1)
			c.path = c.path[:len(c.path)-1]
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			c.finite(v.Index(i), depth+1)
		}
	}
}
