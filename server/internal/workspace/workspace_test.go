package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestPrepareCreatesOwnerOnlyDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "layr", "jobs")

	if err := Prepare(root); err != nil {
		t.Fatal(err)
	}

	info, err := os.Stat(root)
	if err != nil || !info.IsDir() {
		t.Fatalf("root missing: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("mode = %v, want owner-only", info.Mode().Perm())
	}
}

func TestPrepareIsIdempotent(t *testing.T) {
	root := t.TempDir()

	if err := Prepare(root); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareRejectsFile(t *testing.T) {
	file := filepath.Join(t.TempDir(), "occupied")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := Prepare(file); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}
