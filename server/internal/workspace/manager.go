package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Subdirectories a workspace may contain.
const (
	Raw       = "raw"
	Reference = "reference"
	Assets    = "assets"
	Design    = "design"

	importsDir = "imports"
)

var (
	ErrInvalidName  = errors.New("workspace name invalid")
	ErrTooLarge     = errors.New("workspace file too large")
	ErrOutsideRoot  = errors.New("workspace path outside root")
	idPattern       = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	filenamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	allowedSubdirs  = map[string]bool{Raw: true, Reference: true, Assets: true, Design: true}
)

// Manager owns every per-import directory under one root.
type Manager struct {
	root         string
	maxFileBytes int64
	maxTotal     int64
}

// NewManager prepares root and bounds each file and each workspace in bytes.
func NewManager(root string, maxFileBytes, maxWorkspaceBytes int64) (*Manager, error) {
	if err := Prepare(root); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, ErrUnavailable
	}
	m := &Manager{root: filepath.Clean(abs), maxFileBytes: maxFileBytes, maxTotal: maxWorkspaceBytes}
	if err := os.MkdirAll(m.importsPath(), 0o700); err != nil {
		return nil, ErrUnavailable
	}
	return m, nil
}

func (m *Manager) importsPath() string { return filepath.Join(m.root, importsDir) }

// Dir is one import's workspace.
type Dir struct {
	path string
	m    *Manager
	mu   sync.Mutex
	used int64
}

// Create makes (or reopens) the workspace for an import ID; only canonical UUIDs are accepted.
func (m *Manager) Create(importID string) (*Dir, error) {
	path, err := m.dirPath(importID)
	if err != nil {
		return nil, err
	}
	for _, sub := range []string{"", Raw, Reference, Assets, Design} {
		if err := os.MkdirAll(filepath.Join(path, sub), 0o700); err != nil {
			return nil, ErrUnavailable
		}
	}
	// Every directory must be a real one: a planted symlink could aim writes outside the root.
	for _, sub := range []string{"", Raw, Reference, Assets, Design} {
		if info, err := os.Lstat(filepath.Join(path, sub)); err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, ErrUnavailable
		}
	}
	return &Dir{path: path, m: m}, nil
}

// dirPath builds the path from a validated ID only, never from names Figma or users supply.
func (m *Manager) dirPath(importID string) (string, error) {
	if !idPattern.MatchString(importID) {
		return "", ErrInvalidName
	}
	path := filepath.Join(m.importsPath(), importID)
	if !m.within(path) {
		return "", ErrOutsideRoot
	}
	return path, nil
}

// within reports whether path is strictly inside the imports directory.
func (m *Manager) within(path string) bool {
	rel, err := filepath.Rel(m.importsPath(), filepath.Clean(path))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// Cleanup removes one import's workspace; it is safe to repeat and never leaves the imports directory.
func (m *Manager) Cleanup(importID string) error {
	path, err := m.dirPath(importID)
	if err != nil {
		return err
	}
	return m.remove(path)
}

func (m *Manager) remove(path string) error {
	if !m.within(path) {
		return ErrOutsideRoot
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	// A symlink is unlinked, never followed, so it cannot redirect the deletion.
	if info.Mode()&os.ModeSymlink != 0 {
		return os.Remove(path)
	}
	return os.RemoveAll(path)
}

// CleanupStale removes workspaces untouched for ttl unless keep says the import is still running.
func (m *Manager) CleanupStale(ttl time.Duration, now time.Time, keep func(importID string) bool) (int, error) {
	entries, err := os.ReadDir(m.importsPath())
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, entry := range entries {
		name := entry.Name()
		if !idPattern.MatchString(name) {
			continue
		}
		info, err := entry.Info()
		if err != nil || now.Sub(info.ModTime()) < ttl || (keep != nil && keep(name)) {
			continue
		}
		if err := m.remove(filepath.Join(m.importsPath(), name)); err == nil {
			removed++
		}
	}
	return removed, nil
}

// Path returns the workspace directory for diagnostics and later pipeline stages.
func (d *Dir) Path() string { return d.path }

func (d *Dir) filePath(sub, name string) (string, error) {
	if !allowedSubdirs[sub] || !filenamePattern.MatchString(name) {
		return "", ErrInvalidName
	}
	return filepath.Join(d.path, sub, name), nil
}

// Snapshot is a file being written atomically: bytes go to a temporary file that Commit renames.
type Snapshot struct {
	dir       *Dir
	file      *os.File
	final     string
	written   int64
	exceeded  bool
	committed bool
}

// NewSnapshot starts an atomic write of name inside sub.
func (d *Dir) NewSnapshot(sub, name string) (*Snapshot, error) {
	final, err := d.filePath(sub, name)
	if err != nil {
		return nil, err
	}
	f, err := os.CreateTemp(filepath.Dir(final), ".partial-*")
	if err != nil {
		return nil, ErrUnavailable
	}
	return &Snapshot{dir: d, file: f, final: final}, nil
}

// Write enforces the per-file and per-workspace byte limits.
func (s *Snapshot) Write(p []byte) (int, error) {
	s.dir.mu.Lock()
	over := s.written+int64(len(p)) > s.dir.m.maxFileBytes || s.dir.used+s.written+int64(len(p)) > s.dir.m.maxTotal
	s.dir.mu.Unlock()
	if over {
		s.exceeded = true
		return 0, ErrTooLarge
	}
	n, err := s.file.Write(p)
	s.written += int64(n)
	return n, err
}

// Exceeded tells whether a size limit stopped this write.
func (s *Snapshot) Exceeded() bool { return s.exceeded }

// Commit flushes, closes and renames into place; the final name only ever holds complete content.
func (s *Snapshot) Commit() error {
	if s.committed {
		return nil
	}
	if err := s.file.Sync(); err != nil {
		s.Abort()
		return err
	}
	if err := s.file.Close(); err != nil {
		_ = os.Remove(s.file.Name())
		return err
	}
	if err := os.Rename(s.file.Name(), s.final); err != nil {
		_ = os.Remove(s.file.Name())
		return err
	}
	s.committed = true
	s.dir.mu.Lock()
	s.dir.used += s.written
	s.dir.mu.Unlock()
	return nil
}

// CommitAs is Commit with a different final name, chosen after the content is known.
func (s *Snapshot) CommitAs(name string) error {
	final, err := s.dir.filePath(filepath.Base(filepath.Dir(s.final)), name)
	if err != nil {
		s.Abort()
		return err
	}
	s.final = final
	return s.Commit()
}

// ReadBack returns what has been written so far, at most limit bytes, so it can be validated before commit.
func (s *Snapshot) ReadBack(limit int64) ([]byte, error) {
	if err := s.file.Sync(); err != nil {
		return nil, err
	}
	f, err := os.Open(s.file.Name())
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, limit))
}

// Abort discards the partial file; it is safe after Commit or repeated.
func (s *Snapshot) Abort() {
	if s.committed {
		return
	}
	_ = s.file.Close()
	_ = os.Remove(s.file.Name())
}

// WriteJSON stores a small value atomically.
func (d *Dir) WriteJSON(sub, name string, v any) error {
	snap, err := d.NewSnapshot(sub, name)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(snap).Encode(v); err != nil {
		snap.Abort()
		return err
	}
	return snap.Commit()
}

// ReadFile returns a workspace file's content, used by later stages and tests.
func (d *Dir) ReadFile(sub, name string) ([]byte, error) {
	path, err := d.filePath(sub, name)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, d.m.maxFileBytes+1))
}

// String never reveals the filesystem path in logs or errors.
func (d *Dir) String() string { return fmt.Sprintf("workspace(%s)", filepath.Base(d.path)) }

// WriteBytes stores small content atomically under a validated name.
func (d *Dir) WriteBytes(sub, name string, data []byte) error {
	snap, err := d.NewSnapshot(sub, name)
	if err != nil {
		return err
	}
	if _, err := snap.Write(data); err != nil {
		snap.Abort()
		return err
	}
	return snap.Commit()
}

// Open returns a read handle for a stored file, used to verify checksums of existing assets; it never follows a link.
func (d *Dir) Open(sub, name string) (*os.File, error) {
	path, err := d.filePath(sub, name)
	if err != nil {
		return nil, err
	}
	return os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
}

// Remove deletes one stored file; a missing file is not an error.
func (d *Dir) Remove(sub, name string) error {
	path, err := d.filePath(sub, name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// Names lists the regular files in a subdirectory, skipping in-progress temporary files.
func (d *Dir) Names(sub string) ([]string, error) {
	if !allowedSubdirs[sub] {
		return nil, ErrInvalidName
	}
	entries, err := os.ReadDir(filepath.Join(d.path, sub))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.Type().IsRegular() && !strings.HasPrefix(e.Name(), ".") {
			out = append(out, e.Name())
		}
	}
	return out, nil
}

// Rename moves a stored file to another validated name within the same subdirectory.
func (d *Dir) Rename(sub, from, to string) error {
	src, err := d.filePath(sub, from)
	if err != nil {
		return err
	}
	dst, err := d.filePath(sub, to)
	if err != nil {
		return err
	}
	return os.Rename(src, dst)
}

// Existing opens a workspace for reading and never creates one, so an expired workspace stays missing.
func (m *Manager) Existing(importID string) (*Dir, error) {
	path, err := m.dirPath(importID)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return nil, ErrUnavailable
	}
	for _, sub := range []string{Raw, Reference, Assets, Design} {
		if info, err := os.Lstat(filepath.Join(path, sub)); err == nil && (info.Mode()&os.ModeSymlink != 0 || !info.IsDir()) {
			return nil, ErrUnavailable
		}
	}
	return &Dir{path: path, m: m}, nil
}

// OpenRegular opens a stored file for streaming and refuses symlinks and non-regular files.
func (d *Dir) OpenRegular(sub, name string) (*os.File, os.FileInfo, error) {
	path, err := d.filePath(sub, name)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		_ = f.Close()
		return nil, nil, ErrInvalidName
	}
	return f, info, nil
}
