package workspace

import (
	"path/filepath"
	"strings"
	"testing"
)

func FuzzHostileNamesNeverLeaveTheWorkspace(f *testing.F) {
	for _, s := range []string{"../x", "..", "/etc/passwd", `..\x`, "a/../../b", "%2e%2e/x", "x\x00y", "ﾠ", strings.Repeat("a", 300), "raw", "assets"} {
		f.Add(s, s)
	}
	root := f.TempDir()
	m, err := NewManager(root, 1<<20, 1<<22)
	if err != nil {
		f.Fatal(err)
	}
	dir, err := m.Create("0f8c0a1e-6d2b-4f0e-9a51-3c7b1d2e4f60")
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, sub, name string) {
		p, err := dir.filePath(sub, name)
		if err != nil {
			return
		}
		rel, rerr := filepath.Rel(dir.Path(), p)
		if rerr != nil || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) {
			t.Fatalf("filePath(%q, %q) = %q escapes %q", sub, name, p, dir.Path())
		}
		if _, err := m.dirPath(sub); err == nil && sub != "" && !idPattern.MatchString(sub) {
			t.Fatalf("dirPath accepted %q", sub)
		}
	})
}
