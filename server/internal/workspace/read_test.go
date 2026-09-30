package workspace

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestExistingNeverCreatesAndReportsExpiry(t *testing.T) {
	m, root := newTestManager(t, 1<<20, 1<<22)

	if _, err := m.Existing(idA); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing workspace: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "imports", idA)); !os.IsNotExist(err) {
		t.Fatal("Existing created the workspace")
	}
	if _, err := m.Existing("../x"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("bad id: %v", err)
	}

	d, _ := m.Create(idA)
	_ = d.WriteBytes(Design, "design-ir.json", []byte("{}"))
	back, err := m.Existing(idA)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := back.ReadFile(Design, "design-ir.json"); err != nil || string(got) != "{}" {
		t.Fatalf("read back %q %v", got, err)
	}
	_ = m.Cleanup(idA)
	if _, err := m.Existing(idA); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("cleaned workspace: %v", err)
	}
}

func TestExistingRefusesSymlinkedDirectories(t *testing.T) {
	m, root := newTestManager(t, 1<<20, 1<<22)
	d, _ := m.Create(idA)
	outside := filepath.Join(filepath.Dir(root), "elsewhere")
	_ = os.MkdirAll(outside, 0o700)
	ref := filepath.Join(d.Path(), Reference)
	_ = os.Remove(ref)
	if err := os.Symlink(outside, ref); err != nil {
		t.Skip("symlinks unavailable")
	}

	if _, err := m.Existing(idA); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v", err)
	}
}

func TestOpenRegularRefusesLinksAndOddFiles(t *testing.T) {
	m, root := newTestManager(t, 1<<20, 1<<22)
	d, _ := m.Create(idA)
	_ = d.WriteBytes(Reference, "ok.png", []byte("data"))
	secret := filepath.Join(filepath.Dir(root), "secret.txt")
	_ = os.WriteFile(secret, []byte("top secret"), 0o600)
	if err := os.Symlink(secret, filepath.Join(d.Path(), Reference, "link.png")); err != nil {
		t.Skip("symlinks unavailable")
	}
	_ = os.Mkdir(filepath.Join(d.Path(), Reference, "dir.png"), 0o700)

	f, info, err := d.OpenRegular(Reference, "ok.png")
	if err != nil || info.Size() != 4 {
		t.Fatalf("regular file: %v", err)
	}
	if b, _ := io.ReadAll(f); string(b) != "data" {
		t.Fatalf("content %q", b)
	}
	_ = f.Close()

	for _, name := range []string{"link.png", "dir.png", "missing.png", "../ok.png", "a/b.png"} {
		if f, _, err := d.OpenRegular(Reference, name); err == nil {
			_ = f.Close()
			t.Errorf("%q was opened", name)
		}
	}
	if _, _, err := d.OpenRegular("../raw", "ok.png"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("subdir escape: %v", err)
	}
}
