package normalize

import (
	"strings"

	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
)

// layout describes how a container arranges its children; nil for nodes that cannot contain any.
func (n *normalizer) layout(f figma.Node, irType string) *designir.Layout {
	switch irType {
	case designir.TypeFrame, designir.TypeComponent, designir.TypeComponentSet, designir.TypeInstance, designir.TypeSection, designir.TypeGroup:
	default:
		return nil
	}

	mode := designir.LayoutNone
	switch f.LayoutMode {
	case "HORIZONTAL":
		mode = designir.LayoutHorizontal
	case "VERTICAL":
		mode = designir.LayoutVertical
	case "GRID":
		mode = designir.LayoutGrid
		n.warn("UNSUPPORTED_LAYOUT_GRID", f.ID, "Grid layout details are not captured; only the mode is recorded.")
	}
	l := &designir.Layout{Mode: mode}
	if mode == designir.LayoutNone {
		return l
	}

	l.Justify = justify(f.PrimaryAxisAlignItems)
	l.Align = align(f.CounterAxisAlignItems)
	l.Padding = &designir.Padding{Top: snap(f.PaddingTop), Right: snap(f.PaddingRight), Bottom: snap(f.PaddingBottom), Left: snap(f.PaddingLeft)}
	// With space-between the gap is automatic, so no number is invented for it.
	if l.Justify != "space_between" {
		l.Gap = snapPtr(f.ItemSpacing)
	}
	if f.LayoutWrap == "WRAP" {
		l.Wrap = true
		l.CrossGap = snapPtr(f.CounterAxisSpacing)
		l.WrapAlign = strings.ToLower(f.CounterAxisAlignContent)
	}
	return l
}

func justify(v string) string {
	switch v {
	case "CENTER":
		return "center"
	case "MAX":
		return "end"
	case "SPACE_BETWEEN":
		return "space_between"
	}
	return "start"
}

func align(v string) string {
	switch v {
	case "CENTER":
		return "center"
	case "MAX":
		return "end"
	case "BASELINE":
		return "baseline"
	}
	return "start"
}

func sizeMode(v string) string {
	switch v {
	case "HUG":
		return designir.SizeHug
	case "FILL":
		return designir.SizeFill
	}
	return designir.SizeFixed
}

// sizing says how each axis is sized: explicit Figma modes first, then derived for older files; nil means fixed.
func sizing(f figma.Node, p *parent) *designir.Sizing {
	h, v := designir.SizeFixed, designir.SizeFixed
	explicit := f.LayoutSizingHorizontal != "" || f.LayoutSizingVertical != ""
	if explicit {
		h, v = sizeMode(f.LayoutSizingHorizontal), sizeMode(f.LayoutSizingVertical)
	} else {
		h, v = derivedSizing(f, p)
	}

	s := &designir.Sizing{Horizontal: designir.Axis{Mode: h}, Vertical: designir.Axis{Mode: v}}
	if f.MinWidth != nil {
		s.Horizontal.Min = snapPtr(*f.MinWidth)
	}
	if f.MaxWidth != nil {
		s.Horizontal.Max = snapPtr(*f.MaxWidth)
	}
	if f.MinHeight != nil {
		s.Vertical.Min = snapPtr(*f.MinHeight)
	}
	if f.MaxHeight != nil {
		s.Vertical.Max = snapPtr(*f.MaxHeight)
	}
	if h == designir.SizeFixed && v == designir.SizeFixed && s.Horizontal.Min == nil && s.Horizontal.Max == nil && s.Vertical.Min == nil && s.Vertical.Max == nil {
		return nil
	}
	return s
}

func derivedSizing(f figma.Node, p *parent) (string, string) {
	h, v := designir.SizeFixed, designir.SizeFixed
	horizontal := f.LayoutMode == "HORIZONTAL"
	if isAutoLayout(f.LayoutMode) {
		primary, counter := f.PrimaryAxisSizingMode == "AUTO", f.CounterAxisSizingMode == "AUTO"
		if horizontal {
			h, v = pick(primary), pick(counter)
		} else {
			v, h = pick(primary), pick(counter)
		}
	}
	if p != nil && p.autoLayout && f.LayoutPositioning != "ABSOLUTE" {
		if f.LayoutAlign == "STRETCH" {
			if p.horizontal {
				v = designir.SizeFill
			} else {
				h = designir.SizeFill
			}
		}
		if f.LayoutGrow > 0 {
			if p.horizontal {
				h = designir.SizeFill
			} else {
				v = designir.SizeFill
			}
		}
	}
	return h, v
}

func pick(hug bool) string {
	if hug {
		return designir.SizeHug
	}
	return designir.SizeFixed
}

// position says how a child sits in its parent: in the flow, or placed absolutely.
func position(f figma.Node, p *parent) *designir.Position {
	pos := &designir.Position{Mode: designir.PositionAbsolute}
	if p.autoLayout && f.LayoutPositioning != "ABSOLUTE" {
		pos.Mode = designir.PositionFlow
		if strings.EqualFold(f.LayoutAlign, "STRETCH") {
			pos.Align = "stretch"
		}
		if f.LayoutGrow > 0 {
			pos.Grow = snap(f.LayoutGrow)
		}
	}
	if c := f.Constraints; c != nil {
		pos.Constraints = &designir.Constraints{Horizontal: constraint(c.Horizontal), Vertical: constraint(c.Vertical)}
	}
	return pos
}

func constraint(v string) string { return strings.ToLower(v) }
