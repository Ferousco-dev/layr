package normalize

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
)

// parent is what a child needs to know about the container it sits in.
type parent struct {
	abs        *figma.Rect
	autoLayout bool
	horizontal bool
}

func (n *normalizer) limitErr(format string, args ...any) error {
	return &designir.Error{Code: designir.CodeLimit, Problems: []string{fmt.Sprintf(format, args...)}}
}

// node converts one Figma node and its subtree. It is a thin coordinator over the concern-specific helpers.
func (n *normalizer) node(f figma.Node, p *parent, depth int) (designir.Node, error) {
	if depth > n.lim.MaxDepth {
		return designir.Node{}, n.limitErr("nesting deeper than %d levels", n.lim.MaxDepth)
	}
	if n.nodes++; n.nodes > n.lim.MaxNodes {
		return designir.Node{}, n.limitErr("more than %d nodes", n.lim.MaxNodes)
	}

	out := designir.Node{
		ID:       n.nodeID(f.ID),
		Source:   designir.SourceRef{Provider: provider, NodeID: f.ID, Type: f.Type},
		Name:     f.Name,
		Visible:  f.IsVisible(),
		Mask:     f.IsMask,
		Children: []designir.Node{},
	}
	if f.IsMask {
		n.warn("UNSUPPORTED_MASK", f.ID, "This layer is a mask; it is kept but its masking effect is not applied.")
	}

	out.Geometry = n.geometry(f, p)
	out.Sizing = sizing(f, p)
	if p != nil {
		out.Position = position(f, p)
	}
	out.StyleRefs = n.styleRefs(f)
	n.classify(f, &out)

	if assetID, exported := n.exports[f.ID]; exported && p != nil {
		return n.exportedVector(f, out, assetID), nil
	}
	out.Layout = n.layout(f, out.Type)
	out.ClipChildren = f.ClipsContent
	n.content(f, &out)

	ctx := &parent{abs: f.AbsoluteBoundingBox, autoLayout: isAutoLayout(f.LayoutMode), horizontal: f.LayoutMode == "HORIZONTAL"}
	for _, c := range f.Children {
		child, err := n.node(c, ctx, depth+1)
		if err != nil {
			return designir.Node{}, err
		}
		out.Children = append(out.Children, child)
	}
	return out, nil
}

// exportedVector folds a subtree that was exported as one SVG into a single vector node.
func (n *normalizer) exportedVector(f figma.Node, out designir.Node, assetID string) designir.Node {
	out.Type, out.Shape = designir.TypeVector, ""
	out.Asset = &designir.AssetUse{AssetID: assetID, Role: "export"}
	out.CollapsedChildren = countDescendants(f)
	out.Appearance = n.appearance(f, false)
	return out
}

func countDescendants(f figma.Node) int {
	total := 0
	for _, c := range f.Children {
		total += 1 + countDescendants(c)
	}
	return total
}

func isAutoLayout(mode string) bool {
	return mode == "HORIZONTAL" || mode == "VERTICAL" || mode == "GRID"
}

// classify maps the Figma type to the IR vocabulary and records components.
func (n *normalizer) classify(f figma.Node, out *designir.Node) {
	switch f.Type {
	case figma.NodeDocument:
		out.Type = designir.TypeDocument
	case figma.NodeCanvas:
		out.Type = designir.TypeCanvas
	case figma.NodeSection:
		out.Type = designir.TypeSection
	case figma.NodeFrame:
		out.Type = designir.TypeFrame
	case "TRANSFORM_GROUP", figma.NodeGroup:
		out.Type = designir.TypeGroup
	case figma.NodeText:
		out.Type = designir.TypeText
	case figma.NodeRectangle:
		out.Type, out.Shape = designir.TypeShape, "rectangle"
	case figma.NodeEllipse:
		out.Type, out.Shape = designir.TypeShape, "ellipse"
	case figma.NodeLine:
		out.Type, out.Shape = designir.TypeShape, "line"
	case figma.NodePolygon:
		out.Type, out.Shape = designir.TypeShape, "polygon"
	case figma.NodeStar:
		out.Type, out.Shape = designir.TypeShape, "star"
	case figma.NodeVector, figma.NodeBooleanOperation:
		out.Type = designir.TypeVector
		if _, exported := n.exports[f.ID]; !exported {
			out.Asset = &designir.AssetUse{Missing: true, Role: "export"}
			n.warn("MISSING_ASSET", f.ID, "A vector has no exported SVG asset; its artwork is missing from the design.")
		}
	case figma.NodeComponent:
		out.Type = designir.TypeComponent
		out.Component = &designir.ComponentRole{ComponentID: n.registerComponent(f.ID, f.Name)}
		if meta, ok := n.in.Components[f.ID]; ok && meta.ComponentSetID != "" {
			out.Component.SetID = n.registerSet(meta.ComponentSetID)
		}
	case figma.NodeComponentSet:
		out.Type = designir.TypeComponentSet
		out.Component = &designir.ComponentRole{SetID: n.registerSet(f.ID)}
	case figma.NodeInstance:
		out.Type = designir.TypeInstance
		out.Instance = n.instance(f)
	default:
		out.Type = designir.TypeUnknown
		n.warn("UNKNOWN_NODE_TYPE", f.ID, "A layer type is not recognised; it is kept as unknown with its children.")
	}
}

func (n *normalizer) content(f figma.Node, out *designir.Node) {
	switch out.Type {
	case designir.TypeText:
		out.Text = n.text(f)
		out.Appearance = n.appearance(f, true)
	case designir.TypeVector:
		out.Appearance = n.appearance(f, true)
	default:
		out.Appearance = n.appearance(f, true)
		if out.Type == designir.TypeShape && singleImageFill(out.Appearance) {
			out.Type = designir.TypeImage
		}
	}
}

// singleImageFill reports a shape whose only visible fill is one image: an image placeholder.
func singleImageFill(a *designir.Appearance) bool {
	if a == nil {
		return false
	}
	visible := 0
	image := false
	for _, p := range a.Fills {
		if p.Visible {
			visible++
			image = p.Type == designir.PaintImage
		}
	}
	return visible == 1 && image
}

func (n *normalizer) instance(f figma.Node) *designir.Instance {
	inst := &designir.Instance{ComponentSourceID: f.ComponentID}
	if f.ComponentID != "" {
		inst.ComponentID = n.registerComponent(f.ComponentID, "")
	}
	names := make([]string, 0, len(f.ComponentProperties))
	for name := range f.ComponentProperties {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := f.ComponentProperties[name]
		inst.Properties = append(inst.Properties, designir.InstanceProperty{Name: name, Type: strings.ToLower(p.Type), Value: formatValue(p.Value)})
	}
	for _, o := range f.Overrides {
		fields := append([]string(nil), o.Fields...)
		sort.Strings(fields)
		inst.Overrides = append(inst.Overrides, designir.Override{SourceNodeID: o.ID, Fields: fields})
	}
	return inst
}

func (n *normalizer) registerComponent(sourceID, fallbackName string) string {
	id := "comp_" + short(n.in.FileKey, sourceID)
	c := n.components[id]
	if c == nil {
		c = &designir.Component{ID: id, SourceID: sourceID, Name: fallbackName, ScreenIDs: []string{}}
		if meta, ok := n.in.Components[sourceID]; ok {
			c.Key, c.Name, c.Description, c.Remote = meta.Key, meta.Name, meta.Description, meta.Remote
			if meta.ComponentSetID != "" {
				c.SetID = n.registerSet(meta.ComponentSetID)
			}
		}
		n.components[id] = c
	} else if c.Name == "" && fallbackName != "" {
		c.Name = fallbackName
	}
	return id
}

func (n *normalizer) registerSet(sourceID string) string {
	id := "set_" + short(n.in.FileKey, sourceID)
	if n.sets[id] == nil {
		s := &designir.ComponentSet{ID: id, SourceID: sourceID}
		if meta, ok := n.in.ComponentSets[sourceID]; ok {
			s.Key, s.Name = meta.Key, meta.Name
		}
		n.sets[id] = s
	}
	return id
}

func (n *normalizer) styleRefs(f figma.Node) []designir.StyleRef {
	if len(f.Styles) == 0 {
		return nil
	}
	roles := make([]string, 0, len(f.Styles))
	for role := range f.Styles {
		roles = append(roles, role)
	}
	sort.Strings(roles)
	out := make([]designir.StyleRef, 0, len(roles))
	for _, role := range roles {
		source := f.Styles[role]
		id := "style_" + short(n.in.FileKey, source)
		if n.styles[id] == nil {
			s := &designir.Style{ID: id, SourceID: source, Type: strings.ToLower(role)}
			if meta, ok := n.in.Styles[source]; ok {
				s.Key, s.Name, s.Description = meta.Key, meta.Name, meta.Description
				if meta.StyleType != "" {
					s.Type = strings.ToLower(meta.StyleType)
				}
			}
			n.styles[id] = s
		}
		out = append(out, designir.StyleRef{Role: strings.ToLower(role), StyleID: id})
	}
	return out
}

func (n *normalizer) registryComponents() []designir.Component {
	out := make([]designir.Component, 0, len(n.components))
	for _, c := range n.components {
		out = append(out, *c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (n *normalizer) registrySets() []designir.ComponentSet {
	out := make([]designir.ComponentSet, 0, len(n.sets))
	for _, s := range n.sets {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (n *normalizer) registryStyles() []designir.Style {
	out := make([]designir.Style, 0, len(n.styles))
	for _, s := range n.styles {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// geometry derives size and parent-relative position; a rotated layer is solved from its bounds around its center.
func (n *normalizer) geometry(f figma.Node, p *parent) designir.Geometry {
	b := f.AbsoluteBoundingBox
	if b == nil {
		n.warn("MISSING_BOUNDS", f.ID, "A layer has no bounds; its size is recorded as zero.")
		return designir.Geometry{}
	}
	g := designir.Geometry{Width: b.Width, Height: b.Height, Absolute: &designir.Rect{X: snap(b.X), Y: snap(b.Y), Width: snap(b.Width), Height: snap(b.Height)}}
	x, y := b.X, b.Y

	if theta := f.Rotation; math.Abs(theta) > 1e-6 {
		g.Rotation = degrees(theta)
		w, h, ok := unrotatedSize(b.Width, b.Height, theta)
		if !ok {
			n.warn("ROTATED_BOUNDS_APPROXIMATE", f.ID, "A rotated layer's size is approximated from its rotated bounds.")
			w, h = b.Width, b.Height
		}
		cx, cy := b.X+b.Width/2, b.Y+b.Height/2
		g.Width, g.Height = w, h
		x, y = cx-w/2, cy-h/2
	}
	if p != nil && p.abs != nil {
		x, y = x-p.abs.X, y-p.abs.Y
	}
	if p == nil {
		x, y = 0, 0
	}
	g.Width, g.Height, g.X, g.Y = snap(g.Width), snap(g.Height), snap(x), snap(y)
	return g
}

// unrotatedSize inverts W = w|cos|+h|sin|, H = w|sin|+h|cos|; near 45 degrees it has no solution.
func unrotatedSize(bw, bh, theta float64) (float64, float64, bool) {
	c, s := math.Abs(math.Cos(theta)), math.Abs(math.Sin(theta))
	det := c*c - s*s
	if math.Abs(det) < 1e-3 {
		return 0, 0, false
	}
	w, h := (bw*c-bh*s)/det, (bh*c-bw*s)/det
	if w < -1e-6 || h < -1e-6 || math.IsNaN(w) || math.IsNaN(h) {
		return 0, 0, false
	}
	return math.Max(w, 0), math.Max(h, 0), true
}
