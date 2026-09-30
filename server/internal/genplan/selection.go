package genplan

import (
	"sort"

	"github.com/ferousco-dev/layr/server/internal/designir"
)

// maxRequestedScreens bounds the list a caller may send, before any lookups.
const maxRequestedScreens = 5000

// ResolvedSelection is a selection turned into screens of one design, in design order.
type ResolvedSelection struct {
	Mode      string
	FlowID    string
	ScreenIDs []string
	// Index holds each chosen screen's position in the design.
	Index []int
}

// eligible screens have a visible, sized root; the others cannot be generated.
func eligible(s *designir.Screen) bool {
	return s.Root.ID != "" && s.Root.Visible && s.Width > 0 && s.Height > 0
}

// ResolveSelection maps a selection onto the screens of ir. It is deterministic and never modifies ir.
func ResolveSelection(ir *designir.DesignIR, sel Selection) (*ResolvedSelection, error) {
	if ir == nil {
		return nil, fail(CodePlanInvalid, "design is missing")
	}
	pos := make(map[string]int, len(ir.Screens))
	for i := range ir.Screens {
		if _, dup := pos[ir.Screens[i].ID]; dup || ir.Screens[i].ID == "" {
			return nil, fail(CodePlanInvalid, "design has a duplicate or empty screen id")
		}
		pos[ir.Screens[i].ID] = i
	}
	out := &ResolvedSelection{Mode: sel.Mode}
	var idx []int

	switch sel.Mode {
	case ModeOne:
		if sel.ScreenIDs != nil || sel.FlowID != "" {
			return nil, fail(CodeInvalidSelection, "mode one takes only screen_id")
		}
		i, err := lookup(ir, pos, sel.ScreenID)
		if err != nil {
			return nil, err
		}
		idx = []int{i}
	case ModeSelected:
		if sel.ScreenID != "" || sel.FlowID != "" {
			return nil, fail(CodeInvalidSelection, "mode selected takes only screen_ids")
		}
		if len(sel.ScreenIDs) == 0 {
			return nil, fail(CodeEmptySelection, "no screens were chosen")
		}
		if len(sel.ScreenIDs) > maxRequestedScreens {
			return nil, fail(CodeInvalidSelection, "too many screens in one request")
		}
		seen := map[int]bool{}
		for _, id := range sel.ScreenIDs {
			i, err := lookup(ir, pos, id)
			if err != nil {
				return nil, err
			}
			if !seen[i] {
				seen[i] = true
				idx = append(idx, i)
			}
		}
	case ModeFlow:
		if sel.ScreenID != "" || sel.ScreenIDs != nil {
			return nil, fail(CodeInvalidSelection, "mode flow takes only flow_id")
		}
		if !idPattern.MatchString(sel.FlowID) {
			return nil, fail(CodeInvalidSelection, "flow_id is not valid")
		}
		found := false
		for _, f := range ir.Sections {
			if f.ID != sel.FlowID {
				continue
			}
			found = true
			for _, id := range f.ScreenIDs {
				if i, ok := pos[id]; ok && eligible(&ir.Screens[i]) {
					idx = append(idx, i)
				}
			}
			break
		}
		if !found {
			return nil, fail(CodeFlowNotFound, "the flow is not in this design")
		}
		out.FlowID = sel.FlowID
		idx = uniqueSorted(idx)
	case ModeAll:
		if sel.ScreenID != "" || sel.ScreenIDs != nil || sel.FlowID != "" {
			return nil, fail(CodeInvalidSelection, "mode all takes no other fields")
		}
		for i := range ir.Screens {
			if eligible(&ir.Screens[i]) {
				idx = append(idx, i)
			}
		}
	default:
		return nil, fail(CodeInvalidSelection, "mode must be one, selected, flow or all")
	}

	if len(idx) == 0 {
		return nil, fail(CodeEmptySelection, "no eligible screens were chosen")
	}
	sort.Ints(idx)
	out.Index = idx
	out.ScreenIDs = make([]string, len(idx))
	for n, i := range idx {
		out.ScreenIDs[n] = ir.Screens[i].ID
	}
	return out, nil
}

func lookup(ir *designir.DesignIR, pos map[string]int, id string) (int, error) {
	if !idPattern.MatchString(id) {
		return 0, fail(CodeInvalidSelection, "a screen id is not valid")
	}
	i, ok := pos[id]
	if !ok {
		return 0, fail(CodeScreenNotFound, "a chosen screen is not in this design")
	}
	if !eligible(&ir.Screens[i]) {
		return 0, fail(CodeInvalidSelection, "a chosen screen cannot be generated")
	}
	return i, nil
}

func uniqueSorted(v []int) []int {
	sort.Ints(v)
	out := v[:0]
	for i, x := range v {
		if i == 0 || x != v[i-1] {
			out = append(out, x)
		}
	}
	return out
}
