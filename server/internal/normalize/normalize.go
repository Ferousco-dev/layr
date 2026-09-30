// Package normalize turns a Figma snapshot and an Asset Manifest into Design IR, offline and deterministically.
package normalize

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ferousco-dev/layr/server/internal/assets"
	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
)

const provider = "figma"

// ScreenInput is one imported root and where it lives in the file.
type ScreenInput struct {
	Root figma.Node
	Page string
}

// Input is everything the normalizer needs; it never reaches for anything else.
type Input struct {
	FileKey       string
	FileName      string
	Version       string
	ImportID      string
	Roots         []ScreenInput
	Components    map[string]figma.ComponentMeta
	ComponentSets map[string]figma.ComponentSetMeta
	Styles        map[string]figma.StyleMeta
	Manifest      *assets.Manifest
}

// Spec is one screen after sections have been opened up.
type Spec struct {
	Root    figma.Node
	Page    string
	Section *SectionSpec
}

type SectionSpec struct {
	NodeID string
	Name   string
}

// Screens lists the screens of the roots: a section contributes the frames inside it, other roots are screens.
func Screens(roots []ScreenInput) []Spec {
	var out []Spec
	for _, r := range roots {
		if r.Root.Type == figma.NodeSection {
			inside := 0
			for _, c := range r.Root.Children {
				if isScreenType(c.Type) && c.IsVisible() {
					out = append(out, Spec{Root: c, Page: r.Page, Section: &SectionSpec{NodeID: r.Root.ID, Name: r.Root.Name}})
					inside++
				}
			}
			if inside > 0 {
				continue
			}
		}
		out = append(out, Spec{Root: r.Root, Page: r.Page})
	}
	return out
}

func isScreenType(t string) bool { return t == figma.NodeFrame || t == figma.NodeComponent }

type normalizer struct {
	in         Input
	lim        designir.Limits
	screenID   string
	nodes      int
	warnings   []designir.Warning
	byImageRef map[string]string
	exports    map[string]string
	assets     []designir.Asset
	components map[string]*designir.Component
	sets       map[string]*designir.ComponentSet
	styles     map[string]*designir.Style
	err        error
}

// Normalize builds the Design IR for all screens of the input and validates it.
func Normalize(in Input, lim designir.Limits) (*designir.DesignIR, error) {
	if len(in.Roots) == 0 {
		return nil, &designir.Error{Code: designir.CodeRootMissing, Problems: []string{"no screens were provided"}}
	}
	if in.FileKey == "" {
		return nil, &designir.Error{Code: designir.CodeInvalidInput, Problems: []string{"the file key is missing"}}
	}
	specs := Screens(in.Roots)
	if len(specs) > lim.MaxScreens {
		return nil, &designir.Error{Code: designir.CodeLimit, Problems: []string{fmt.Sprintf("%d screens exceed the limit of %d", len(specs), lim.MaxScreens)}}
	}

	n := &normalizer{
		in: in, lim: lim, byImageRef: map[string]string{}, exports: map[string]string{},
		components: map[string]*designir.Component{}, sets: map[string]*designir.ComponentSet{}, styles: map[string]*designir.Style{},
	}
	n.indexManifest()

	ir := &designir.DesignIR{
		Source: designir.Source{Provider: provider, FileKey: in.FileKey, FileName: in.FileName, Version: in.Version, ImportID: in.ImportID, NodeIDs: []string{}},
	}
	sections := map[string]*designir.Section{}
	var order []*designir.Section
	for _, spec := range specs {
		screen, err := n.screen(spec)
		if err != nil {
			return nil, err
		}
		if spec.Section != nil {
			sec := sections[spec.Section.NodeID]
			if sec == nil {
				sec = &designir.Section{ID: "section_" + short(in.FileKey, spec.Section.NodeID), SourceNodeID: spec.Section.NodeID, Name: spec.Section.Name, ScreenIDs: []string{}}
				sections[spec.Section.NodeID] = sec
				order = append(order, sec)
			}
			screen.SectionID = sec.ID
			sec.ScreenIDs = append(sec.ScreenIDs, screen.ID)
		}
		ir.Screens = append(ir.Screens, screen)
		ir.Source.NodeIDs = append(ir.Source.NodeIDs, screen.SourceNodeID)
	}
	for _, sec := range order {
		ir.Sections = append(ir.Sections, *sec)
	}

	ir.Assets, ir.Components, ir.ComponentSets, ir.Styles = n.assets, n.registryComponents(), n.registrySets(), n.registryStyles()
	ir.Warnings = n.warnings
	ir.Sections = nonNil(ir.Sections)
	ir.Finalize()

	if err := designir.Validate(ir, lim); err != nil {
		return nil, err
	}
	return ir, nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// indexManifest resolves imageRef and exported-vector lookups once.
func (n *normalizer) indexManifest() {
	m := n.in.Manifest
	if m == nil {
		return
	}
	for _, a := range m.Assets {
		n.assets = append(n.assets, designir.Asset{
			ID: a.ID, Kind: a.Kind, Format: a.Format, MediaType: a.MediaType, Path: a.Path, SizeBytes: a.SizeBytes,
			SHA256: a.SHA256, Width: a.Width, Height: a.Height, Sanitized: a.Sanitized, ScreenIDs: []string{},
		})
		for _, s := range a.Sources {
			switch {
			case s.ImageRef != "":
				n.byImageRef[s.ImageRef] = a.ID
			case s.Role == "export":
				n.exports[s.NodeID] = a.ID
			}
		}
	}
	for _, w := range m.Warnings {
		if w.NodeID != "" || w.Code != "" {
			n.warn("ASSET_"+strings.ToUpper(w.Code), w.NodeID, w.Message)
		}
	}
}

func (n *normalizer) screen(spec Spec) (designir.Screen, error) {
	n.screenID = "screen_" + short(n.in.FileKey, spec.Root.ID)
	root, err := n.node(spec.Root, nil, 1)
	if err != nil {
		return designir.Screen{}, err
	}
	s := designir.Screen{
		ID: n.screenID, SourceNodeID: spec.Root.ID, Name: spec.Root.Name, Page: spec.Page,
		Width: root.Geometry.Width, Height: root.Geometry.Height, ComponentIDs: []string{}, AssetIDs: []string{}, Root: root,
	}
	if m := n.in.Manifest; m != nil {
		for _, r := range m.References {
			if r.NodeID == spec.Root.ID {
				ref := &designir.Reference{Path: r.Path, Width: r.Width, Height: r.Height, SHA256: r.SHA256}
				s.Reference = ref
			}
		}
	}
	return s, nil
}

// warn records a warning against the screen being processed; identical ones are merged later.
func (n *normalizer) warn(code, nodeID, message string) {
	n.warnings = append(n.warnings, designir.Warning{Code: code, SourceNodeID: nodeID, ScreenID: n.screenID, Message: message})
}

// short derives a stable identifier fragment from the file and the provider node.
func short(fileKey, id string) string {
	sum := sha256.Sum256([]byte(fileKey + "\x00" + id))
	return hex.EncodeToString(sum[:])[:16]
}

func (n *normalizer) nodeID(figmaID string) string { return "n_" + short(n.in.FileKey, figmaID) }
