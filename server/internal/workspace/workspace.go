// Package workspace prepares the temporary directory root used by later imports.
package workspace

import (
	"errors"
	"os"
)

var ErrUnavailable = errors.New("workspace unavailable")

// Prepare creates the root owner-only since it will hold private design data.
func Prepare(root string) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return ErrUnavailable
	}

	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		return ErrUnavailable
	}
	return nil
}
