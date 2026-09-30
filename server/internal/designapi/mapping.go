package designapi

import (
	"strings"

	"github.com/ferousco-dev/layr/server/internal/designir"
)

// warningMessages are the only texts clients see for design warnings, so IR wording can change freely.
var warningMessages = map[string]string{
	"UNKNOWN_NODE_TYPE":          "This design contains an element type Layr does not recognise yet.",
	"UNSUPPORTED_PAINT":          "This design uses a fill (such as video or a pattern) that Layr does not fully support yet.",
	"UNSUPPORTED_EFFECT":         "This design uses a visual effect that Layr does not support yet.",
	"UNSUPPORTED_MASK":           "This design uses a mask that Layr keeps but does not apply yet.",
	"UNSUPPORTED_LAYOUT_GRID":    "This design uses a grid layout whose details Layr does not capture yet.",
	"UNKNOWN_BLEND_MODE":         "This design uses a blend mode Layr does not recognise.",
	"MISSING_ASSET":              "An image or graphic in this design could not be retrieved.",
	"MISSING_BOUNDS":             "An element in this design has no size information.",
	"MIXED_STYLE_PARTIAL":        "Some mixed text styling in this design could not be read.",
	"ROTATED_BOUNDS_APPROXIMATE": "A rotated element's size is approximate.",
}

const genericWarning = "This design contains something Layr does not fully support yet."

func warningMessage(code string) string {
	if m, ok := warningMessages[code]; ok {
		return m
	}
	if strings.HasPrefix(code, "ASSET_") {
		return "An image or graphic in this design is not fully supported yet."
	}
	return genericWarning
}

func mapWarning(w designir.Warning) Warning {
	return Warning{Code: publicCode(w.Code), ScreenID: w.ScreenID, Message: warningMessage(w.Code)}
}

// publicCode keeps known codes and folds anything else into one generic code.
func publicCode(code string) string {
	if _, ok := warningMessages[code]; ok {
		return code
	}
	if strings.HasPrefix(code, "ASSET_") {
		return "ASSET_NOT_FULLY_SUPPORTED"
	}
	return "DESIGN_NOT_FULLY_SUPPORTED"
}

func (ix *index) previewURL(projectID, screenID string) string {
	return "/api/v1/projects/" + projectID + "/design/screens/" + screenID + "/preview?v=" + ix.version
}

func (ix *index) screenDTO(projectID string, i int) Screen {
	s := ix.ir.Screens[i]
	out := Screen{
		ID: s.ID, Name: s.Name, SourceNodeID: s.SourceNodeID, Page: s.Page, FlowID: ix.flowOf[s.ID], Index: i,
		Width: s.Width, Height: s.Height, WarningCount: len(ix.warnings[s.ID]),
	}
	if s.Reference != nil && previewFile(s.Reference.Path) != "" {
		out.Preview = Preview{Available: true, Width: s.Reference.Width, Height: s.Reference.Height, MediaType: "image/png", URL: ix.previewURL(projectID, s.ID)}
	}
	return out
}

func (ix *index) flowDTO(i int) Flow {
	f := ix.ir.Sections[i]
	return Flow{ID: f.ID, Name: f.Name, SourceNodeID: f.SourceNodeID, ScreenIDs: append([]string{}, f.ScreenIDs...)}
}

func (ix *index) designSystem() *DesignSystem {
	t := ix.ir.Tokens
	return &DesignSystem{Colors: len(t.Colors), TextStyles: len(t.Typography), Spacing: len(t.Spacing), Radii: len(t.Radii), Shadows: len(t.Shadows)}
}

func (ix *index) sharedComponents() int {
	n := 0
	for _, c := range ix.ir.Components {
		if len(c.ScreenIDs) > 1 {
			n++
		}
	}
	return n
}

func countNodes(n *designir.Node) int {
	total := 1
	for i := range n.Children {
		total += countNodes(&n.Children[i])
	}
	return total
}
