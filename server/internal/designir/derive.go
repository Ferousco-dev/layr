package designir

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
)

const maxWarnings = 500
const maxTokens = 200

// Finalize recomputes everything derived from the screens: usage indexes, tokens, statistics and ordering.
func (ir *DesignIR) Finalize() {
	ir.SchemaVersion = SchemaVersion
	d := newDeriver(ir)
	for i := range ir.Screens {
		d.screen(&ir.Screens[i])
	}
	d.apply()
}

type deriver struct {
	ir         *DesignIR
	comps      map[string]*compUse
	assets     map[string]map[string]bool
	styles     map[string]bool
	colors     map[Color]int
	typography map[string]*TypographyToken
	spacing    map[float64]int
	radii      map[float64]int
	shadows    map[string]*ShadowToken
	nodes      int
}

type compUse struct {
	instances int
	screens   map[string]bool
	defScreen string
	defNode   string
}

func newDeriver(ir *DesignIR) *deriver {
	return &deriver{
		ir: ir, comps: map[string]*compUse{}, assets: map[string]map[string]bool{}, styles: map[string]bool{},
		colors: map[Color]int{}, typography: map[string]*TypographyToken{}, spacing: map[float64]int{},
		radii: map[float64]int{}, shadows: map[string]*ShadowToken{},
	}
}

func (d *deriver) screen(s *Screen) {
	componentIDs, assetIDs := map[string]bool{}, map[string]bool{}
	var walk func(n *Node)
	walk = func(n *Node) {
		d.nodes++
		if n.Children == nil {
			n.Children = []Node{}
		}
		d.node(s, n, componentIDs, assetIDs)
		for i := range n.Children {
			walk(&n.Children[i])
		}
	}
	walk(&s.Root)
	s.ComponentIDs, s.AssetIDs = sortedKeys(componentIDs), sortedKeys(assetIDs)
}

func (d *deriver) node(s *Screen, n *Node, componentIDs, assetIDs map[string]bool) {
	if n.Component != nil && n.Component.ComponentID != "" {
		use := d.comp(n.Component.ComponentID)
		use.screens[s.ID] = true
		if use.defNode == "" {
			use.defScreen, use.defNode = s.ID, n.ID
		}
		componentIDs[n.Component.ComponentID] = true
	}
	if n.Instance != nil && n.Instance.ComponentID != "" {
		use := d.comp(n.Instance.ComponentID)
		use.instances++
		use.screens[s.ID] = true
		componentIDs[n.Instance.ComponentID] = true
	}
	if n.Asset != nil && n.Asset.AssetID != "" {
		d.useAsset(n.Asset.AssetID, s.ID, assetIDs)
	}
	for _, r := range n.StyleRefs {
		d.styles[r.StyleID] = true
	}
	d.layoutTokens(n)
	if n.Appearance != nil {
		d.appearanceTokens(s, n, assetIDs)
	}
	if n.Text != nil {
		d.textTokens(s, n, assetIDs)
	}
}

func (d *deriver) comp(id string) *compUse {
	use := d.comps[id]
	if use == nil {
		use = &compUse{screens: map[string]bool{}}
		d.comps[id] = use
	}
	return use
}

func (d *deriver) useAsset(id, screenID string, assetIDs map[string]bool) {
	assetIDs[id] = true
	if d.assets[id] == nil {
		d.assets[id] = map[string]bool{}
	}
	d.assets[id][screenID] = true
}

func (d *deriver) layoutTokens(n *Node) {
	if n.Layout == nil {
		return
	}
	l := n.Layout
	for _, v := range []*float64{l.Gap, l.CrossGap} {
		if v != nil && *v > 0 {
			d.spacing[*v]++
		}
	}
	if l.Padding != nil {
		for _, v := range []float64{l.Padding.Top, l.Padding.Right, l.Padding.Bottom, l.Padding.Left} {
			if v > 0 {
				d.spacing[v]++
			}
		}
	}
}

func (d *deriver) appearanceTokens(s *Screen, n *Node, assetIDs map[string]bool) {
	a := n.Appearance
	d.paintTokens(s, a.Fills, assetIDs)
	if a.Stroke != nil {
		d.paintTokens(s, a.Stroke.Paints, assetIDs)
	}
	if r := a.Radius; r != nil {
		for _, v := range []float64{r.TopLeft, r.TopRight, r.BottomRight, r.BottomLeft} {
			if v > 0 {
				d.radii[v]++
			}
		}
	}
	for _, e := range a.Effects {
		if e.Type != EffectDropShadow && e.Type != EffectInnerShadow {
			continue
		}
		key := fmt.Sprintf("%s|%g|%g|%g|%g|%v", e.Type, e.OffsetX, e.OffsetY, e.Radius, e.Spread, e.Color)
		if e.Color != nil {
			key = fmt.Sprintf("%s|%g|%g|%g|%g|%d,%d,%d,%g", e.Type, e.OffsetX, e.OffsetY, e.Radius, e.Spread, e.Color.R, e.Color.G, e.Color.B, e.Color.A)
		}
		t := d.shadows[key]
		if t == nil {
			t = &ShadowToken{ID: "shadow_" + shortHash(key), Type: e.Type, OffsetX: e.OffsetX, OffsetY: e.OffsetY, Radius: e.Radius, Spread: e.Spread, Color: e.Color}
			d.shadows[key] = t
		}
		t.Count++
	}
}

func (d *deriver) paintTokens(s *Screen, paints []Paint, assetIDs map[string]bool) {
	for _, p := range paints {
		if p.Type == PaintSolid && p.Color != nil && p.Visible {
			d.colors[*p.Color]++
		}
		if p.Image != nil && p.Image.AssetID != "" {
			d.useAsset(p.Image.AssetID, s.ID, assetIDs)
		}
	}
}

func (d *deriver) textTokens(s *Screen, n *Node, assetIDs map[string]bool) {
	d.paintTokens(s, n.Text.Style.Fills, assetIDs)
	d.typographyToken(n.Text.Style)
	for _, r := range n.Text.Runs {
		d.paintTokens(s, r.Style.Fills, assetIDs)
		if r.Style.FontFamily != "" && r.Style.FontSize != nil {
			d.typographyToken(mergeStyle(n.Text.Style, r.Style))
		}
	}
}

// mergeStyle applies the parts of override that are set on top of base.
func mergeStyle(base, override TextStyle) TextStyle {
	out := base
	if override.FontFamily != "" {
		out.FontFamily = override.FontFamily
	}
	if override.FontWeight != nil {
		out.FontWeight = override.FontWeight
	}
	if override.FontSize != nil {
		out.FontSize = override.FontSize
	}
	if override.LineHeight != nil {
		out.LineHeight = override.LineHeight
	}
	if override.LetterSpacing != nil {
		out.LetterSpacing = override.LetterSpacing
	}
	return out
}

func (d *deriver) typographyToken(st TextStyle) {
	if st.FontFamily == "" || st.FontSize == nil {
		return
	}
	key := fmt.Sprintf("%s|%s|%g|%s|%s", st.FontFamily, numKey(st.FontWeight), *st.FontSize, lineKey(st.LineHeight), numKey(st.LetterSpacing))
	t := d.typography[key]
	if t == nil {
		t = &TypographyToken{
			ID: "type_" + shortHash(key), FontFamily: st.FontFamily, FontWeight: st.FontWeight, FontSize: *st.FontSize,
			LineHeight: st.LineHeight, LetterSpacing: st.LetterSpacing,
		}
		d.typography[key] = t
	}
	t.Count++
}

func numKey(v *float64) string {
	if v == nil {
		return "-"
	}
	return strconv.FormatFloat(*v, 'g', -1, 64)
}

func lineKey(l *LineHeight) string {
	if l == nil {
		return "-"
	}
	return l.Mode + numKey(l.Value)
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:10]
}

// apply writes the collected indexes back, dropping registry entries nothing uses.
func (d *deriver) apply() {
	ir := d.ir

	components := ir.Components[:0:0]
	usedSets := map[string]bool{}
	for _, c := range ir.Components {
		use := d.comps[c.ID]
		if use == nil {
			continue
		}
		c.InstanceCount, c.ScreenIDs = use.instances, sortedKeys(use.screens)
		c.DefinitionScreenID, c.DefinitionNodeID = use.defScreen, use.defNode
		components = append(components, c)
		if c.SetID != "" {
			usedSets[c.SetID] = true
		}
	}
	sort.SliceStable(components, func(i, j int) bool {
		return components[i].Name+"\x00"+components[i].SourceID < components[j].Name+"\x00"+components[j].SourceID
	})
	ir.Components = nonNilComponents(components)

	sets := []ComponentSet{}
	for _, s := range ir.ComponentSets {
		if usedSets[s.ID] {
			sets = append(sets, s)
		}
	}
	sort.SliceStable(sets, func(i, j int) bool { return sets[i].Name+sets[i].SourceID < sets[j].Name+sets[j].SourceID })
	ir.ComponentSets = sets

	styles := []Style{}
	for _, s := range ir.Styles {
		if d.styles[s.ID] {
			styles = append(styles, s)
		}
	}
	sort.SliceStable(styles, func(i, j int) bool {
		return styles[i].Type+"\x00"+styles[i].Name+"\x00"+styles[i].SourceID < styles[j].Type+"\x00"+styles[j].Name+"\x00"+styles[j].SourceID
	})
	ir.Styles = styles

	assets := []Asset{}
	for _, a := range ir.Assets {
		if screens := d.assets[a.ID]; screens != nil {
			a.ScreenIDs = sortedKeys(screens)
			assets = append(assets, a)
		}
	}
	sort.SliceStable(assets, func(i, j int) bool { return assets[i].Path < assets[j].Path })
	ir.Assets = assets

	d.sections()
	d.warnings()
	ir.Tokens = d.tokens()
	ir.Stats = Stats{Screens: len(ir.Screens), Nodes: d.nodes, Components: len(ir.Components), Assets: len(ir.Assets), Warnings: len(ir.Warnings)}
}

func nonNilComponents(c []Component) []Component {
	if c == nil {
		return []Component{}
	}
	return c
}

func (d *deriver) sections() {
	present := map[string]bool{}
	for _, s := range d.ir.Screens {
		present[s.ID] = true
	}
	out := []Section{}
	for _, sec := range d.ir.Sections {
		ids := []string{}
		for _, id := range sec.ScreenIDs {
			if present[id] {
				ids = append(ids, id)
			}
		}
		if len(ids) > 0 {
			sec.ScreenIDs = ids
			out = append(out, sec)
		}
	}
	d.ir.Sections = out
}

func (d *deriver) warnings() {
	present := map[string]bool{}
	for _, s := range d.ir.Screens {
		present[s.ID] = true
	}
	seen := map[Warning]bool{}
	out := []Warning{}
	for _, w := range d.ir.Warnings {
		if (w.ScreenID != "" && !present[w.ScreenID]) || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return a.Code+"\x00"+a.ScreenID+"\x00"+a.SourceNodeID+"\x00"+a.Message < b.Code+"\x00"+b.ScreenID+"\x00"+b.SourceNodeID+"\x00"+b.Message
	})
	if len(out) > maxWarnings {
		out = out[:maxWarnings]
	}
	d.ir.Warnings = out
}

func (d *deriver) tokens() Tokens {
	t := Tokens{Colors: []ColorToken{}, Typography: []TypographyToken{}, Spacing: []NumberToken{}, Radii: []NumberToken{}, Shadows: []ShadowToken{}}
	for c, n := range d.colors {
		t.Colors = append(t.Colors, ColorToken{ID: fmt.Sprintf("color_%02x%02x%02x_%s", c.R, c.G, c.B, alphaKey(c.A)), Value: c, Count: n})
	}
	sort.SliceStable(t.Colors, func(i, j int) bool {
		if t.Colors[i].Count != t.Colors[j].Count {
			return t.Colors[i].Count > t.Colors[j].Count
		}
		return t.Colors[i].ID < t.Colors[j].ID
	})
	for _, tok := range d.typography {
		tok := *tok
		t.Typography = append(t.Typography, tok)
	}
	sort.SliceStable(t.Typography, func(i, j int) bool {
		if t.Typography[i].Count != t.Typography[j].Count {
			return t.Typography[i].Count > t.Typography[j].Count
		}
		return t.Typography[i].ID < t.Typography[j].ID
	})
	t.Spacing = numberTokens("space", d.spacing)
	t.Radii = numberTokens("radius", d.radii)
	for _, s := range d.shadows {
		t.Shadows = append(t.Shadows, *s)
	}
	sort.SliceStable(t.Shadows, func(i, j int) bool {
		if t.Shadows[i].Count != t.Shadows[j].Count {
			return t.Shadows[i].Count > t.Shadows[j].Count
		}
		return t.Shadows[i].ID < t.Shadows[j].ID
	})
	t.Colors, t.Typography = capTokens(t.Colors), capTokens(t.Typography)
	t.Shadows = capTokens(t.Shadows)
	return t
}

func capTokens[T any](in []T) []T {
	if len(in) > maxTokens {
		return in[:maxTokens]
	}
	return in
}

func alphaKey(a float64) string { return strconv.FormatFloat(a, 'g', -1, 64) }

func numberTokens(prefix string, counts map[float64]int) []NumberToken {
	out := []NumberToken{}
	for v, n := range counts {
		out = append(out, NumberToken{ID: prefix + "_" + strconv.FormatFloat(v, 'g', -1, 64), Value: v, Count: n})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Value < out[j].Value
	})
	return capTokens(out)
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
