package assets

import (
	"sort"
	"strings"

	"github.com/ferousco-dev/layr/server/internal/figma"
)

// ImageUse records one place a node paints an image; the bytes are shared, the usage is not.
type ImageUse struct {
	NodeID    string
	NodeName  string
	Field     string
	ScaleMode string
	Rotation  float64
	Transform bool
}

// ImageWork is one distinct imageRef and everywhere it is used.
type ImageWork struct {
	Ref  string
	Uses []ImageUse
}

// VectorWork is one node to export as SVG.
type VectorWork struct {
	NodeID   string
	NodeName string
	// Reason is "vector", "vector-group" or "export-setting".
	Reason string
}

// Work is everything the imported design needs resolved and downloaded.
type Work struct {
	Images   []*ImageWork
	Vectors  []VectorWork
	Warnings []Warning
}

// Discover walks the imported tree once, deterministically and in document order.
func Discover(root figma.Node) Work {
	d := &discoverer{byRef: map[string]*ImageWork{}, info: map[string]shape{}}
	d.walk(root, true)
	sort.SliceStable(d.work.Warnings, func(i, j int) bool {
		a, b := d.work.Warnings[i], d.work.Warnings[j]
		return a.Code+a.NodeID+a.ImageRef < b.Code+b.NodeID+b.ImageRef
	})
	return d.work
}

type discoverer struct {
	work  Work
	byRef map[string]*ImageWork
	info  map[string]shape
}

// shape summarises a subtree for the vector-art decision.
type shape struct {
	// pure: only vector artwork and plain primitives, with no boxes, text or images of its own.
	pure bool
	// hasVector: contains at least one real vector leaf, so a group of plain rectangles is not art.
	hasVector bool
}

func (d *discoverer) walk(n figma.Node, isRoot bool) {
	if !n.IsVisible() {
		return
	}
	d.paints(n, "fills", n.Fills)
	d.paints(n, "strokes", n.Strokes)

	if !isRoot {
		if reason, ok := d.exportReason(n); ok {
			d.work.Vectors = append(d.work.Vectors, VectorWork{NodeID: n.ID, NodeName: n.Name, Reason: reason})
			return
		}
	}
	for _, c := range n.Children {
		d.walk(c, false)
	}
}

// exportReason picks the outermost node that is one piece of vector artwork.
func (d *discoverer) exportReason(n figma.Node) (string, bool) {
	for _, e := range n.ExportSettings {
		if strings.EqualFold(e.Format, "SVG") {
			return "export-setting", true
		}
	}
	s := d.shapeOf(n)
	if !s.pure || !s.hasVector {
		return "", false
	}
	if vectorLeaf(n.Type) {
		return "vector", true
	}
	return "vector-group", true
}

func vectorLeaf(t string) bool {
	switch t {
	case figma.NodeVector, figma.NodeBooleanOperation, figma.NodeStar, figma.NodePolygon:
		return true
	}
	return false
}

func primitive(t string) bool {
	return t == figma.NodeRectangle || t == figma.NodeEllipse || t == figma.NodeLine
}

func container(t string) bool {
	switch t {
	case figma.NodeGroup, figma.NodeFrame, figma.NodeComponent, figma.NodeInstance, "TRANSFORM_GROUP":
		return true
	}
	return false
}

func (d *discoverer) shapeOf(n figma.Node) shape {
	if s, ok := d.info[n.ID]; ok && n.ID != "" {
		return s
	}
	s := d.computeShape(n)
	if n.ID != "" {
		d.info[n.ID] = s
	}
	return s
}

func (d *discoverer) computeShape(n figma.Node) shape {
	switch {
	case vectorLeaf(n.Type):
		return shape{pure: !hasImagePaint(n), hasVector: true}
	case primitive(n.Type):
		return shape{pure: !hasImagePaint(n)}
	case container(n.Type):
		if styledBox(n) {
			return shape{}
		}
		out, seen := shape{pure: true}, false
		for _, c := range n.Children {
			if !c.IsVisible() {
				continue
			}
			seen = true
			cs := d.shapeOf(c)
			out.pure = out.pure && cs.pure
			out.hasVector = out.hasVector || cs.hasVector
		}
		out.pure = out.pure && seen
		return out
	}
	return shape{}
}

// styledBox is a container that draws something itself, such as a background, border or shadow.
func styledBox(n figma.Node) bool {
	for _, p := range n.Fills {
		if p.IsVisible() {
			return true
		}
	}
	for _, p := range n.Strokes {
		if p.IsVisible() {
			return true
		}
	}
	for _, e := range n.Effects {
		if e.IsVisible() {
			return true
		}
	}
	return false
}

func hasImagePaint(n figma.Node) bool {
	for _, group := range [][]figma.Paint{n.Fills, n.Strokes} {
		for _, p := range group {
			if p.IsVisible() && p.Type == "IMAGE" {
				return true
			}
		}
	}
	return false
}

func (d *discoverer) paints(n figma.Node, field string, paints []figma.Paint) {
	for _, p := range paints {
		if !p.IsVisible() {
			continue
		}
		switch p.Type {
		case "IMAGE":
			d.image(n, field, p)
		case "VIDEO":
			d.warn(WarnVideoUnsupported, n.ID, "", "Video fills are not downloaded; the frame keeps its other content.")
		case "PATTERN":
			d.warn(WarnPatternUnsupported, n.ID, "", "Pattern fills are not extracted.")
		}
	}
}

func (d *discoverer) image(n figma.Node, field string, p figma.Paint) {
	if p.ImageRef == "" {
		d.warn(WarnImageUnresolved, n.ID, "", "An image fill has no image reference.")
		return
	}
	w := d.byRef[p.ImageRef]
	if w == nil {
		w = &ImageWork{Ref: p.ImageRef}
		d.byRef[p.ImageRef] = w
		d.work.Images = append(d.work.Images, w)
	}
	w.Uses = append(w.Uses, ImageUse{
		NodeID: n.ID, NodeName: n.Name, Field: field, ScaleMode: p.ScaleMode,
		Rotation: p.Rotation, Transform: len(p.ImageTransform) > 0,
	})
	if p.GIFRef != "" {
		d.warn(WarnAnimatedImage, n.ID, p.ImageRef, "This image is animated in Figma; only the static image is exported.")
	}
}

func (d *discoverer) warn(code, nodeID, ref, msg string) {
	for _, w := range d.work.Warnings {
		if w.Code == code && w.NodeID == nodeID && w.ImageRef == ref {
			return
		}
	}
	d.work.Warnings = append(d.work.Warnings, Warning{Code: code, NodeID: nodeID, ImageRef: ref, Message: msg})
}
