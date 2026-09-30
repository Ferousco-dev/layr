package genplan

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
)

var (
	safeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,100}$`)
	// idPattern is what a caller-supplied screen or flow ID may look like.
	idPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
)

// safeID keeps plain IDs and replaces anything else by a hash, so IDs are never usable as paths.
func safeID(s string) string {
	if safeIDPattern.MatchString(s) {
		return s
	}
	sum := sha256.Sum256([]byte(s))
	return "h" + hex.EncodeToString(sum[:])[:24]
}
