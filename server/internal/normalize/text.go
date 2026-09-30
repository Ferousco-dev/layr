package normalize

import (
	"strconv"
	"strings"

	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
)

// text converts a TEXT node, keeping the characters exactly and any per-range styling.
func (n *normalizer) text(f figma.Node) *designir.Text {
	t := &designir.Text{Characters: f.Characters}
	if f.Style != nil {
		t.Style = n.textStyle(f.ID, *f.Style)
	}
	t.Runs = n.runs(f)
	return t
}

func (n *normalizer) textStyle(nodeID string, s figma.TypeStyle) designir.TextStyle {
	out := designir.TextStyle{
		FontFamily: s.FontFamily, FontPostScriptName: s.FontPostScriptName,
		Align: strings.ToLower(s.TextAlignHorizontal), VerticalAlign: strings.ToLower(s.TextAlignVertical),
		Case: strings.ToLower(s.TextCase), Decoration: strings.ToLower(s.TextDecoration), AutoResize: strings.ToLower(s.TextAutoResize),
		Fills: n.paints(figma.Node{ID: nodeID}, s.Fills),
	}
	if s.FontWeight != 0 {
		out.FontWeight = snapPtr(s.FontWeight)
	}
	if s.FontSize != 0 {
		out.FontSize = snapPtr(s.FontSize)
	}
	if s.Italic {
		v := true
		out.Italic = &v
	}
	out.LineHeight = lineHeight(s)
	if s.LetterSpacing != 0 {
		out.LetterSpacing = snapPtr(s.LetterSpacing)
	}
	if s.ParagraphSpacing != 0 {
		out.ParagraphSpacing = snapPtr(s.ParagraphSpacing)
	}
	if s.ParagraphIndent != 0 {
		out.ParagraphIndent = snapPtr(s.ParagraphIndent)
	}
	if h := s.Hyperlink; h != nil {
		out.Hyperlink = &designir.Hyperlink{Type: strings.ToLower(h.Type), URL: h.URL, NodeID: h.NodeID}
	}
	return out
}

// lineHeight keeps Figma's unit instead of flattening everything to pixels.
func lineHeight(s figma.TypeStyle) *designir.LineHeight {
	switch s.LineHeightUnit {
	case "PIXELS":
		return &designir.LineHeight{Mode: designir.LineHeightPixels, Value: snapPtr(s.LineHeightPx)}
	case "FONT_SIZE_%":
		return &designir.LineHeight{Mode: designir.LineHeightPercent, Value: snapPtr(s.LineHeightPercentFontSize)}
	case "INTRINSIC_%":
		return &designir.LineHeight{Mode: designir.LineHeightAuto}
	}
	if s.LineHeightPx != 0 {
		return &designir.LineHeight{Mode: designir.LineHeightPixels, Value: snapPtr(s.LineHeightPx)}
	}
	return nil
}

// runs turns Figma's per-character override list into contiguous ranges over code points.
func (n *normalizer) runs(f figma.Node) []designir.Run {
	overrides := f.CharacterStyleOverrides
	if len(overrides) == 0 || len(f.StyleOverrideTable) == 0 {
		return nil
	}
	offsets := utf16Offsets(f.Characters)
	count := len(offsets) - 1

	var keyAt func(cp int) int
	switch len(overrides) {
	case count:
		keyAt = func(cp int) int { return overrides[cp] }
	case offsets[count]:
		// Figma counts UTF-16 units, so a code point outside the basic plane spans two entries.
		keyAt = func(cp int) int { return overrides[offsets[cp]] }
	default:
		n.warn("MIXED_STYLE_PARTIAL", f.ID, "Text style ranges do not match the text length and were not applied.")
		return nil
	}
	return n.buildRuns(f, count, keyAt)
}

// buildRuns groups consecutive code points that share a non-zero override key.
func (n *normalizer) buildRuns(f figma.Node, count int, keyAt func(int) int) []designir.Run {
	var out []designir.Run
	start, current := 0, 0
	flush := func(end int) {
		if current == 0 || end <= start {
			return
		}
		style, ok := f.StyleOverrideTable[strconv.Itoa(current)]
		if !ok {
			n.warn("MIXED_STYLE_PARTIAL", f.ID, "A text style range refers to a missing style and was skipped.")
			return
		}
		out = append(out, designir.Run{Start: start, End: end, Style: n.textStyle(f.ID, style)})
	}
	for cp := 0; cp < count; cp++ {
		if key := keyAt(cp); key != current {
			flush(cp)
			start, current = cp, key
		}
	}
	flush(count)
	return out
}
