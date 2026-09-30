package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const idA = "11111111-1111-4111-8111-111111111111"
const idB = "22222222-2222-4222-8222-222222222222"

func newTestManager(t *testing.T, file, total int64) (*Manager, string) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "root")
	m, err := NewManager(root, file, total)
	if err != nil {
		t.Fatal(err)
	}
	return m, root
}

func TestCreateBuildsIDBasedPrivateLayout(t *testing.T) {
	m, root := newTestManager(t, 1<<20, 1<<22)

	d, err := m.Create(idA)
	if err != nil {
		t.Fatal(err)
	}

	for _, sub := range []string{"", Raw, Reference} {
		info, err := os.Stat(filepath.Join(root, "imports", idA, sub))
		if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%q: %v mode %v", sub, err, info)
		}
	}
	if !strings.HasPrefix(d.Path(), filepath.Join(root, "imports")) || strings.Contains(d.String(), root) {
		t.Fatalf("path %q, string %q", d.Path(), d.String())
	}
	if _, err := m.Create(idA); err != nil {
		t.Fatalf("reopen: %v", err)
	}
}

func TestIDsAndNamesCannotEscapeTheRoot(t *testing.T) {
	m, _ := newTestManager(t, 1<<20, 1<<22)
	for _, bad := range []string{"", "..", "../x", idA + "/..", "/etc", "a/b", "11111111-1111-4111-8111-11111111111", "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA", idA + "\x00"} {
		if _, err := m.Create(bad); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Create(%q) = %v", bad, err)
		}
		if err := m.Cleanup(bad); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Cleanup(%q) = %v", bad, err)
		}
	}

	d, _ := m.Create(idA)
	for _, name := range []string{"", "../x", "a/b", ".hidden", "Upper.json", "x y", strings.Repeat("a", 65)} {
		if _, err := d.NewSnapshot(Raw, name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("name %q accepted", name)
		}
	}
	if _, err := d.NewSnapshot("../etc", "x.json"); !errors.Is(err, ErrInvalidName) {
		t.Error("sub directory escape accepted")
	}
}

func TestSnapshotIsAtomic(t *testing.T) {
	m, _ := newTestManager(t, 1<<20, 1<<22)
	d, _ := m.Create(idA)

	s, _ := d.NewSnapshot(Raw, "file.json")
	_, _ = s.Write([]byte(`{"half":`))
	if _, err := d.ReadFile(Raw, "file.json"); !os.IsNotExist(err) {
		t.Fatalf("partial content visible under the final name: %v", err)
	}
	s.Abort()
	s.Abort()
	entries, _ := os.ReadDir(filepath.Join(d.Path(), Raw))
	if len(entries) != 0 {
		t.Fatalf("abort left %d files", len(entries))
	}

	s, _ = d.NewSnapshot(Raw, "file.json")
	_, _ = s.Write([]byte(`{"ok":true}`))
	if err := s.Commit(); err != nil {
		t.Fatal(err)
	}
	got, _ := d.ReadFile(Raw, "file.json")
	info, _ := os.Stat(filepath.Join(d.Path(), Raw, "file.json"))
	if string(got) != `{"ok":true}` || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("content %q mode %v", got, info.Mode())
	}
}

func TestSizeLimits(t *testing.T) {
	m, _ := newTestManager(t, 10, 15)
	d, _ := m.Create(idA)

	big, _ := d.NewSnapshot(Raw, "big.json")
	if _, err := big.Write([]byte("01234567890")); !errors.Is(err, ErrTooLarge) || !big.Exceeded() {
		t.Fatalf("per-file limit: %v", err)
	}
	big.Abort()

	a, _ := d.NewSnapshot(Raw, "a.json")
	_, _ = a.Write([]byte("0123456789"))
	_ = a.Commit()
	b, _ := d.NewSnapshot(Raw, "b.json")
	if _, err := b.Write([]byte("012345")); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("workspace limit: %v", err)
	}
	b.Abort()
}

func TestWriteJSON(t *testing.T) {
	m, _ := newTestManager(t, 1<<20, 1<<22)
	d, _ := m.Create(idA)

	if err := d.WriteJSON(Reference, "render.json", map[string]any{"format": "png"}); err != nil {
		t.Fatal(err)
	}
	got, _ := d.ReadFile(Reference, "render.json")
	if strings.TrimSpace(string(got)) != `{"format":"png"}` {
		t.Fatalf("got %q", got)
	}
}

func TestCleanupIsRepeatableAndStaysInsideTheRoot(t *testing.T) {
	m, root := newTestManager(t, 1<<20, 1<<22)
	_, _ = m.Create(idA)
	_, _ = m.Create(idB)
	outside := filepath.Join(filepath.Dir(root), "precious.txt")
	_ = os.WriteFile(outside, []byte("keep"), 0o600)

	if err := m.Cleanup(idA); err != nil {
		t.Fatal(err)
	}
	if err := m.Cleanup(idA); err != nil {
		t.Fatalf("second cleanup: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "imports", idA)); !os.IsNotExist(err) {
		t.Fatal("workspace A survived")
	}
	if _, err := os.Stat(filepath.Join(root, "imports", idB)); err != nil {
		t.Fatal("workspace B was removed")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("file outside the root was touched")
	}
}

func TestRemoveRefusesPathsOutsideImports(t *testing.T) {
	m, root := newTestManager(t, 1<<20, 1<<22)
	victim := filepath.Join(filepath.Dir(root), "victim")
	_ = os.MkdirAll(victim, 0o700)

	for _, path := range []string{root, filepath.Join(root, "imports"), filepath.Join(root, "imports", ".."), victim, filepath.Join(root, "imports", "..", "..", "victim"), "/"} {
		if err := m.remove(path); !errors.Is(err, ErrOutsideRoot) {
			t.Errorf("remove(%q) = %v", path, err)
		}
	}
	if _, err := os.Stat(victim); err != nil {
		t.Fatal("victim directory was deleted")
	}
}

func TestSymlinkedWorkspaceIsUnlinkedNotFollowed(t *testing.T) {
	m, root := newTestManager(t, 1<<20, 1<<22)
	target := filepath.Join(filepath.Dir(root), "target")
	_ = os.MkdirAll(target, 0o700)
	_ = os.WriteFile(filepath.Join(target, "data.txt"), []byte("keep"), 0o600)
	link := filepath.Join(root, "imports", idA)
	if err := os.Symlink(target, link); err != nil {
		t.Skip("symlinks unavailable")
	}

	if _, err := m.Create(idA); err == nil {
		t.Fatal("a symlinked workspace was accepted")
	}
	if err := m.Cleanup(idA); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(target, "data.txt")); err != nil {
		t.Fatal("cleanup followed the symlink and deleted its target")
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatal("symlink itself was not removed")
	}
}

func TestCleanupStale(t *testing.T) {
	m, root := newTestManager(t, 1<<20, 1<<22)
	_, _ = m.Create(idA)
	_, _ = m.Create(idB)
	_ = os.MkdirAll(filepath.Join(root, "imports", "not-an-id"), 0o700)
	old := time.Now().Add(-48 * time.Hour)
	_ = os.Chtimes(filepath.Join(root, "imports", idA), old, old)
	_ = os.Chtimes(filepath.Join(root, "imports", "not-an-id"), old, old)

	removed, err := m.CleanupStale(24*time.Hour, time.Now(), nil)

	if err != nil || removed != 1 {
		t.Fatalf("removed %d err %v", removed, err)
	}
	if _, err := os.Stat(filepath.Join(root, "imports", idB)); err != nil {
		t.Fatal("fresh workspace removed")
	}
	if _, err := os.Stat(filepath.Join(root, "imports", "not-an-id")); err != nil {
		t.Fatal("unrecognised directory removed")
	}

	_ = os.Chtimes(filepath.Join(root, "imports", idB), old, old)
	removed, _ = m.CleanupStale(24*time.Hour, time.Now(), func(id string) bool { return id == idB })
	if removed != 0 {
		t.Fatal("a running import's workspace was removed")
	}
}

func TestSymlinkedSubdirectoryIsRefused(t *testing.T) {
	m, root := newTestManager(t, 1<<20, 1<<22)
	d, _ := m.Create(idA)
	outside := filepath.Join(filepath.Dir(root), "elsewhere")
	_ = os.MkdirAll(outside, 0o700)
	assets := filepath.Join(d.Path(), Assets)
	_ = os.Remove(assets)
	if err := os.Symlink(outside, assets); err != nil {
		t.Skip("symlinks unavailable")
	}

	if _, err := m.Create(idA); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("reopening with a symlinked assets directory: %v", err)
	}
	entries, _ := os.ReadDir(outside)
	if len(entries) != 0 {
		t.Fatal("something was written outside the workspace")
	}
}

func TestRename(t *testing.T) {
	m, _ := newTestManager(t, 1<<20, 1<<22)
	d, _ := m.Create(idA)
	_ = d.WriteBytes(Assets, "aaaa.png", []byte("x"))

	if err := d.Rename(Assets, "aaaa.png", "logo-a1b2c3.png"); err != nil {
		t.Fatal(err)
	}
	if err := d.Rename(Assets, "logo-a1b2c3.png", "../escape.png"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("escape: %v", err)
	}
	if got, err := d.ReadFile(Assets, "logo-a1b2c3.png"); err != nil || string(got) != "x" {
		t.Fatalf("renamed content %q %v", got, err)
	}
}

func TestHelpersRefuseHostileNamesAndLinks(t *testing.T) {
	m, _ := newTestManager(t, 1<<20, 1<<22)
	d, err := m.Create(idA)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.WriteBytes(Assets, "logo-abc123.svg", []byte("<svg/>")); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"../x", "..", "/etc/passwd", `a\b`, "A.svg", "", ".hidden"} {
		if _, err := d.Open(Assets, name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Open(%q) = %v", name, err)
		}
		if err := d.Remove(Assets, name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Remove(%q) = %v", name, err)
		}
	}
	if _, err := d.Names("../raw"); !errors.Is(err, ErrInvalidName) {
		t.Errorf("Names with a hostile directory = %v", err)
	}

	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(d.Path(), Assets, "link-abc123.svg")); err != nil {
		t.Fatal(err)
	}
	if f, err := d.Open(Assets, "link-abc123.svg"); err == nil {
		f.Close()
		t.Fatal("Open followed a symlink out of the workspace")
	}
	names, err := d.Names(Assets)
	if err != nil || len(names) != 1 || names[0] != "logo-abc123.svg" {
		t.Fatalf("Names = %v, %v (links and temporary files must be skipped)", names, err)
	}
	if err := d.Remove(Assets, "link-abc123.svg"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatal("removing a link must not touch its target")
	}
	if err := d.Remove(Assets, "missing-abc123.svg"); err != nil {
		t.Fatalf("a missing file is not an error: %v", err)
	}
}

func TestCommitAsValidatesTheFinalNameAndReadBackIsBounded(t *testing.T) {
	m, _ := newTestManager(t, 1<<20, 1<<22)
	d, _ := m.Create(idA)

	snap, err := d.NewSnapshot(Assets, "tmp-name-111111.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snap.Write([]byte("0123456789")); err != nil {
		t.Fatal(err)
	}
	got, err := snap.ReadBack(4)
	if err != nil || string(got) != "0123" {
		t.Fatalf("ReadBack = %q, %v", got, err)
	}
	if err := snap.CommitAs("../escape.png"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("hostile final name = %v", err)
	}
	if names, _ := d.Names(Assets); len(names) != 0 {
		t.Fatalf("a rejected commit left files behind: %v", names)
	}

	snap, _ = d.NewSnapshot(Assets, "tmp-name-111111.png")
	_, _ = snap.Write([]byte("abc"))
	if err := snap.CommitAs("final-222222.png"); err != nil {
		t.Fatal(err)
	}
	if data, err := d.ReadFile(Assets, "final-222222.png"); err != nil || string(data) != "abc" {
		t.Fatalf("committed content = %q, %v", data, err)
	}
}
