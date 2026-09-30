package genplan

import (
	"sort"
)

// Graph is the validated dependency DAG of a plan.
type Graph struct {
	units      []Unit
	index      map[string]int
	deps       map[string][]string
	dependents map[string][]string
	order      []string
	stages     [][]string
}

// NewGraph validates units and edges and computes the deterministic order and stages.
// Units must already be listed in their canonical order; it breaks every tie.
func NewGraph(units []Unit, edges []Edge) (*Graph, error) {
	g := &Graph{units: units, index: make(map[string]int, len(units)), deps: map[string][]string{}, dependents: map[string][]string{}}
	for i, u := range units {
		if !safeIDPattern.MatchString(u.ID) {
			return nil, fail(CodeDependencyInvalid, "a unit id is empty or not safe")
		}
		if _, dup := g.index[u.ID]; dup {
			return nil, fail(CodeDependencyInvalid, "duplicate unit "+u.ID)
		}
		if _, ok := unitRank[u.Type]; !ok {
			return nil, fail(CodeDependencyInvalid, "unit "+u.ID+" has an invalid type")
		}
		g.index[u.ID] = i
	}
	seen := make(map[Edge]bool, len(edges))
	for _, e := range edges {
		if _, ok := g.index[e.Unit]; !ok {
			return nil, fail(CodeDependencyInvalid, "dependency of a missing unit "+e.Unit)
		}
		if _, ok := g.index[e.DependsOn]; !ok {
			return nil, fail(CodeDependencyInvalid, e.Unit+" depends on a missing unit "+e.DependsOn)
		}
		if e.Unit == e.DependsOn {
			return nil, fail(CodeDependencyInvalid, e.Unit+" depends on itself")
		}
		if seen[e] {
			return nil, fail(CodeDependencyInvalid, "duplicate dependency "+e.Unit+" on "+e.DependsOn)
		}
		seen[e] = true
		g.deps[e.Unit] = append(g.deps[e.Unit], e.DependsOn)
		g.dependents[e.DependsOn] = append(g.dependents[e.DependsOn], e.Unit)
	}
	for _, m := range []map[string][]string{g.deps, g.dependents} {
		for k := range m {
			g.sortIDs(m[k])
		}
	}
	return g, g.levels()
}

func (g *Graph) sortIDs(ids []string) {
	sort.Slice(ids, func(a, b int) bool { return g.index[ids[a]] < g.index[ids[b]] })
}

// levels runs Kahn's algorithm one layer at a time; a layer holds units that can run together.
func (g *Graph) levels() error {
	left := make(map[string]int, len(g.units))
	var current []string
	for _, u := range g.units {
		left[u.ID] = len(g.deps[u.ID])
		if left[u.ID] == 0 {
			current = append(current, u.ID)
		}
	}
	placed := 0
	for len(current) > 0 {
		g.sortIDs(current)
		g.stages = append(g.stages, current)
		g.order = append(g.order, current...)
		placed += len(current)
		var next []string
		for _, id := range current {
			for _, d := range g.dependents[id] {
				if left[d]--; left[d] == 0 {
					next = append(next, d)
				}
			}
		}
		current = next
	}
	if placed == len(g.units) {
		return nil
	}
	return g.cycle(left)
}

// cycle names one cycle among the units that never became ready; every one of them still waits on another.
func (g *Graph) cycle(left map[string]int) error {
	blocked := func(id string) bool { return left[id] > 0 }
	start := ""
	for _, u := range g.units {
		if blocked(u.ID) {
			start = u.ID
			break
		}
	}
	at := map[string]int{}
	var path []string
	for cur := start; ; {
		if i, ok := at[cur]; ok {
			return fail(CodeDependencyCycle, joinIDs(append(path[i:], cur)))
		}
		at[cur] = len(path)
		path = append(path, cur)
		next := ""
		for _, d := range g.deps[cur] {
			if blocked(d) {
				next = d
				break
			}
		}
		if next == "" {
			return fail(CodeDependencyCycle, start)
		}
		cur = next
	}
}

// Order lists every unit so that dependencies come first.
func (g *Graph) Order() []string { return append([]string(nil), g.order...) }

// Stages groups units that may run at the same time once earlier stages are done.
func (g *Graph) Stages() [][]string {
	out := make([][]string, len(g.stages))
	for i, s := range g.stages {
		out[i] = append([]string(nil), s...)
	}
	return out
}

// DependsOn returns what a unit waits for.
func (g *Graph) DependsOn(id string) []string { return append([]string(nil), g.deps[id]...) }

// Dependents returns the units that wait for id.
func (g *Graph) Dependents(id string) []string { return append([]string(nil), g.dependents[id]...) }

// Ready returns the units not yet done whose dependencies are all done, in plan order.
// Units that are not ready are blocked; everything returned together may run concurrently.
func (g *Graph) Ready(done map[string]bool) []string {
	var out []string
	for _, id := range g.order {
		if done[id] {
			continue
		}
		ok := true
		for _, d := range g.deps[id] {
			if !done[d] {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, id)
		}
	}
	return out
}

// Graph rebuilds the validated graph of a plan.
func (p *Plan) Graph() (*Graph, error) { return NewGraph(p.Units, p.Dependencies) }
