package imports

import (
	"os"
	"path/filepath"
)

func readDir(root, id string) ([]os.DirEntry, error) {
	return os.ReadDir(filepath.Join(root, "imports", id))
}
