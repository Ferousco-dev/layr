package genplan

import (
	"slices"

	"github.com/ferousco-dev/layr/server/internal/designir"
)

// Validate checks a plan on its own and, when ir is given, against the design it was made from.
func Validate(p *Plan, ir *designir.DesignIR) error {
	bad := func(detail string) error { return fail(CodePlanInvalid, detail) }
	if p == nil {
		return bad("plan is missing")
	}
	if p.SchemaVersion != SchemaVersion {
		return bad("schema version is not supported")
	}
	if p.DesignVersion == "" {
		return bad("design version is missing")
	}
	if _, err := ValidateTarget(p.Target); err != nil || p.Target == (Target{}) {
		return fail(CodeTargetUnsupported, "only nextjs with typescript is supported")
	}
	if len(p.Selection.ScreenIDs) == 0 {
		return bad("selection resolved to no screens")
	}
	switch p.Selection.Mode {
	case ModeOne, ModeSelected, ModeFlow, ModeAll:
	default:
		return bad("selection mode is not valid")
	}
	screens := make(map[string]bool, len(p.Selection.ScreenIDs))
	for _, id := range p.Selection.ScreenIDs {
		if screens[id] {
			return bad("selection lists a screen twice")
		}
		screens[id] = true
	}

	g, err := p.Graph()
	if err != nil {
		return err
	}
	if !slices.Equal(g.order, p.Order) || !stagesEqual(g.stages, p.Stages) {
		return bad("order or stages do not match the dependencies")
	}

	assets := make(map[string]bool, len(p.Assets))
	for _, a := range p.Assets {
		if a.ID == "" || assets[a.ID] {
			return bad("asset list has an empty or duplicate id")
		}
		assets[a.ID] = true
	}
	units := make(map[string]*Unit, len(p.Units))
	for i := range p.Units {
		units[p.Units[i].ID] = &p.Units[i]
	}
	counts := map[string]int{}
	shownScreens := map[string]bool{}
	for i := range p.Units {
		u := &p.Units[i]
		counts[u.Type]++
		for _, a := range u.AssetIDs {
			if !assets[a] {
				return bad("unit " + u.ID + " needs an asset missing from the plan")
			}
		}
		for _, c := range u.ComponentIDs {
			if units[UnitID(UnitComponent, c)] == nil {
				return bad("unit " + u.ID + " needs a component missing from the plan")
			}
		}
		switch u.Type {
		case UnitComponent:
			if u.ComponentID == "" || u.ID != UnitID(UnitComponent, u.ComponentID) {
				return bad("component unit " + u.ID + " has a wrong identity")
			}
		case UnitScreen:
			if !screens[u.ScreenID] || u.ID != UnitID(UnitScreen, u.ScreenID) || u.IR == nil || u.IR.ScreenID != u.ScreenID {
				return bad("screen unit " + u.ID + " is not part of the selection")
			}
			shownScreens[u.ScreenID] = true
		case UnitVerification:
			if !screens[u.ScreenID] || u.Reference == nil || !safeIDPattern.MatchString(u.Reference.ID) {
				return bad("verification unit " + u.ID + " has no valid reference render")
			}
		}
	}
	if counts[UnitFoundation] != 1 || counts[UnitIntegration] != 1 || counts[UnitDesignFoundation] > 1 {
		return bad("the plan must have one foundation, one integration and at most one design foundation")
	}
	if len(shownScreens) != len(screens) {
		return bad("a selected screen has no unit")
	}
	if p.Summary != summarize(p) {
		return bad("summary does not match the plan")
	}
	if fp, err := Fingerprint(p); err != nil || fp != p.Fingerprint {
		return bad("fingerprint does not match the plan")
	}
	if ir != nil {
		return checkAgainst(p, ir)
	}
	return nil
}

func checkAgainst(p *Plan, ir *designir.DesignIR) error {
	screens := make(map[string]bool, len(ir.Screens))
	for i := range ir.Screens {
		screens[ir.Screens[i].ID] = true
	}
	comps := make(map[string]bool, len(ir.Components))
	for _, c := range ir.Components {
		comps[c.ID] = true
	}
	assets := make(map[string]bool, len(ir.Assets))
	for _, a := range ir.Assets {
		assets[a.ID] = true
	}
	for _, id := range p.Selection.ScreenIDs {
		if !screens[id] {
			return fail(CodePlanInvalid, "a selected screen is not in the design")
		}
	}
	for _, u := range p.Units {
		if u.Type == UnitComponent && !comps[u.ComponentID] {
			return fail(CodeDependencyInvalid, "unit "+u.ID+" is not a component of the design")
		}
		if u.IR != nil && !screens[u.IR.ScreenID] {
			return fail(CodePlanInvalid, "unit "+u.ID+" points at a screen that is not in the design")
		}
	}
	for _, a := range p.Assets {
		if !assets[a.ID] {
			return fail(CodeDependencyInvalid, "the plan needs an asset that is not in the design")
		}
	}
	return nil
}

func stagesEqual(a, b [][]string) bool {
	return slices.EqualFunc(a, b, func(x, y []string) bool { return slices.Equal(x, y) })
}
