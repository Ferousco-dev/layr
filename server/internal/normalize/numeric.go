package normalize

import (
	"math"
	"strconv"
	"unicode/utf16"
	"unicode/utf8"
)

const (
	// snapEpsilon is how close to a whole number a value must be to become that number.
	snapEpsilon = 0.001
	// decimals bounds the precision kept for fractional values (Figma noise is far below this).
	decimals = 10000.0
)

// snap removes floating-point noise: 63.999996 becomes 64, while 12.5 and 0.3333 stay as they are.
func snap(v float64) float64 {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return v
	}
	if r := math.Round(v); math.Abs(v-r) < snapEpsilon {
		return r + 0 // avoids negative zero
	}
	return math.Round(v*decimals) / decimals
}

func snapPtr(v float64) *float64 {
	s := snap(v)
	return &s
}

// channel converts a Figma 0..1 color channel to 0..255, rounding to nearest.
func channel(v float64) int {
	c := int(math.Round(v * 255))
	return min(max(c, 0), 255)
}

func degrees(radians float64) float64 { return snap(radians * 180 / math.Pi) }

func formatValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case bool:
		return strconv.FormatBool(t)
	case float64:
		return strconv.FormatFloat(t, 'g', -1, 64)
	case nil:
		return ""
	}
	return ""
}

// utf16Offsets returns the UTF-16 index of every code point in s, plus the total length last.
func utf16Offsets(s string) []int {
	offsets := make([]int, 0, len(s)+1)
	pos := 0
	for _, r := range s {
		offsets = append(offsets, pos)
		pos += len(utf16.Encode([]rune{r}))
	}
	offsets = append(offsets, pos)
	return offsets
}

func runeCount(s string) int { return utf8.RuneCountInString(s) }
