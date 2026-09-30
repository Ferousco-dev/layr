package designapi

import (
	"context"
	"fmt"
	"sort"

	"github.com/ferousco-dev/layr/server/internal/designir"
)

const (
	maxColors  = 200
	maxFonts   = 50
	maxStyles  = 200
	maxNumbers = 64
)

// Tokens returns the colours, fonts, spacing and radii of the project's current design.
func (s *Service) Tokens(ctx context.Context, userID, projectID string) (*DesignTokens, error) {
	_, ix, err := s.ready(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	t := ix.ir.Tokens
	out := &DesignTokens{Colors: []ColorSwatch{}, Fonts: []FontFamily{}, Typography: []TextStyle{}, Spacing: []float64{}, Radii: []float64{}, Shadows: []ShadowStyle{}}

	for _, c := range t.Colors {
		out.Colors = append(out.Colors, ColorSwatch{Hex: fmt.Sprintf("#%02X%02X%02X", c.Value.R, c.Value.G, c.Value.B), Alpha: c.Value.A, Count: c.Count})
	}
	sort.SliceStable(out.Colors, func(i, j int) bool {
		if out.Colors[i].Count != out.Colors[j].Count {
			return out.Colors[i].Count > out.Colors[j].Count
		}
		return out.Colors[i].Hex < out.Colors[j].Hex
	})
	if len(out.Colors) > maxColors {
		out.Colors = out.Colors[:maxColors]
	}

	families := map[string]*FontFamily{}
	weights, sizes := map[string]map[int]bool{}, map[string]map[int]bool{}
	for _, ty := range t.Typography {
		if ty.FontFamily == "" {
			continue
		}
		f := families[ty.FontFamily]
		if f == nil {
			f = &FontFamily{Family: ty.FontFamily}
			families[ty.FontFamily], weights[ty.FontFamily], sizes[ty.FontFamily] = f, map[int]bool{}, map[int]bool{}
		}
		f.Count += ty.Count
		if ty.FontWeight != nil {
			weights[ty.FontFamily][int(*ty.FontWeight)] = true
		}
		sizes[ty.FontFamily][int(ty.FontSize+0.5)] = true
	}
	for name, f := range families {
		f.Weights, f.Sizes = sortedKeys(weights[name]), sortedKeys(sizes[name])
		out.Fonts = append(out.Fonts, *f)
	}
	sort.SliceStable(out.Fonts, func(i, j int) bool {
		if out.Fonts[i].Count != out.Fonts[j].Count {
			return out.Fonts[i].Count > out.Fonts[j].Count
		}
		return out.Fonts[i].Family < out.Fonts[j].Family
	})
	if len(out.Fonts) > maxFonts {
		out.Fonts = out.Fonts[:maxFonts]
	}

	styles := make([]TextStyle, 0, len(t.Typography))
	for _, ty := range t.Typography {
		if ty.FontFamily == "" {
			continue
		}
		style := TextStyle{Family: ty.FontFamily, Weight: ty.FontWeight, Size: ty.FontSize, LetterSpacing: ty.LetterSpacing, Count: ty.Count}
		if ty.LineHeight != nil {
			style.LineHeight = lineHeightText(ty.LineHeight)
		}
		styles = append(styles, style)
	}
	sort.SliceStable(styles, func(i, j int) bool {
		if styles[i].Count != styles[j].Count {
			return styles[i].Count > styles[j].Count
		}
		if styles[i].Family != styles[j].Family {
			return styles[i].Family < styles[j].Family
		}
		return styles[i].Size < styles[j].Size
	})
	if len(styles) > maxStyles {
		styles = styles[:maxStyles]
	}
	out.Typography = styles

	for _, sh := range t.Shadows {
		style := ShadowStyle{Type: sh.Type, X: sh.OffsetX, Y: sh.OffsetY, Blur: sh.Radius, Spread: sh.Spread, Alpha: 1, Count: sh.Count}
		if sh.Color != nil {
			style.Hex, style.Alpha = fmt.Sprintf("#%02X%02X%02X", sh.Color.R, sh.Color.G, sh.Color.B), sh.Color.A
		}
		out.Shadows = append(out.Shadows, style)
	}
	sort.SliceStable(out.Shadows, func(i, j int) bool { return out.Shadows[i].Count > out.Shadows[j].Count })
	if len(out.Shadows) > maxStyles {
		out.Shadows = out.Shadows[:maxStyles]
	}

	out.Spacing = numberValues(t.Spacing)
	out.Radii = numberValues(t.Radii)
	return out, nil
}

func sortedKeys(set map[int]bool) []int {
	out := make([]int, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// numberValues lists the most used values first (ties by size), keeping each once.
func numberValues(tokens []designir.NumberToken) []float64 {
	sorted := append([]designir.NumberToken(nil), tokens...)
	sort.SliceStable(sorted, func(i, j int) bool {
		if sorted[i].Count != sorted[j].Count {
			return sorted[i].Count > sorted[j].Count
		}
		return sorted[i].Value < sorted[j].Value
	})
	out := make([]float64, 0, min(len(sorted), maxNumbers))
	for _, n := range sorted {
		if len(out) == maxNumbers {
			break
		}
		out = append(out, n.Value)
	}
	return out
}

// lineHeightText writes a line height the way designers read it: "auto", "24px" or "120%".
func lineHeightText(l *designir.LineHeight) string {
	if l.Value == nil {
		return l.Mode
	}
	switch l.Mode {
	case "percent":
		return fmt.Sprintf("%g%%", *l.Value)
	case "px", "fixed":
		return fmt.Sprintf("%gpx", *l.Value)
	}
	return fmt.Sprintf("%g", *l.Value)
}
