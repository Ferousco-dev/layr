package genplan

import "github.com/ferousco-dev/layr/server/internal/designir"

// instRef remembers where a component was first used, so its unit can point at real IR.
type instRef struct {
	screenID string
	node     *designir.Node
}

// usage is everything the selected screens need, discovered by walking the IR once per subtree.
type usage struct {
	ir           *designir.DesignIR
	comps        map[string]*designir.Component
	assets       map[string]*designir.Asset
	screens      map[string]*designir.Screen
	work, budget int

	screenComps  map[string]map[string]bool
	screenAssets map[string]map[string]bool
	compAssets   map[string]map[string]bool
	edges        map[[2]string]bool
	used         map[string]bool
	explored     map[string]bool
	firstInst    map[string]instRef
	defs         map[string]instRef
	queue        []string
}

func newUsage(ir *designir.DesignIR, budget int) *usage {
	u := &usage{
		ir: ir, budget: budget,
		comps: map[string]*designir.Component{}, assets: map[string]*designir.Asset{}, screens: map[string]*designir.Screen{},
		screenComps: map[string]map[string]bool{}, screenAssets: map[string]map[string]bool{}, compAssets: map[string]map[string]bool{},
		edges: map[[2]string]bool{}, used: map[string]bool{}, explored: map[string]bool{},
		firstInst: map[string]instRef{}, defs: map[string]instRef{},
	}
	for i := range ir.Components {
		u.comps[ir.Components[i].ID] = &ir.Components[i]
	}
	for i := range ir.Assets {
		u.assets[ir.Assets[i].ID] = &ir.Assets[i]
	}
	for i := range ir.Screens {
		u.screens[ir.Screens[i].ID] = &ir.Screens[i]
	}
	return u
}

type frame struct {
	n   *designir.Node
	ctx string
}

// walk visits a subtree without recursion. ctx is the component the subtree belongs to ("" for the screen itself).
// When skipSelf is set the node is the component's own root, so it is not registered as a use of itself.
func (u *usage) walk(screenID string, root *designir.Node, ctx string, skipSelf bool) error {
	stack := []frame{{root, ctx}}
	first := true
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if u.work++; u.work > u.budget {
			return fail(CodePlanTooLarge, "the design needs more planning work than allowed")
		}
		n, owner := f.n, f.ctx
		if !(first && skipSelf) {
			id, origin := "", ""
			if n.Component != nil && n.Component.ComponentID != "" {
				id, origin = n.Component.ComponentID, "definition"
			} else if n.Instance != nil && n.Instance.ComponentID != "" {
				id, origin = n.Instance.ComponentID, "instance"
			}
			if id != "" {
				if err := u.register(screenID, owner, id, origin, n); err != nil {
					return err
				}
				owner = id
			}
		}
		first = false
		u.useAssets(screenID, owner, n)
		for i := len(n.Children) - 1; i >= 0; i-- {
			stack = append(stack, frame{&n.Children[i], owner})
		}
	}
	return nil
}

func (u *usage) register(screenID, owner, id, origin string, n *designir.Node) error {
	if _, ok := u.comps[id]; !ok {
		return fail(CodeDependencyInvalid, "a screen uses a component that is not in the design")
	}
	if owner == id {
		return fail(CodeDependencyCycle, UnitID(UnitComponent, id)+" contains itself")
	}
	if !u.used[id] {
		u.used[id] = true
		u.queue = append(u.queue, id)
	}
	if owner == "" {
		set(u.screenComps, screenID)[id] = true
	} else {
		u.edges[[2]string{owner, id}] = true
	}
	switch origin {
	case "definition":
		if _, ok := u.defs[id]; !ok {
			u.defs[id] = instRef{screenID, n}
		}
		u.explored[id] = true
	default:
		if _, ok := u.firstInst[id]; !ok {
			u.firstInst[id] = instRef{screenID, n}
		}
	}
	return nil
}

func set(m map[string]map[string]bool, key string) map[string]bool {
	s := m[key]
	if s == nil {
		s = map[string]bool{}
		m[key] = s
	}
	return s
}

// useAssets attributes the node's own assets to the nearest component, or to the screen.
func (u *usage) useAssets(screenID, owner string, n *designir.Node) {
	add := func(id string) {
		if id == "" {
			return
		}
		if owner == "" {
			set(u.screenAssets, screenID)[id] = true
		} else {
			set(u.compAssets, owner)[id] = true
		}
	}
	if n.Asset != nil && !n.Asset.Missing {
		add(n.Asset.AssetID)
	}
	paints := func(ps []designir.Paint) {
		for _, p := range ps {
			if p.Image != nil && !p.Image.Missing {
				add(p.Image.AssetID)
			}
		}
	}
	if n.Appearance != nil {
		paints(n.Appearance.Fills)
		if n.Appearance.Stroke != nil {
			paints(n.Appearance.Stroke.Paints)
		}
	}
	if n.Text != nil {
		paints(n.Text.Style.Fills)
		for _, r := range n.Text.Runs {
			paints(r.Style.Fills)
		}
	}
}

// explore follows components used by components until nothing new appears.
func (u *usage) explore() error {
	for len(u.queue) > 0 {
		id := u.queue[0]
		u.queue = u.queue[1:]
		if u.explored[id] {
			continue
		}
		u.explored[id] = true
		c := u.comps[id]
		if ref, ok := u.findDefinition(c); ok {
			if err := u.walk(ref.screenID, ref.node, id, true); err != nil {
				return err
			}
			u.defs[id] = ref
			continue
		}
		if ref, ok := u.firstInst[id]; ok {
			if err := u.walk(ref.screenID, ref.node, id, true); err != nil {
				return err
			}
		}
	}
	return nil
}

// findDefinition locates the master node named by the component, when it lives in an imported screen.
func (u *usage) findDefinition(c *designir.Component) (instRef, bool) {
	if c.DefinitionScreenID == "" || c.DefinitionNodeID == "" {
		return instRef{}, false
	}
	s, ok := u.screens[c.DefinitionScreenID]
	if !ok {
		return instRef{}, false
	}
	stack := []*designir.Node{&s.Root}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if u.work++; u.work > u.budget {
			return instRef{}, false
		}
		if n.ID == c.DefinitionNodeID {
			return instRef{s.ID, n}, true
		}
		for i := range n.Children {
			stack = append(stack, &n.Children[i])
		}
	}
	return instRef{}, false
}
