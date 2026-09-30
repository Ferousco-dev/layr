package genplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/ferousco-dev/layr/server/internal/designir"
)

// ValidateTarget accepts only the one target Layr can generate for.
func ValidateTarget(t Target) (Target, error) {
	if t == (Target{}) {
		return Target{Framework: FrameworkNextJS, Language: LanguageTypeScript}, nil
	}
	if t.Framework != FrameworkNextJS || t.Language != LanguageTypeScript {
		return Target{}, fail(CodeTargetUnsupported, "only nextjs with typescript is supported")
	}
	return t, nil
}

// Build plans the generation of a selection of ir. It only reads ir, so many plans may be built from one design at once.
func Build(ir *designir.DesignIR, req Request, lim Limits) (plan *Plan, err error) {
	defer func() {
		if r := recover(); r != nil {
			plan, err = nil, fail(CodePlanInvalid, "the design could not be planned")
		}
	}()
	lim = lim.withDefaults()
	target, err := ValidateTarget(req.Target)
	if err != nil {
		return nil, err
	}
	if ir == nil {
		return nil, fail(CodePlanInvalid, "design is missing")
	}
	if req.DesignVersion == "" {
		return nil, fail(CodeInvalidDesignVersion, "design version is required")
	}
	sel, err := ResolveSelection(ir, req.Selection)
	if err != nil {
		return nil, err
	}
	u := newUsage(ir, lim.MaxWork)
	for _, i := range sel.Index {
		s := &ir.Screens[i]
		if err := u.walk(s.ID, &s.Root, "", false); err != nil {
			return nil, err
		}
	}
	if err := u.explore(); err != nil {
		return nil, err
	}
	b := &builder{ir: ir, u: u, sel: sel}
	units, edges, assets, warnings, err := b.assemble()
	if err != nil {
		return nil, err
	}
	if len(units) > lim.MaxUnits {
		return nil, fail(CodePlanTooLarge, fmt.Sprintf("the plan would have %d units; the limit is %d", len(units), lim.MaxUnits))
	}
	g, err := NewGraph(units, edges)
	if err != nil {
		return nil, err
	}
	plan = &Plan{
		SchemaVersion: SchemaVersion, DesignVersion: req.DesignVersion,
		Selection: PlanSelection{Mode: sel.Mode, FlowID: sel.FlowID, ScreenIDs: sel.ScreenIDs},
		Target:    target, Units: units, Dependencies: edges, Order: g.Order(), Stages: g.Stages(), Assets: assets, Warnings: warnings,
	}
	plan.Summary = summarize(plan)
	if plan.Fingerprint, err = Fingerprint(plan); err != nil {
		return nil, err
	}
	if err := Validate(plan, ir); err != nil {
		return nil, err
	}
	return plan, nil
}

type builder struct {
	ir  *designir.DesignIR
	u   *usage
	sel *ResolvedSelection
}

func (b *builder) assemble() ([]Unit, []Edge, []AssetRef, []Warning, error) {
	ir, u := b.ir, b.u
	compPos := make(map[string]int, len(ir.Components))
	for i := range ir.Components {
		compPos[ir.Components[i].ID] = i
	}
	assetPos := make(map[string]int, len(ir.Assets))
	for i := range ir.Assets {
		assetPos[ir.Assets[i].ID] = i
	}
	sets := make(map[string]bool, len(ir.ComponentSets))
	for _, s := range ir.ComponentSets {
		sets[s.ID] = true
	}
	byComp := func(ids map[string]bool) []string { return sortedBy(ids, compPos) }

	var units []Unit
	var edges []Edge
	var warnings []Warning
	units = append(units, Unit{ID: "foundation", Type: UnitFoundation, Name: "Project foundation"})
	base := "foundation"
	if t := tokenCounts(ir); t != nil {
		units = append(units, Unit{ID: "design_foundation", Type: UnitDesignFoundation, Name: "Design foundation", Tokens: t})
		edges = append(edges, Edge{Unit: "design_foundation", DependsOn: "foundation"})
		base = "design_foundation"
	}

	nested := map[string]map[string]bool{}
	for e := range u.edges {
		set(nested, e[0])[e[1]] = true
	}
	usedAssets := map[string]bool{}
	note := func(ids map[string]bool) []string {
		for id := range ids {
			if _, ok := assetPos[id]; !ok {
				return nil
			}
			usedAssets[id] = true
		}
		return sortedBy(ids, assetPos)
	}
	checkAssets := func(ids map[string]bool) error {
		for id := range ids {
			if _, ok := assetPos[id]; !ok {
				return fail(CodeDependencyInvalid, "a design node uses an asset that is not in the design")
			}
		}
		return nil
	}

	for _, id := range byComp(u.used) {
		c := u.comps[id]
		if c.SetID != "" && !sets[c.SetID] {
			return nil, nil, nil, nil, fail(CodeDependencyInvalid, "component "+UnitID(UnitComponent, id)+" belongs to a missing set")
		}
		if err := checkAssets(u.compAssets[id]); err != nil {
			return nil, nil, nil, nil, err
		}
		unit := Unit{ID: UnitID(UnitComponent, id), Type: UnitComponent, Name: c.Name, ComponentID: id, SetID: c.SetID}
		if ref, ok := u.defs[id]; ok {
			unit.IR = &IRRef{ScreenID: ref.screenID, NodeID: ref.node.ID, Origin: "definition"}
		} else if ref, ok := u.firstInst[id]; ok {
			unit.IR = &IRRef{ScreenID: ref.screenID, NodeID: ref.node.ID, Origin: "instance"}
		}
		if deps := nested[id]; len(deps) > 0 {
			unit.ComponentIDs = byComp(deps)
			for _, d := range unit.ComponentIDs {
				edges = append(edges, Edge{Unit: unit.ID, DependsOn: UnitID(UnitComponent, d)})
			}
		} else {
			edges = append(edges, Edge{Unit: unit.ID, DependsOn: base})
		}
		unit.AssetIDs = note(u.compAssets[id])
		units = append(units, unit)
	}

	compAttributed := map[string]bool{}
	for _, set := range u.compAssets {
		for id := range set {
			compAttributed[id] = true
		}
	}
	var screenUnits []Unit
	for _, i := range b.sel.Index {
		s := &ir.Screens[i]
		assets := map[string]bool{}
		for id := range u.screenAssets[s.ID] {
			assets[id] = true
		}
		for _, id := range s.AssetIDs {
			if !compAttributed[id] {
				assets[id] = true
			}
		}
		if err := checkAssets(assets); err != nil {
			return nil, nil, nil, nil, err
		}
		unit := Unit{
			ID: UnitID(UnitScreen, s.ID), Type: UnitScreen, Name: s.Name, ScreenID: s.ID,
			IR: &IRRef{ScreenID: s.ID, NodeID: s.Root.ID, Origin: "screen"}, ComponentIDs: byComp(u.screenComps[s.ID]), AssetIDs: note(assets),
		}
		if s.Reference != nil {
			unit.Reference = &Reference{ID: "ref_" + safeID(s.ID), SHA256: s.Reference.SHA256, Width: s.Reference.Width, Height: s.Reference.Height}
		} else {
			warnings = append(warnings, Warning{Code: "REFERENCE_MISSING", ScreenID: s.ID, Message: "This screen has no reference render, so it cannot be verified visually."})
		}
		if len(unit.ComponentIDs) == 0 {
			edges = append(edges, Edge{Unit: unit.ID, DependsOn: base})
		}
		for _, c := range unit.ComponentIDs {
			edges = append(edges, Edge{Unit: unit.ID, DependsOn: UnitID(UnitComponent, c)})
		}
		screenUnits = append(screenUnits, unit)
	}
	units = append(units, screenUnits...)

	units = append(units, Unit{ID: "integration", Type: UnitIntegration, Name: "Integration"})
	for _, s := range screenUnits {
		edges = append(edges, Edge{Unit: "integration", DependsOn: s.ID})
	}
	for _, s := range screenUnits {
		if s.Reference == nil {
			continue
		}
		v := Unit{ID: UnitID(UnitVerification, s.ScreenID), Type: UnitVerification, Name: "Verify " + s.Name, ScreenID: s.ScreenID, Reference: s.Reference}
		units = append(units, v)
		edges = append(edges, Edge{Unit: v.ID, DependsOn: "integration"})
	}

	rank := make(map[string]int, len(units))
	for i, un := range units {
		rank[un.ID] = i
	}
	sort.SliceStable(edges, func(a, b int) bool {
		if ra, rb := rank[edges[a].Unit], rank[edges[b].Unit]; ra != rb {
			return ra < rb
		}
		return rank[edges[a].DependsOn] < rank[edges[b].DependsOn]
	})
	return units, edges, assetRefs(ir, usedAssets, assetPos), warnings, nil
}

func assetRefs(ir *designir.DesignIR, used map[string]bool, pos map[string]int) []AssetRef {
	out := make([]AssetRef, 0, len(used))
	for _, id := range sortedBy(used, pos) {
		a := ir.Assets[pos[id]]
		out = append(out, AssetRef{ID: a.ID, Kind: a.Kind, Format: a.Format, MediaType: a.MediaType, SizeBytes: a.SizeBytes, SHA256: a.SHA256})
	}
	return out
}

func tokenCounts(ir *designir.DesignIR) *TokenCounts {
	t := TokenCounts{
		Colors: len(ir.Tokens.Colors), Typography: len(ir.Tokens.Typography), Spacing: len(ir.Tokens.Spacing),
		Radii: len(ir.Tokens.Radii), Shadows: len(ir.Tokens.Shadows), Styles: len(ir.Styles),
	}
	if t == (TokenCounts{}) {
		return nil
	}
	return &t
}

// sortedBy returns the keys of ids ordered by their position in the design.
func sortedBy(ids map[string]bool, pos map[string]int) []string {
	out := make([]string, 0, len(ids))
	for id := range ids {
		out = append(out, id)
	}
	sort.Slice(out, func(a, b int) bool {
		if pa, pb := pos[out[a]], pos[out[b]]; pa != pb {
			return pa < pb
		}
		return out[a] < out[b]
	})
	return out
}

func summarize(p *Plan) Summary {
	s := Summary{Screens: len(p.Selection.ScreenIDs), Assets: len(p.Assets), Units: len(p.Units), Dependencies: len(p.Dependencies), Stages: len(p.Stages)}
	for _, u := range p.Units {
		if u.Type == UnitComponent {
			s.SharedComponents++
		}
	}
	for _, st := range p.Stages {
		if len(st) > s.MaxParallel {
			s.MaxParallel = len(st)
		}
	}
	return s
}

// Fingerprint is a SHA-256 over what the plan means: design version, selection, target and structure.
// It leaves out the database ID, project and timestamps, so equal input gives an equal fingerprint.
func Fingerprint(p *Plan) (string, error) {
	data, err := json.Marshal(struct {
		SchemaVersion int           `json:"schema_version"`
		DesignVersion string        `json:"design_version"`
		Selection     PlanSelection `json:"selection"`
		Target        Target        `json:"target"`
		Units         []Unit        `json:"units"`
		Dependencies  []Edge        `json:"dependencies"`
		Assets        []AssetRef    `json:"assets"`
	}{p.SchemaVersion, p.DesignVersion, p.Selection, p.Target, p.Units, p.Dependencies, p.Assets})
	if err != nil {
		return "", fail(CodePlanInvalid, "the plan could not be encoded")
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
