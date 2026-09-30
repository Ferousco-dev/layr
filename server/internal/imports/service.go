package imports

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ferousco-dev/layr/server/internal/assets"
	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/figmaurl"
	"github.com/ferousco-dev/layr/server/internal/normalize"
	"github.com/ferousco-dev/layr/server/internal/project"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

const (
	// staleAfter lets a crashed import stop blocking its project.
	staleAfter    = 10 * time.Minute
	sweepInterval = time.Hour
	failTimeout   = 5 * time.Second
	renderScale   = 1.0
)

type Config struct {
	// Timeout bounds one import's background work.
	Timeout time.Duration
	// WorkspaceTTL is how long an unused workspace survives before cleanup.
	WorkspaceTTL time.Duration
	// MaxRunning bounds imports processed at once.
	MaxRunning int
	// MaxScreens bounds the screens of one import.
	MaxScreens int
}

// DesignConfig turns on Design IR generation.
type DesignConfig struct {
	MaxNodes int
	MaxBytes int64
}

type Service struct {
	repo   Repository
	figma  FigmaAPI
	ws     Workspaces
	assets AssetPipeline
	design *DesignConfig
	log    *slog.Logger
	cfg    Config
	now    func() time.Time
	base   context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	slots  chan struct{}
	sweep  atomic.Int64

	owners   sync.Map
	coolMu   sync.Mutex
	cooldown map[string]time.Time
}

func NewService(repo Repository, api FigmaAPI, ws Workspaces, log *slog.Logger, cfg Config) *Service {
	if cfg.MaxRunning < 1 {
		cfg.MaxRunning = 4
	}
	if cfg.MaxScreens < 1 {
		cfg.MaxScreens = designir.DefaultLimits.MaxScreens
	}
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	base, cancel := context.WithCancel(context.Background())
	return &Service{
		repo: repo, figma: api, ws: ws, log: log, cfg: cfg,
		now:  func() time.Time { return time.Now().UTC() },
		base: base, cancel: cancel, slots: make(chan struct{}, cfg.MaxRunning),
	}
}

// SetDesign enables Design IR generation at the end of an import.
func (s *Service) SetDesign(c DesignConfig) { s.design = &c }

// SetAssets enables reference and asset downloading; without it an import stops after the render request.
func (s *Service) SetAssets(p AssetPipeline) { s.assets = p }

// Start records an import and processes it in the background; poll Get for the outcome.
func (s *Service) Start(ctx context.Context, userID, projectID, rawURL string) (Import, error) {
	projectID, err := project.NormalizeID(projectID)
	if err != nil {
		return Import{}, ErrInvalidID
	}
	parsed, err := figmaurl.Parse(rawURL)
	if err != nil {
		return Import{}, ErrInvalidURL
	}
	if err := s.guard(userID); err != nil {
		return Import{}, err
	}
	if !s.acquire() {
		return Import{}, ErrBusy
	}

	now := s.now()
	imp, err := s.repo.Create(ctx, userID, projectID, parsed.FileKey, parsed.NodeID, now, now.Add(-staleAfter))
	if err != nil {
		s.release()
		return Import{}, err
	}
	s.launch(imp, userID)
	s.maybeSweep()
	return imp, nil
}

// Refresh imports the project's Figma file again, from the file its latest finished import came from.
func (s *Service) Refresh(ctx context.Context, userID, projectID string) (Import, error) {
	projectID, err := project.NormalizeID(projectID)
	if err != nil {
		return Import{}, ErrInvalidID
	}
	last, err := s.repo.LatestCompleted(ctx, userID, projectID)
	if err != nil {
		return Import{}, err
	}
	return s.Start(ctx, userID, projectID, "https://www.figma.com/design/"+last.FileKey)
}

// Select continues an import that is waiting for the user to choose screens.
func (s *Service) Select(ctx context.Context, userID, projectID, importID string, sel Selection) (Import, error) {
	projectID, importID, err := ids(projectID, importID)
	if err != nil {
		return Import{}, err
	}
	if !sel.All && (len(sel.NodeIDs) == 0 || len(sel.NodeIDs) > s.cfg.MaxScreens) {
		return Import{}, ErrInvalidSelection
	}
	if err := s.guard(userID); err != nil {
		return Import{}, err
	}
	if !s.acquire() {
		return Import{}, ErrBusy
	}
	imp, err := s.repo.BeginSelection(ctx, userID, projectID, importID, sel, s.cfg.MaxScreens, s.now())
	if err != nil {
		s.release()
		return Import{}, err
	}
	s.launch(imp, userID)
	return imp, nil
}

func (s *Service) Get(ctx context.Context, userID, projectID, importID string) (Import, error) {
	projectID, importID, err := ids(projectID, importID)
	if err != nil {
		return Import{}, err
	}
	return s.repo.Get(ctx, userID, projectID, importID)
}

func (s *Service) Latest(ctx context.Context, userID, projectID string) (Import, error) {
	projectID, err := project.NormalizeID(projectID)
	if err != nil {
		return Import{}, ErrInvalidID
	}
	return s.repo.Latest(ctx, userID, projectID)
}

func ids(projectID, importID string) (string, string, error) {
	p, err := project.NormalizeID(projectID)
	if err != nil {
		return "", "", ErrInvalidID
	}
	i, err := project.NormalizeID(importID)
	if err != nil {
		return "", "", ErrInvalidID
	}
	return p, i, nil
}

func (s *Service) acquire() bool {
	select {
	case s.slots <- struct{}{}:
		return true
	default:
		return false
	}
}

func (s *Service) release() { <-s.slots }

func (s *Service) launch(imp Import, userID string) {
	s.wg.Add(1)
	s.owners.Store(imp.ID, userID)
	go func() {
		defer s.wg.Done()
		defer s.owners.Delete(imp.ID)
		defer s.release()
		defer func() {
			if r := recover(); r != nil {
				s.fail(imp, CodeInternal, fmt.Errorf("panic: %v", r))
			}
		}()
		ctx, cancel := context.WithTimeout(s.base, s.cfg.Timeout)
		defer cancel()
		s.execute(ctx, imp, userID)
	}()
}

// Close cancels running imports and waits for them, so dependencies can close safely afterwards.
func (s *Service) Close(ctx context.Context) {
	s.cancel()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Sweep removes workspaces older than the TTL unless their import is still running.
func (s *Service) Sweep(ctx context.Context) {
	now := s.now()
	if n, err := s.repo.FailAbandoned(ctx, now.Add(-s.abandonedAfter()), CodeInterrupted, now); err == nil && n > 0 {
		s.log.WarnContext(ctx, "import.abandoned_failed", slog.Int("count", n))
	}
	running, err := s.repo.RunningIDs(ctx)
	if err != nil {
		return
	}
	removed, _ := s.ws.CleanupStale(s.cfg.WorkspaceTTL, s.now(), func(id string) bool { return running[id] })
	if removed > 0 {
		s.log.InfoContext(ctx, "import.workspaces_swept", slog.Int("removed", removed))
	}
}

// abandonedAfter is longer than any live import can run, so only a dead process leaves an import this old.
func (s *Service) abandonedAfter() time.Duration {
	if limit := s.cfg.Timeout + time.Minute; limit > staleAfter {
		return limit
	}
	return staleAfter
}

func (s *Service) maybeSweep() {
	now := s.now().Unix()
	last := s.sweep.Load()
	if now-last < int64(sweepInterval.Seconds()) || !s.sweep.CompareAndSwap(last, now) {
		return
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.Sweep(s.base)
	}()
}

// stages records how long each phase took, for one summary log line.
type stages struct {
	start time.Time
	marks []any
}

func (st *stages) mark(name string, since time.Time) {
	st.marks = append(st.marks, slog.Int64(name, time.Since(since).Milliseconds()))
}

// fetched is the design data of the chosen roots, straight from Figma.
type fetched struct {
	info          figma.FileInfo
	roots         []normalize.ScreenInput
	components    map[string]figma.ComponentMeta
	componentSets map[string]figma.ComponentSetMeta
	styles        map[string]figma.StyleMeta
}

func (s *Service) execute(ctx context.Context, imp Import, userID string) {
	st := &stages{start: time.Now()}

	dir, err := s.ws.Create(imp.ID)
	if err != nil {
		s.fail(imp, CodeWorkspaceFailed, err)
		return
	}
	if imp.Status == StatusPending {
		if err := s.repo.MarkProcessing(ctx, imp.ID, s.now()); err != nil {
			s.log.WarnContext(ctx, "import.not_started", slog.String("import_id", imp.ID), slog.String("code", "IMPORT_CONFLICT"))
			return
		}
	}

	nodeIDs := imp.NodeIDs
	if len(nodeIDs) == 0 && imp.NodeID != "" {
		nodeIDs = []string{imp.NodeID}
	}
	if len(nodeIDs) == 0 {
		frames, info, code, err := s.discover(ctx, dir, imp, userID, st, "")
		if err != nil {
			s.failFor(ctx, imp, code, err)
			return
		}
		if len(frames) > 1 {
			if err := s.repo.AwaitSelection(ctx, imp.ID, info.Name, info.Version, frames, s.now()); err != nil {
				s.failFor(ctx, imp, CodeInternal, err)
				return
			}
			s.log.InfoContext(ctx, "import.awaiting_selection", s.fields(imp, userID, len(frames))...)
			return
		}
		nodeIDs = []string{frames[0].ID}
		imp.Candidates = frames
	}

	data, code, err := s.fetchTargets(ctx, dir, imp, userID, nodeIDs, st)
	if err != nil && containerLink(err) && len(imp.NodeIDs) == 0 && imp.NodeID != "" {
		// The link points at a page or another container: offer the frames around it instead of failing.
		frames, info, found, ferr := s.discover(ctx, dir, imp, userID, st, imp.NodeID)
		if ferr != nil {
			s.failFor(ctx, imp, found, ferr)
			return
		}
		if len(frames) > 1 {
			if err := s.repo.AwaitSelection(ctx, imp.ID, info.Name, info.Version, frames, s.now()); err != nil {
				s.failFor(ctx, imp, CodeInternal, err)
				return
			}
			s.log.InfoContext(ctx, "import.awaiting_selection", s.fields(imp, userID, len(frames))...)
			return
		}
		nodeIDs = []string{frames[0].ID}
		imp.Candidates = frames
		data, code, err = s.fetchTargets(ctx, dir, imp, userID, nodeIDs, st)
	}
	if err != nil {
		s.failFor(ctx, imp, code, err)
		return
	}
	specs := normalize.Screens(data.roots)
	if len(specs) > s.cfg.MaxScreens {
		s.failFor(ctx, imp, CodeTooManyScreens, fmt.Errorf("%d screens exceed the limit of %d", len(specs), s.cfg.MaxScreens))
		return
	}

	refs, code, err := s.referenceScreens(ctx, dir, imp, userID, specs, st)
	if err != nil {
		s.failFor(ctx, imp, code, err)
		return
	}
	summary, err := s.downloadAssets(ctx, dir, imp, userID, specs, refs, st)
	if err != nil {
		s.failFor(ctx, imp, s.codeFor(ctx, err), err)
		return
	}
	design, err := s.buildDesign(ctx, dir, imp, userID, data, st)
	if err != nil {
		s.failFor(ctx, imp, s.codeFor(ctx, err), err)
		return
	}

	result := Result{
		FileName: data.info.Name, Version: data.info.Version, NodeID: nodeIDs[0], NodeIDs: nodeIDs, NodeName: specs[0].Root.Name,
		RenderFormat: figma.FormatPNG, RenderScale: renderScale, AssetCount: summary.Assets, WarningCount: summary.Warnings,
		ScreenCount: len(specs), DesignNodes: design.Nodes, DesignWarnings: design.Warnings,
	}
	if err := s.repo.Complete(ctx, imp.ID, result, s.now()); err != nil {
		s.failFor(ctx, imp, CodeInternal, err)
		return
	}
	st.mark("total_ms", st.start)
	s.log.InfoContext(ctx, "import.completed", append(s.fields(imp, userID, 0),
		append(st.marks, slog.Int("screens", len(specs)), slog.Int("design_nodes", design.Nodes))...)...)
}

// discover reads the file's pages and top-level nodes to find importable frames.
// A focus node that is not itself importable (a page, a group) narrows the list to the frames around it.
func (s *Service) discover(ctx context.Context, dir *workspace.Dir, imp Import, userID string, st *stages, focus string) ([]Frame, figma.FileInfo, string, error) {
	began := time.Now()
	var file *figma.File
	err := s.snapshot(dir, "file.json", func(w io.Writer) error {
		var e error
		file, e = s.figma.GetFile(ctx, userID, imp.FileKey, figma.FileOptions{Depth: 2, Snapshot: w})
		return e
	})
	st.mark("figma_file_fetch_ms", began)
	if err != nil {
		return nil, figma.FileInfo{}, s.codeFor(ctx, err), err
	}

	frames := CandidatesAround(file.Document, focus)
	if len(frames) == 0 {
		return nil, file.FileInfo, CodeNoFrames, errors.New("no importable frames")
	}
	return frames, file.FileInfo, "", nil
}

// fetchTargets loads the full subtree of every chosen node in one request and checks each is importable.
func (s *Service) fetchTargets(ctx context.Context, dir *workspace.Dir, imp Import, userID string, nodeIDs []string, st *stages) (*fetched, string, error) {
	began := time.Now()
	res := &figma.FileNodes{Nodes: map[string]figma.NodeEntry{}}
	for i, part := range batches(nodeIDs) {
		name := "target-node.json"
		if i > 0 {
			name = fmt.Sprintf("target-node-%d.json", i)
		}
		var got *figma.FileNodes
		err := s.snapshot(dir, name, func(w io.Writer) error {
			var e error
			got, e = s.figma.GetFileNodes(ctx, userID, imp.FileKey, part, figma.NodesOptions{Snapshot: w})
			return e
		})
		if err != nil {
			st.mark("figma_node_fetch_ms", began)
			return nil, s.codeFor(ctx, err), err
		}
		res.FileInfo = got.FileInfo
		for k, v := range got.Nodes {
			res.Nodes[k] = v
		}
		res.Missing = append(res.Missing, got.Missing...)
		res.Malformed = append(res.Malformed, got.Malformed...)
	}
	st.mark("figma_node_fetch_ms", began)

	pages := map[string]string{}
	for _, f := range imp.Candidates {
		pages[f.ID] = f.Page
	}
	out := &fetched{
		info: res.FileInfo, components: map[string]figma.ComponentMeta{}, componentSets: map[string]figma.ComponentSetMeta{},
		styles: map[string]figma.StyleMeta{},
	}
	for _, id := range nodeIDs {
		entry, ok := res.Nodes[id]
		switch {
		case ok && !SupportedRoot[entry.Document.Type]:
			return nil, CodeUnsupportedNode, &unsupportedNode{Type: entry.Document.Type}
		case !ok && contains(res.Malformed, id):
			return nil, CodeImportFailed, errors.New("node entry unreadable")
		case !ok:
			return nil, CodeNodeNotFound, errors.New("node not found")
		}
		out.roots = append(out.roots, normalize.ScreenInput{Root: entry.Document, Page: pages[id]})
		for k, v := range entry.Components {
			out.components[k] = v
		}
		for k, v := range entry.ComponentSets {
			out.componentSets[k] = v
		}
		for k, v := range entry.Styles {
			out.styles[k] = v
		}
	}
	return out, "", nil
}

func contains(list []string, id string) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}

// referenceScreens renders every screen as a PNG, downloads them at once and keeps no temporary URL.
func (s *Service) referenceScreens(ctx context.Context, dir *workspace.Dir, imp Import, userID string, specs []normalize.Spec, st *stages) ([]assets.Reference, string, error) {
	ids := make([]string, 0, len(specs))
	for _, sp := range specs {
		ids = append(ids, sp.Root.ID)
	}
	began := time.Now()
	renders := &figma.Renders{URLs: map[string]string{}}
	for _, part := range batches(ids) {
		got, err := s.figma.RenderNodes(ctx, userID, imp.FileKey, part, figma.RenderOptions{Format: figma.FormatPNG, Scale: renderScale})
		if err != nil {
			st.mark("render_request_ms", began)
			return nil, s.codeFor(ctx, err), err
		}
		for k, v := range got.URLs {
			renders.URLs[k] = v
		}
		renders.Failed = append(renders.Failed, got.Failed...)
	}
	st.mark("render_request_ms", began)
	if len(renders.Failed) > 0 || len(renders.URLs) != len(ids) {
		return nil, CodeImportFailed, errors.New("figma could not render every screen")
	}

	var refs []assets.Reference
	if s.assets != nil {
		var err error
		began = time.Now()
		if refs, err = s.assets.SaveReferences(ctx, dir, userID, imp.FileKey, renders.URLs); err != nil {
			return nil, s.codeFor(ctx, err), err
		}
		st.mark("reference_download_ms", began)
	}

	entries := make([]map[string]any, 0, len(refs))
	for _, r := range refs {
		entries = append(entries, map[string]any{"node_id": r.NodeID, "path": r.Path, "sha256": r.SHA256})
	}
	meta := map[string]any{"format": figma.FormatPNG, "scale": renderScale, "requested_at": s.now(), "screens": ids, "references": entries}
	if err := dir.WriteJSON(workspace.Reference, "render.json", meta); err != nil {
		if errors.Is(err, workspace.ErrTooLarge) {
			return nil, CodeSnapshotTooLarge, err
		}
		return nil, CodeWorkspaceFailed, err
	}
	return refs, "", nil
}

// downloadAssets resolves and stores the real images and SVGs that the screens reference.
func (s *Service) downloadAssets(ctx context.Context, dir *workspace.Dir, imp Import, userID string, specs []normalize.Spec, refs []assets.Reference, st *stages) (assets.Summary, error) {
	if s.assets == nil {
		return assets.Summary{}, nil
	}
	ids := make([]string, 0, len(specs))
	group := figma.Node{Type: figma.NodeDocument}
	for _, sp := range specs {
		ids = append(ids, sp.Root.ID)
		group.Children = append(group.Children, sp.Root)
	}
	began := time.Now()
	summary, err := s.assets.Run(ctx, assets.Input{
		ImportID: imp.ID, UserID: userID, FileKey: imp.FileKey, NodeIDs: ids, Root: group, Dir: dir, References: refs,
	})
	st.mark("assets_ms", began)
	return summary, err
}

// designSummary is what building the Design IR reports back.
type designSummary struct {
	Nodes    int
	Warnings int
}

// buildDesign turns the snapshot and manifest into Design IR and stores it atomically in the workspace.
func (s *Service) buildDesign(ctx context.Context, dir *workspace.Dir, imp Import, userID string, data *fetched, st *stages) (designSummary, error) {
	if s.design == nil {
		return designSummary{}, nil
	}
	began := time.Now()
	in := normalize.Input{
		FileKey: imp.FileKey, FileName: data.info.Name, Version: data.info.Version, ImportID: imp.ID, Roots: data.roots,
		Components: data.components, ComponentSets: data.componentSets, Styles: data.styles,
	}
	if s.assets != nil {
		manifest, err := assets.LoadManifest(dir)
		if err != nil {
			return designSummary{}, &designir.Error{Code: designir.CodeInvalidInput, Problems: []string{"the asset manifest could not be read"}}
		}
		in.Manifest = manifest
	}

	limits := designir.Limits{MaxNodes: s.design.MaxNodes, MaxDepth: designir.DefaultLimits.MaxDepth, MaxScreens: s.cfg.MaxScreens}
	ir, err := normalize.Normalize(in, limits)
	if err != nil {
		return designSummary{}, err
	}
	encoded, err := designir.Marshal(ir, limits)
	if err != nil {
		return designSummary{}, err
	}
	if int64(len(encoded)) > s.design.MaxBytes {
		return designSummary{}, &designir.Error{Code: designir.CodeLimit, Problems: []string{"the serialized design is too large"}}
	}
	if err := dir.WriteBytes(workspace.Design, "design-ir.json", encoded); err != nil {
		if errors.Is(err, workspace.ErrTooLarge) {
			return designSummary{}, &designir.Error{Code: designir.CodeLimit, Problems: []string{"the serialized design is too large"}}
		}
		return designSummary{}, &workspaceError{err}
	}
	st.mark("design_ms", began)
	s.log.InfoContext(ctx, "import.design_built",
		slog.String("import_id", imp.ID), slog.String("user_id", userID), slog.Int("schema_version", designir.SchemaVersion),
		slog.Int("screens", ir.Stats.Screens), slog.Int("node_count", ir.Stats.Nodes), slog.Int("asset_count", ir.Stats.Assets),
		slog.Int("warning_count", ir.Stats.Warnings), slog.Int("bytes", len(encoded)))
	return designSummary{Nodes: ir.Stats.Nodes, Warnings: ir.Stats.Warnings}, nil
}

// snapshot runs call with an atomic snapshot writer, keeping the file only if the call succeeds.
func (s *Service) snapshot(dir *workspace.Dir, name string, call func(io.Writer) error) error {
	snap, err := dir.NewSnapshot(workspace.Raw, name)
	if err != nil {
		return &workspaceError{err}
	}
	if err := call(snap); err != nil {
		snap.Abort()
		if snap.Exceeded() {
			return &tooLargeError{err}
		}
		return err
	}
	if err := snap.Commit(); err != nil {
		return &workspaceError{err}
	}
	return nil
}

type workspaceError struct{ error }
type tooLargeError struct{ error }

// codeFor maps any failure to a stable stored code; it never includes provider detail.
func (s *Service) codeFor(ctx context.Context, err error) string {
	var we *workspaceError
	var tl *tooLargeError
	if code := assets.CodeOf(err); code != "" && ctx.Err() == nil {
		return code
	}
	switch {
	case designir.CodeOf(err) != "":
		return designir.CodeOf(err)
	case errors.As(err, &we):
		return CodeWorkspaceFailed
	case errors.As(err, &tl):
		return CodeSnapshotTooLarge
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return CodeTimeout
	case errors.Is(ctx.Err(), context.Canceled):
		return CodeCancelled
	}
	if kind := figma.KindOf(err); kind != "" {
		switch kind {
		case figma.KindTokenExpired:
			return string(figma.KindAuthRequired)
		case figma.KindBadRequest, figma.KindBadResponse:
			return CodeImportFailed
		}
		return string(kind)
	}
	return CodeImportFailed
}

func (s *Service) fields(imp Import, userID string, frames int) []any {
	fields := []any{
		slog.String("import_id", imp.ID), slog.String("project_id", imp.ProjectID), slog.String("user_id", userID),
		slog.String("figma_file_key", imp.FileKey), slog.String("figma_node_id", imp.NodeID),
	}
	if frames > 0 {
		fields = append(fields, slog.Int("candidate_frames", frames))
	}
	return fields
}

// failFor prefers a context-derived code when the whole import was cancelled or timed out.
func (s *Service) failFor(ctx context.Context, imp Import, code string, err error) {
	if ctx.Err() != nil {
		code = s.codeFor(ctx, err)
	}
	s.fail(imp, code, err)
}

// fail records the failure with a detached context, so cancellation cannot leave the row running.
func (s *Service) fail(imp Import, code string, cause error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(s.base), failTimeout)
	defer cancel()
	userID, _ := s.owners.Load(imp.ID)
	owner, _ := userID.(string)
	attrs := append(s.fields(imp, owner, 0), slog.String("code", code))
	var fe *figma.Error
	if errors.As(cause, &fe) && fe.Kind == figma.KindRateLimited {
		s.noteRateLimit(owner, fe.RetryAfter)
	}
	if errors.As(cause, &fe) && fe.RetryAfter > 0 {
		attrs = append(attrs, slog.Int64("retry_after_s", int64(fe.RetryAfter.Seconds())))
	}
	var ie *designir.Error
	if errors.As(cause, &ie) && len(ie.Problems) > 0 {
		attrs = append(attrs, slog.String("problems", ie.Error()))
	}
	s.log.WarnContext(ctx, "import.failed", attrs...)

	// Cleanup first: once the row says failed, nothing may still be using the workspace.
	_ = s.ws.Cleanup(imp.ID)
	_ = s.repo.Fail(ctx, imp.ID, code, s.now())
}

// nodeBatch is how many nodes Figma answers in one request.
const nodeBatch = 100

// batches splits IDs into requests Figma will accept.
func batches(ids []string) [][]string {
	var out [][]string
	for len(ids) > nodeBatch {
		out = append(out, ids[:nodeBatch])
		ids = ids[nodeBatch:]
	}
	return append(out, ids)
}

// unsupportedNode says a linked node is not something Layr can import as a screen.
type unsupportedNode struct{ Type string }

func (e *unsupportedNode) Error() string {
	return fmt.Sprintf("node type %q cannot be imported", e.Type)
}

// containerLink reports a link to a page or a group, which holds screens instead of being one.
func containerLink(err error) bool {
	var u *unsupportedNode
	return errors.As(err, &u) && (u.Type == figma.NodeCanvas || u.Type == figma.NodeGroup)
}
