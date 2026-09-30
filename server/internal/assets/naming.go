// Package assets turns Figma asset references into verified local files and a versioned manifest.
package assets

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const maxSlugLength = 40

// slug reduces a Figma layer name to lowercase ASCII words; anything else becomes a hyphen.
func slug(name, fallback string) string {
	var b strings.Builder
	dash := true
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case !dash:
			b.WriteByte('-')
			dash = true
		}
		if b.Len() >= maxSlugLength {
			break
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return fallback
	}
	return out
}

// fileName is slug-hash.ext: readable, safe, and stable for the same name and content.
func fileName(name, fallback, sha256Hex, ext string, hashChars int) string {
	return slug(name, fallback) + "-" + sha256Hex[:hashChars] + "." + ext
}

// assetID is derived from content, so the same bytes always get the same ID.
func assetID(sha256Hex string) string { return "asset_" + sha256Hex[:16] }

func sumHex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
