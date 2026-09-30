package imports

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ferousco-dev/layr/server/internal/assets"
	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

const (
	ownerA   = "aaaaaaaa-0000-4000-8000-000000000001"
	ownerB   = "bbbbbbbb-0000-4000-8000-000000000002"
	projectA = "aaaaaaaa-1111-4111-8111-000000000001"
	projectB = "bbbbbbbb-1111-4111-8111-000000000002"
	fileKey  = "AbCdEfGhIjKlMnOpQrStUv"
	fileURL  = "https://www.figma.com/design/" + fileKey + "/Landing"
)

// memRepo mirrors the SQL repository's ownership and status guards.
type memRepo struct {
	mu      sync.Mutex
	owners  map[string]string
	rows    []*Import
	seq     int
	createN atomic.Int32
}

func newRepo() *memRepo {
	return &memRepo{owners: map[string]string{projectA: ownerA, projectB: ownerB}}
}

func (m *memRepo) find(id string) *Import {
	for _, r := range m.rows {
		if r.ID == id {
			return r
		}
	}
	return nil
}

func (m *memRepo) owned(owner, project, id string) *Import {
	if m.owners[project] != owner {
		return nil
	}
	if r := m.find(id); r != nil && r.ProjectID == project {
		return r
	}
	return nil
}

func (m *memRepo) Create(_ context.Context, owner, project, key, node string, now, stale time.Time) (Import, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.owners[project] != owner {
		return Import{}, ErrProjectNotFound
	}
	for _, r := range m.rows {
		if r.ProjectID != project {
			continue
		}
		if r.Status == StatusAwaitingSelection || ((r.Status == StatusPending || r.Status == StatusProcessing) && r.UpdatedAt.Before(stale)) {
			r.Status, r.ErrorCode, r.UpdatedAt = StatusFailed, CodeSuperseded, now
		}
	}
	for _, r := range m.rows {
		if r.ProjectID == project && (r.Status == StatusPending || r.Status == StatusProcessing) {
			return Import{}, ErrConflict
		}
	}
	m.seq++
	m.createN.Add(1)
	imp := &Import{ID: fmt.Sprintf("cccccccc-0000-4000-8000-%012d", m.seq), ProjectID: project, FileKey: key, NodeID: node, Status: StatusPending, CreatedAt: now, UpdatedAt: now}
	m.rows = append(m.rows, imp)
	return *imp, nil
}

func (m *memRepo) Get(_ context.Context, owner, project, id string) (Import, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if r := m.owned(owner, project, id); r != nil {
		return *r, nil
	}
	return Import{}, ErrNotFound
}

func (m *memRepo) Latest(_ context.Context, owner, project string) (Import, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.owners[project] == owner {
		for i := len(m.rows) - 1; i >= 0; i-- {
			if m.rows[i].ProjectID == project {
				return *m.rows[i], nil
			}
		}
	}
	return Import{}, ErrNotFound
}

func (m *memRepo) LatestCompleted(_ context.Context, owner, project string) (Import, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.owners[project] == owner {
		for i := len(m.rows) - 1; i >= 0; i-- {
			if m.rows[i].ProjectID == project && m.rows[i].Status == StatusCompleted {
				return *m.rows[i], nil
			}
		}
	}
	return Import{}, ErrNotFound
}

func (m *memRepo) BeginSelection(_ context.Context, owner, project, id string, sel Selection, maxScreens int, now time.Time) (Import, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.owned(owner, project, id)
	switch {
	case r == nil:
		return Import{}, ErrNotFound
	case r.Status != StatusAwaitingSelection:
		return Import{}, ErrConflict
	}
	known := map[string]bool{}
	for _, f := range r.Candidates {
		known[f.ID] = true
	}
	want := map[string]bool{}
	for _, n := range sel.NodeIDs {
		if !known[n] || want[n] {
			return Import{}, ErrInvalidSelection
		}
		want[n] = true
	}
	var chosen []string
	for _, f := range r.Candidates {
		if sel.All || want[f.ID] {
			chosen = append(chosen, f.ID)
		}
	}
	if len(chosen) == 0 || len(chosen) > maxScreens {
		return Import{}, ErrInvalidSelection
	}
	r.Status, r.NodeID, r.NodeIDs, r.UpdatedAt = StatusProcessing, chosen[0], chosen, now
	return *r, nil
}

func (m *memRepo) transition(id string, from []string, apply func(*Import)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := m.find(id)
	if r == nil {
		return ErrConflict
	}
	for _, s := range from {
		if r.Status == s {
			apply(r)
			return nil
		}
	}
	return ErrConflict
}

func (m *memRepo) MarkProcessing(_ context.Context, id string, now time.Time) error {
	return m.transition(id, []string{StatusPending}, func(r *Import) { r.Status, r.UpdatedAt = StatusProcessing, now })
}

func (m *memRepo) AwaitSelection(_ context.Context, id, name, version string, frames []Frame, now time.Time) error {
	return m.transition(id, []string{StatusProcessing}, func(r *Import) {
		r.Status, r.FileName, r.Version, r.Candidates, r.UpdatedAt = StatusAwaitingSelection, name, version, frames, now
	})
}

func (m *memRepo) Complete(_ context.Context, id string, res Result, now time.Time) error {
	return m.transition(id, []string{StatusProcessing}, func(r *Import) {
		r.Status, r.FileName, r.Version, r.NodeID, r.NodeName = StatusCompleted, res.FileName, res.Version, res.NodeID, res.NodeName
		r.RenderFormat, r.RenderScale, r.UpdatedAt, r.CompletedAt, r.Candidates = res.RenderFormat, res.RenderScale, now, &now, nil
		r.AssetCount, r.WarningCount = res.AssetCount, res.WarningCount
		r.ScreenCount, r.DesignNodes, r.DesignWarnings, r.NodeIDs = res.ScreenCount, res.DesignNodes, res.DesignWarnings, res.NodeIDs
	})
}

func (m *memRepo) Fail(_ context.Context, id, code string, now time.Time) error {
	_ = m.transition(id, []string{StatusPending, StatusProcessing, StatusAwaitingSelection}, func(r *Import) {
		r.Status, r.ErrorCode, r.UpdatedAt = StatusFailed, code, now
	})
	return nil
}

func (m *memRepo) FailAbandoned(_ context.Context, before time.Time, code string, now time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, r := range m.rows {
		if (r.Status == StatusPending || r.Status == StatusProcessing) && r.UpdatedAt.Before(before) {
			r.Status, r.ErrorCode, r.UpdatedAt = StatusFailed, code, now
			n++
		}
	}
	return n, nil
}

func (m *memRepo) RunningIDs(context.Context) (map[string]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]bool{}
	for _, r := range m.rows {
		if r.Status == StatusPending || r.Status == StatusProcessing {
			out[r.ID] = true
		}
	}
	return out, nil
}

func (m *memRepo) snapshot(id string) Import {
	m.mu.Lock()
	defer m.mu.Unlock()
	return *m.find(id)
}

// fakeFigma serves canned answers and writes canned raw bodies to the snapshot writer.
type fakeFigma struct {
	file       *figma.File
	fileErr    error
	nodes      map[string]figma.Node
	nodesErr   error
	renderErr  error
	renderURL  string
	fills      map[string]string
	components map[string]figma.ComponentMeta
	renderMap  map[string]string
	block      chan struct{}
	panicNodes bool
	calls      atomic.Int32
	renders    atomic.Int32
	users      []string
	mu         sync.Mutex
}

func (f *fakeFigma) note(user string) {
	f.calls.Add(1)
	f.mu.Lock()
	f.users = append(f.users, user)
	f.mu.Unlock()
}

func (f *fakeFigma) wait(ctx context.Context) error {
	if f.block == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return &figma.Error{Kind: figma.KindCancelled, Operation: "test", Err: ctx.Err()}
	case <-f.block:
		return nil
	}
}

func (f *fakeFigma) GetFile(ctx context.Context, user, key string, opts figma.FileOptions) (*figma.File, error) {
	f.note(user)
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	if f.fileErr != nil {
		return nil, f.fileErr
	}
	if _, err := opts.Snapshot.Write([]byte(`{"name":"Landing","document":{"id":"0:0","type":"DOCUMENT"}}`)); err != nil {
		return nil, &figma.Error{Kind: figma.KindBadResponse, Err: err}
	}
	return f.file, nil
}

func (f *fakeFigma) GetFileNodes(ctx context.Context, user, key string, ids []string, opts figma.NodesOptions) (*figma.FileNodes, error) {
	f.note(user)
	if f.panicNodes {
		panic("boom SECRET_TOKEN")
	}
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	if f.nodesErr != nil {
		return nil, f.nodesErr
	}
	if len(ids) > 100 {
		return nil, &figma.Error{Kind: figma.KindBadRequest}
	}
	if _, err := opts.Snapshot.Write([]byte(`{"name":"Landing","version":"77","nodes":{"` + ids[0] + `":{"document":{}}}}`)); err != nil {
		return nil, &figma.Error{Kind: figma.KindBadResponse, Err: err}
	}
	out := &figma.FileNodes{FileInfo: figma.FileInfo{Name: "Landing", Version: "77"}, Nodes: map[string]figma.NodeEntry{}}
	for _, id := range ids {
		if n, ok := f.nodes[id]; ok {
			out.Nodes[id] = figma.NodeEntry{Document: n, Components: f.components}
		} else {
			out.Missing = append(out.Missing, id)
		}
	}
	return out, nil
}

func (f *fakeFigma) RenderNodes(ctx context.Context, user, key string, ids []string, opts figma.RenderOptions) (*figma.Renders, error) {
	f.note(user)
	f.renders.Add(1)
	if err := f.wait(ctx); err != nil {
		return nil, err
	}
	if f.renderErr != nil {
		return nil, f.renderErr
	}
	if len(ids) > 100 {
		return nil, &figma.Error{Kind: figma.KindBadRequest}
	}
	out := &figma.Renders{URLs: map[string]string{}}
	for _, id := range ids {
		switch {
		case f.renderMap[id] != "":
			out.URLs[id] = f.renderMap[id]
		case f.renderURL != "":
			out.URLs[id] = f.renderURL
		default:
			out.Failed = append(out.Failed, id)
		}
	}
	return out, nil
}

func frame(id, name, typ string) figma.Node { return figma.Node{ID: id, Name: name, Type: typ} }

func fileWith(frames ...figma.Node) *figma.File {
	return &figma.File{
		FileInfo: figma.FileInfo{Name: "Landing", Version: "77"},
		Document: figma.Node{ID: "0:0", Type: figma.NodeDocument, Children: []figma.Node{{ID: "0:1", Type: figma.NodeCanvas, Children: frames}}},
	}
}

// fakeAssets stands in for the asset pipeline and records what it was asked to do.
type fakeAssets struct {
	referenceErr error
	runErr       error
	summary      assets.Summary
	referenceURL map[string]string
	root         figma.Node
	input        assets.Input
	runs         atomic.Int32
}

func (f *fakeAssets) SaveReferences(_ context.Context, dir *workspace.Dir, _, _ string, urls map[string]string) ([]assets.Reference, error) {
	f.referenceURL = urls
	if f.referenceErr != nil {
		return nil, f.referenceErr
	}
	ids := make([]string, 0, len(urls))
	for id := range urls {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	var out []assets.Reference
	for _, id := range ids {
		name := strings.ReplaceAll(id, ":", "-") + ".png"
		if err := dir.WriteBytes(workspace.Reference, name, []byte("png")); err != nil {
			return nil, err
		}
		out = append(out, assets.Reference{NodeID: id, Path: "reference/" + name, MediaType: "image/png", SizeBytes: 3, SHA256: "abc"})
	}
	return out, nil
}

func (f *fakeAssets) Run(_ context.Context, in assets.Input) (assets.Summary, error) {
	f.runs.Add(1)
	f.input, f.root = in, in.Root
	if f.runErr != nil {
		return f.summary, f.runErr
	}
	m := &assets.Manifest{ImportID: in.ImportID, Source: assets.ManifestSource{FileKey: in.FileKey, NodeIDs: in.NodeIDs}, References: in.References}
	if err := m.Save(in.Dir); err != nil {
		return assets.Summary{}, err
	}
	return f.summary, nil
}

type rig struct {
	svc  *Service
	repo *memRepo
	api  *fakeFigma
	mgr  *workspace.Manager
	root string
	logs *strings.Builder
}

func newRig(t *testing.T, api *fakeFigma, mutate ...func(*Config)) *rig {
	t.Helper()
	root := t.TempDir()
	mgr, err := workspace.NewManager(root, 1<<20, 1<<22)
	if err != nil {
		t.Fatal(err)
	}
	repo := newRepo()
	cfg := Config{Timeout: 5 * time.Second, WorkspaceTTL: time.Hour, MaxRunning: 4}
	for _, m := range mutate {
		m(&cfg)
	}
	logs := &strings.Builder{}
	svc := NewService(repo, api, mgr, slog.New(slog.NewTextHandler(logs, nil)), cfg)
	t.Cleanup(func() { svc.Close(context.Background()) })
	return &rig{svc: svc, repo: repo, api: api, mgr: mgr, root: root, logs: logs}
}

// settle waits until the import leaves the pending and processing states.
func (r *rig) settle(t *testing.T, id string) Import {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		imp := r.repo.snapshot(id)
		if imp.Status != StatusPending && imp.Status != StatusProcessing {
			return imp
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("import %s did not settle: %+v", id, r.repo.snapshot(id))
	return Import{}
}

func (r *rig) workspaceExists(id string) bool {
	_, err := readDir(r.root, id)
	return err == nil
}

func readFile(t *testing.T, r *rig, id, sub, name string) map[string]any {
	t.Helper()
	dir, err := r.mgr.Create(id)
	if err != nil {
		t.Fatal(err)
	}
	b, err := dir.ReadFile(sub, name)
	if err != nil {
		t.Fatalf("%s/%s: %v", sub, name, err)
	}
	var v map[string]any
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%s/%s is not valid JSON: %v", sub, name, err)
	}
	return v
}

var _ = io.Discard

func (f *fakeFigma) GetImageFills(context.Context, string, string) (map[string]string, error) {
	f.note("")
	return f.fills, nil
}
