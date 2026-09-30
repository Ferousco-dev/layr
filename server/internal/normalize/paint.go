package normalize

import (
	"sort"
	"strings"

	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
)

func color(c figma.Color) designir.Color {
	return designir.Color{R: channel(c.R), G: channel(c.G), B: channel(c.B), A: snap(c.A)}
}

var knownBlend = map[string]string{
	"PASS_THROUGH": "pass_through", "NORMAL": "normal", "DARKEN": "darken", "MULTIPLY": "multiply", "LINEAR_BURN": "linear_burn",
	"COLOR_BURN": "color_burn", "LIGHTEN": "lighten", "SCREEN": "screen", "LINEAR_DODGE": "linear_dodge", "COLOR_DODGE": "color_dodge",
	"OVERLAY": "overlay", "SOFT_LIGHT": "soft_light", "HARD_LIGHT": "hard_light", "DIFFERENCE": "difference",
	"EXCLUSION": "exclusion", "HUE": "hue", "SATURATION": "saturation", "COLOR": "color", "LUMINOSITY": "luminosity",
}

// blend returns the canonical blend mode; defaults return "" and unknown modes are kept with a warning.
func (n *normalizer) blend(source, nodeID string) string {
	switch source {
	case "", "NORMAL", "PASS_THROUGH":
		return ""
	}
	if v, ok := knownBlend[source]; ok {
		return v
	}
	n.warn("UNKNOWN_BLEND_MODE", nodeID, "A blend mode is not recognised and was kept as written.")
	return strings.ToLower(source)
}

func (n *normalizer) paints(f figma.Node, paints []figma.Paint) []designir.Paint {
	if len(paints) == 0 {
		return nil
	}
	out := make([]designir.Paint, 0, len(paints))
	for _, p := range paints {
		out = append(out, n.paint(f.ID, p))
	}
	return out
}

func (n *normalizer) paint(nodeID string, p figma.Paint) designir.Paint {
	out := designir.Paint{Visible: p.IsVisible(), BlendMode: n.blend(p.BlendMode, nodeID)}
	if p.Opacity != nil && *p.Opacity != 1 {
		out.Opacity = snapPtr(*p.Opacity)
	}

	switch p.Type {
	case "SOLID":
		out.Type = designir.PaintSolid
		if p.Color != nil {
			c := color(*p.Color)
			out.Color = &c
		}
	case "GRADIENT_LINEAR":
		out.Type, out.Gradient = designir.PaintLinearGradient, gradient(p)
	case "GRADIENT_RADIAL":
		out.Type, out.Gradient = designir.PaintRadialGradient, gradient(p)
	case "GRADIENT_ANGULAR":
		out.Type, out.Gradient = designir.PaintAngularGradient, gradient(p)
	case "GRADIENT_DIAMOND":
		out.Type, out.Gradient = designir.PaintDiamondGradient, gradient(p)
	case "IMAGE":
		out.Type, out.Image = designir.PaintImage, n.imagePaint(nodeID, p)
	case "VIDEO":
		out.Type, out.SourceType = designir.PaintVideo, p.Type
		n.warn("UNSUPPORTED_PAINT", nodeID, "A video fill is not supported and is not rendered.")
	case "PATTERN":
		out.Type, out.SourceType = designir.PaintPattern, p.Type
		n.warn("UNSUPPORTED_PAINT", nodeID, "A pattern fill is not supported and is not rendered.")
	default:
		out.Type, out.SourceType = designir.PaintUnknown, p.Type
		n.warn("UNSUPPORTED_PAINT", nodeID, "A fill type is not recognised and is not rendered.")
	}
	return out
}

func gradient(p figma.Paint) *designir.Gradient {
	g := &designir.Gradient{Stops: make([]designir.Stop, 0, len(p.GradientStops))}
	for _, s := range p.GradientStops {
		g.Stops = append(g.Stops, designir.Stop{Position: snap(s.Position), Color: color(s.Color)})
	}
	for _, h := range p.GradientHandlePositions {
		g.Handles = append(g.Handles, designir.Point{X: snap(h.X), Y: snap(h.Y)})
	}
	return g
}

func (n *normalizer) imagePaint(nodeID string, p figma.Paint) *designir.ImagePaint {
	img := &designir.ImagePaint{ScaleMode: strings.ToLower(p.ScaleMode), Rotation: snap(p.Rotation)}
	if p.ScalingFactor != nil {
		img.ScalingFactor = snapPtr(*p.ScalingFactor)
	}
	for _, row := range p.ImageTransform {
		out := make([]float64, len(row))
		for i, v := range row {
			out[i] = snap(v)
		}
		img.Transform = append(img.Transform, out)
	}
	names := make([]string, 0, len(p.Filters))
	for name := range p.Filters {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		img.Filters = append(img.Filters, designir.Filter{Name: name, Value: snap(p.Filters[name])})
	}

	if asset, ok := n.byImageRef[p.ImageRef]; ok {
		img.AssetID = asset
		return img
	}
	img.Missing = true
	n.warn("MISSING_ASSET", nodeID, "An image fill has no stored asset; the image is missing from the design.")
	return img
}

func (n *normalizer) effects(f figma.Node) []designir.Effect {
	if len(f.Effects) == 0 {
		return nil
	}
	out := make([]designir.Effect, 0, len(f.Effects))
	for _, e := range f.Effects {
		x := designir.Effect{Visible: e.IsVisible(), Radius: snap(e.Radius), Spread: snap(e.Spread), BlendMode: n.blend(e.BlendMode, f.ID), ShowBehind: e.ShowShadowBehindNode}
		switch e.Type {
		case "DROP_SHADOW":
			x.Type = designir.EffectDropShadow
		case "INNER_SHADOW":
			x.Type = designir.EffectInnerShadow
		case "LAYER_BLUR":
			x.Type = designir.EffectLayerBlur
		case "BACKGROUND_BLUR":
			x.Type = designir.EffectBackgroundBlur
		default:
			x.Type, x.SourceType = designir.EffectUnknown, e.Type
			n.warn("UNSUPPORTED_EFFECT", f.ID, "An effect type is not supported and is not rendered.")
		}
		if e.Color != nil {
			c := color(*e.Color)
			x.Color = &c
		}
		if e.Offset != nil {
			x.OffsetX, x.OffsetY = snap(e.Offset.X), snap(e.Offset.Y)
		}
		out = append(out, x)
	}
	return out
}

func stroke(f figma.Node, paints []designir.Paint) *designir.Stroke {
	if len(paints) == 0 {
		return nil
	}
	s := &designir.Stroke{
		Paints: paints, Weight: snap(f.StrokeWeight), Align: strings.ToLower(f.StrokeAlign),
		Cap: strings.ToLower(f.StrokeCap), Join: strings.ToLower(f.StrokeJoin), MiterAngle: snap(f.StrokeMiterAngle),
	}
	for _, d := range f.StrokeDashes {
		s.Dashes = append(s.Dashes, snap(d))
	}
	if w := f.IndividualStrokeWeights; w != nil {
		s.Sides = &designir.Padding{Top: snap(w.Top), Right: snap(w.Right), Bottom: snap(w.Bottom), Left: snap(w.Left)}
	}
	return s
}

// radius keeps per-corner values when Figma provides them, and returns nil when there is no rounding.
func radius(f figma.Node) *designir.Radius {
	if len(f.RectangleCornerRadii) == 4 {
		r := f.RectangleCornerRadii
		out := designir.Radius{TopLeft: snap(r[0]), TopRight: snap(r[1]), BottomRight: snap(r[2]), BottomLeft: snap(r[3])}
		if out == (designir.Radius{}) {
			return nil
		}
		return &out
	}
	if f.CornerRadius == 0 {
		return nil
	}
	v := snap(f.CornerRadius)
	return &designir.Radius{TopLeft: v, TopRight: v, BottomRight: v, BottomLeft: v}
}

// appearance gathers everything that paints a node; nil means nothing to paint.
func (n *normalizer) appearance(f figma.Node, withPaint bool) *designir.Appearance {
	a := &designir.Appearance{BlendMode: n.blend(f.BlendMode, f.ID)}
	if f.Opacity != nil && *f.Opacity != 1 {
		a.Opacity = snapPtr(*f.Opacity)
	}
	if withPaint {
		a.Fills = n.paints(f, f.Fills)
		a.Stroke = stroke(f, n.paints(f, f.Strokes))
		a.Radius = radius(f)
		a.Effects = n.effects(f)
	}
	if a.Opacity == nil && a.BlendMode == "" && a.Fills == nil && a.Stroke == nil && a.Radius == nil && a.Effects == nil {
		return nil
	}
	return a
}
