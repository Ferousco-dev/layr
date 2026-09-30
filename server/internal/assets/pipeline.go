package assets

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

const (
	vectorBatch    = 50
	maxAttempts    = 3
	maxReresolves  = 20
	maxWarnings    = 200
	downloadTmp    = "download.tmp"
	longestBackoff = 10 * time.Second
)

// FigmaAPI is the part of the M1.4 client the pipeline needs.
type FigmaAPI interface {
	GetImageFills(ctx context.Context, userID, fileKey string) (map[string]string, error)
	RenderNodes(ctx context.Context, userID, fileKey string, nodeIDs []string, opts figma.RenderOptions) (*figma.Renders, error)
}

// Fetcher downloads a provider URL into dst; *Downloader is the production implementation.
type Fetcher interface {
	Fetch(ctx context.Context, rawURL string, dst io.Writer) (Fetched, error)
}

type Config struct {
	// MaxAssetBytes bounds one file; MaxImportBytes bounds all assets of one import.
	MaxAssetBytes  int64
	MaxImportBytes int64
	// Concurrency is the number of simultaneous downloads.
	Concurrency int
	// MaxAssets bounds how many files one import may produce.
	MaxAssets int
}

type Pipeline struct {
	figma FigmaAPI
	dl    Fetcher
	cfg   Config
	log   *slog.Logger
	sleep func(ctx context.Context, d time.Duration) error
}

func NewPipeline(api FigmaAPI, dl Fetcher, log *slog.Logger, cfg Config) *Pipeline {
	if cfg.Concurrency < 1 {
		cfg.Concurrency = 4
	}
	if cfg.MaxAssets < 1 {
		cfg.MaxAssets = 300
	}
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &Pipeline{figma: api, dl: dl, cfg: cfg, log: log, sleep: sleepCtx}
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// Input is one import's design and workspace.
type Input struct {
	ImportID   string
	UserID     string
	FileKey    string
	NodeIDs    []string
	Root       figma.Node
	Dir        *workspace.Dir
	References []Reference
}

// Summary is what the importer records.
type Summary struct {
	Assets   int
	Warnings int
}

// budget tracks the bytes of all assets of one import as they stream in.
type budget struct {
	limit int64
	used  atomic.Int64
}

func (b *budget) reserve(n int64) bool {
	if b.used.Add(n) > b.limit {
		b.used.Add(-n)
		return false
	}
	return true
}

func (b *budget) release(n int64) { b.used.Add(-n) }

// sink hashes, counts and limits everything written to a download.
type sink struct {
	w      io.Writer
	hash   hash.Hash
	head   []byte
	n      int64
	max    int64
	budget *budget
}

func newSink(w io.Writer, max int64, b *budget) *sink {
	return &sink{w: w, hash: sha256.New(), max: max, budget: b}
}

func (s *sink) Write(p []byte) (int, error) {
	if s.n+int64(len(p)) > s.max {
		return 0, ErrTooLarge
	}
	if !s.budget.reserve(int64(len(p))) {
		return 0, ErrBudgetExceeded
	}
	n, err := s.w.Write(p)
	s.budget.release(int64(len(p) - n))
	s.n += int64(n)
	s.hash.Write(p[:n])
	if room := HeadBytes - len(s.head); room > 0 {
		if n < room {
			room = n
		}
		s.head = append(s.head, p[:room]...)
	}
	return n, err
}

func (s *sink) sum() string { return hex.EncodeToString(s.hash.Sum(nil)) }

// task is one file to obtain.
type task struct {
	order    int
	key      string
	nodeID   string
	name     string
	imageRef string
	sources  []Source
	url      string
}

// entry is an asset being assembled under a provisional name until final names are assigned.
type entry struct {
	asset *Asset
	// order and name come from the earliest task that uses the content, so naming is stable.
	order int
	name  string
	// fixed entries were verified from an earlier run and keep their existing path.
	fixed       bool
	provisional string
	report      SanitizeReport
}

// run holds the state shared by the workers of one Run call.
type run struct {
	p         *Pipeline
	in        Input
	budget    *budget
	mu        sync.Mutex
	bySHA     map[string]*entry
	paths     map[string]string
	warnings  []Warning
	reresolve atomic.Int32
	fills     map[string]string
}

func (r *run) warn(w Warning) {
	r.mu.Lock()
	r.warnings = append(r.warnings, w)
	r.mu.Unlock()
}

// Run discovers, resolves, downloads and records every asset of an imported design.
func (p *Pipeline) Run(ctx context.Context, in Input) (Summary, error) {
	work := Discover(in.Root)
	r := &run{
		p: p, in: in, budget: &budget{limit: p.cfg.MaxImportBytes},
		bySHA: map[string]*entry{}, paths: map[string]string{}, warnings: work.Warnings,
	}
	tasks := r.plan(work)
	r.reuse(&tasks)

	if err := r.resolve(ctx, tasks); err != nil {
		return Summary{}, err
	}
	live := tasks[:0]
	for _, t := range tasks {
		if t.url != "" {
			live = append(live, t)
		}
	}
	if err := r.downloadAll(ctx, live); err != nil {
		return Summary{}, err
	}
	return r.finish()
}

// plan turns discovery into tasks, keeping the count within MaxAssets (images before vectors).
func (r *run) plan(work Work) []*task {
	var tasks []*task
	for _, w := range work.Images {
		t := &task{key: "image:" + w.Ref, imageRef: w.Ref, name: firstName(w.Uses)}
		t.nodeID = w.Uses[0].NodeID
		for _, u := range w.Uses {
			t.sources = append(t.sources, Source{
				NodeID: u.NodeID, ImageRef: w.Ref, Role: roleOf(u.Field),
				ScaleMode: u.ScaleMode, Rotation: u.Rotation, HasTransform: u.Transform,
			})
		}
		tasks = append(tasks, t)
	}
	for _, v := range work.Vectors {
		tasks = append(tasks, &task{
			key: "node:" + v.NodeID, nodeID: v.NodeID, name: v.NodeName,
			sources: []Source{{NodeID: v.NodeID, Role: "export"}},
		})
	}
	for i, t := range tasks {
		t.order = i
	}
	if len(tasks) > r.p.cfg.MaxAssets {
		dropped := len(tasks) - r.p.cfg.MaxAssets
		tasks = tasks[:r.p.cfg.MaxAssets]
		r.warn(Warning{Code: WarnTooManyAssets, Message: fmt.Sprintf("%d assets beyond the limit of %d were skipped.", dropped, r.p.cfg.MaxAssets)})
	}
	return tasks
}

func roleOf(field string) string {
	if field == "strokes" {
		return "stroke"
	}
	return "fill"
}

func firstName(uses []ImageUse) string {
	for _, u := range uses {
		if u.NodeName != "" {
			return u.NodeName
		}
	}
	return ""
}

// reuse keeps assets from an earlier run whose files still match their recorded checksum.
func (r *run) reuse(tasks *[]*task) {
	old, err := LoadManifest(r.in.Dir)
	if err != nil {
		return
	}
	verified := map[string]*Asset{}
	for i := range old.Assets {
		a := old.Assets[i]
		if verifyStored(r.in.Dir, a) {
			verified[a.SHA256] = &a
		}
	}

	remaining := (*tasks)[:0]
	for _, t := range *tasks {
		if a := findReusable(old, verified, t); a != nil {
			r.adopt(a, t)
			continue
		}
		remaining = append(remaining, t)
	}
	*tasks = remaining
	r.budgetSeed()
}

func findReusable(old *Manifest, verified map[string]*Asset, t *task) *Asset {
	for _, a := range old.Assets {
		if verified[a.SHA256] == nil {
			continue
		}
		for _, s := range a.Sources {
			if (t.imageRef != "" && s.ImageRef == t.imageRef) || (t.imageRef == "" && s.ImageRef == "" && s.NodeID == t.nodeID) {
				return verified[a.SHA256]
			}
		}
	}
	return nil
}

// adopt records a verified existing asset, refreshing its sources from the current design.
func (r *run) adopt(existing *Asset, t *task) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.bySHA[existing.SHA256]
	if !ok {
		clone := *existing
		clone.Sources = nil
		e = &entry{asset: &clone, fixed: true}
		r.bySHA[clone.SHA256] = e
		r.paths[clone.Path] = clone.SHA256
	}
	e.asset.Sources = mergeSources(e.asset.Sources, t.sources)
}

func (r *run) budgetSeed() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.bySHA {
		r.budget.used.Add(e.asset.SizeBytes)
	}
}

func mergeSources(have, add []Source) []Source {
	for _, s := range add {
		dup := false
		for _, h := range have {
			if h == s {
				dup = true
				break
			}
		}
		if !dup {
			have = append(have, s)
		}
	}
	return have
}

// resolve asks Figma for the temporary URLs: one image-fill lookup, and SVG exports in batches.
func (r *run) resolve(ctx context.Context, tasks []*task) error {
	var images, vectors []*task
	for _, t := range tasks {
		if t.imageRef != "" {
			images = append(images, t)
		} else {
			vectors = append(vectors, t)
		}
	}

	if len(images) > 0 {
		fills, err := r.p.figma.GetImageFills(ctx, r.in.UserID, r.in.FileKey)
		if err != nil {
			return err
		}
		r.fills = fills
		for _, t := range images {
			if t.url = fills[t.imageRef]; t.url == "" {
				r.warn(Warning{Code: WarnImageUnresolved, NodeID: t.nodeID, ImageRef: t.imageRef, Message: "Figma did not return this image."})
			}
		}
	}

	for start := 0; start < len(vectors); start += vectorBatch {
		end := min(start+vectorBatch, len(vectors))
		ids := make([]string, 0, end-start)
		for _, t := range vectors[start:end] {
			ids = append(ids, t.nodeID)
		}
		renders, err := r.p.figma.RenderNodes(ctx, r.in.UserID, r.in.FileKey, ids, figma.RenderOptions{Format: figma.FormatSVG})
		if err != nil {
			return err
		}
		for _, t := range vectors[start:end] {
			if t.url = renders.URLs[t.nodeID]; t.url == "" {
				r.warn(Warning{Code: WarnVectorUnresolved, NodeID: t.nodeID, Message: "Figma could not export this node as SVG."})
			}
		}
	}
	return nil
}

// downloadAll runs the tasks on a bounded pool and stops everything on the first fatal error.
func (r *run) downloadAll(ctx context.Context, tasks []*task) error {
	return parallel(ctx, len(tasks), r.p.cfg.Concurrency, func(ctx context.Context, i int) error {
		return r.fetchAsset(ctx, tasks[i])
	})
}

// finish names assets in a fixed order, removes leftovers, then writes the manifest last.
func (r *run) finish() (Summary, error) {
	r.mu.Lock()
	entries := make([]*entry, 0, len(r.bySHA))
	for _, e := range r.bySHA {
		entries = append(entries, e)
	}
	r.mu.Unlock()

	if err := r.nameAssets(entries); err != nil {
		return Summary{}, &Error{Code: CodeDownloadFailed, Err: err}
	}
	r.sanitizeWarnings(entries)

	m := &Manifest{
		ImportID:   r.in.ImportID,
		Source:     ManifestSource{FileKey: r.in.FileKey, NodeID: firstOf(r.in.NodeIDs), NodeIDs: r.in.NodeIDs},
		References: r.in.References,
	}
	keep := map[string]bool{manifestName: true}
	for _, e := range entries {
		m.Assets = append(m.Assets, *e.asset)
		if name, ok := baseName(e.asset.Path); ok {
			keep[name] = true
		}
	}
	r.mu.Lock()
	m.Warnings = append(m.Warnings, r.warnings...)
	r.mu.Unlock()
	if len(m.Warnings) > maxWarnings {
		m.Warnings = m.Warnings[:maxWarnings]
	}

	if names, err := r.in.Dir.Names(workspace.Assets); err == nil {
		for _, n := range names {
			if !keep[n] {
				_ = r.in.Dir.Remove(workspace.Assets, n)
			}
		}
	}
	if err := m.Save(r.in.Dir); err != nil {
		return Summary{}, &Error{Code: CodeDownloadFailed, Err: err}
	}
	return Summary{Assets: len(m.Assets), Warnings: len(m.Warnings)}, nil
}

// sanitizeWarnings reports each sanitized SVG once, against its lowest node ID, so it is deterministic.
func (r *run) sanitizeWarnings(entries []*entry) {
	for _, e := range entries {
		if !e.asset.Sanitized {
			continue
		}
		node := ""
		for _, s := range e.asset.Sources {
			if node == "" || s.NodeID < node {
				node = s.NodeID
			}
		}
		msg := "The SVG had unsafe content removed when it was first stored."
		if !e.fixed {
			msg = fmt.Sprintf("Removed %d unsafe elements and %d unsafe attributes from the SVG.", e.report.Elements, e.report.Attributes)
		}
		r.warn(Warning{Code: WarnSVGSanitized, NodeID: node, Message: msg})
	}
}

// nameAssets renames provisional files to slug-hash.ext in design order, not download order.
func (r *run) nameAssets(entries []*entry) error {
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].order != entries[j].order {
			return entries[i].order < entries[j].order
		}
		return entries[i].asset.SHA256 < entries[j].asset.SHA256
	})

	taken := map[string]string{}
	for _, e := range entries {
		if e.fixed {
			taken[e.asset.Path] = e.asset.SHA256
		}
	}
	for _, e := range entries {
		if e.fixed {
			continue
		}
		a := e.asset
		fallback := "image"
		if a.Kind == "svg" {
			fallback = "vector"
		}
		ext := strings.TrimPrefix(e.provisional[strings.LastIndex(e.provisional, "."):], ".")
		name := fileName(e.name, fallback, a.SHA256, ext, 6)
		if other, clash := taken["assets/"+name]; clash && other != a.SHA256 {
			name = fileName(e.name, fallback, a.SHA256, ext, 12)
		}
		if err := r.in.Dir.Rename(workspace.Assets, e.provisional, name); err != nil {
			return err
		}
		a.Path = "assets/" + name
		taken[a.Path] = a.SHA256
	}
	return nil
}

// fetchAsset downloads, validates and stores one task, retrying only what is worth retrying.
func (r *run) fetchAsset(ctx context.Context, t *task) error {
	got, err := r.p.download(ctx, r.in.Dir, workspace.Assets, r.budget, r.p.cfg.MaxAssetBytes, t.url, t.nodeID,
		func(ctx context.Context) (string, error) { return r.refresh(ctx, t) })
	if err != nil {
		return err
	}
	return r.store(ctx, t, got)
}

// refresh gets a new temporary URL for a task whose URL expired; the total is bounded.
func (r *run) refresh(ctx context.Context, t *task) (string, error) {
	if r.reresolve.Add(1) > maxReresolves {
		return "", ErrExpired
	}
	if t.imageRef != "" {
		fills, err := r.p.figma.GetImageFills(ctx, r.in.UserID, r.in.FileKey)
		if err != nil {
			return "", err
		}
		return fills[t.imageRef], nil
	}
	renders, err := r.p.figma.RenderNodes(ctx, r.in.UserID, r.in.FileKey, []string{t.nodeID}, figma.RenderOptions{Format: figma.FormatSVG})
	if err != nil {
		return "", err
	}
	return renders.URLs[t.nodeID], nil
}

// download is the shared retry loop: expired URLs are re-resolved once, transient failures retried.
func (p *Pipeline) download(ctx context.Context, dir *workspace.Dir, sub string, b *budget, max int64, url, nodeID string, refresh func(context.Context) (string, error)) (*downloaded, error) {
	refreshed := false
	for attempt := 1; ; attempt++ {
		snap, err := dir.NewSnapshot(sub, downloadTmp)
		if err != nil {
			return nil, &Error{Code: CodeDownloadFailed, NodeID: nodeID, Err: err}
		}
		s := newSink(snap, max, b)
		res, err := p.dl.Fetch(ctx, url, s)
		if err == nil {
			return &downloaded{snap: snap, sink: s, contentType: res.ContentType, dir: dir, budget: b}, nil
		}
		snap.Abort()
		b.release(s.n)

		var status *StatusError
		switch {
		case ctx.Err() != nil:
			return nil, ctx.Err()
		case errors.As(err, &status) && expiredStatus(status.Status) && !refreshed:
			refreshed = true
			fresh, rerr := refresh(ctx)
			if rerr != nil {
				return nil, rerr
			}
			if fresh == "" {
				return nil, &Error{Code: CodeResolveFailed, NodeID: nodeID, Err: err}
			}
			url = fresh
			continue
		case (errors.As(err, &status) && transientStatus(status.Status)) || errors.Is(err, ErrDownload):
			if attempt >= maxAttempts {
				return nil, wrapFetch(err, nodeID)
			}
			if serr := p.sleep(ctx, backoff(attempt)); serr != nil {
				return nil, serr
			}
			continue
		}
		return nil, wrapFetch(err, nodeID)
	}
}

func expiredStatus(s int) bool {
	return s == http.StatusForbidden || s == http.StatusNotFound || s == http.StatusGone
}

func transientStatus(s int) bool {
	return s == http.StatusTooManyRequests || s == http.StatusInternalServerError || s == http.StatusBadGateway ||
		s == http.StatusServiceUnavailable || s == http.StatusGatewayTimeout
}

func backoff(attempt int) time.Duration {
	return min(250*time.Millisecond<<(attempt-1), longestBackoff)
}

func wrapFetch(err error, nodeID string) error {
	code := CodeDownloadFailed
	switch {
	case errors.Is(err, ErrTooLarge):
		code = CodeTooLarge
	case errors.Is(err, ErrBudgetExceeded):
		code = CodeBudgetExceeded
	case errors.Is(err, ErrURLBlocked):
		code = CodeURLBlocked
	}
	return &Error{Code: code, NodeID: nodeID, Err: err}
}

// downloaded is a finished download waiting in a temporary file for validation.
type downloaded struct {
	snap        *workspace.Snapshot
	sink        *sink
	contentType string
	dir         *workspace.Dir
	budget      *budget
}

func (d *downloaded) discard() {
	d.snap.Abort()
	d.budget.release(d.sink.n)
}

// checked is validated content ready to be named and stored.
type checked struct {
	format    Format
	svg       []byte
	sha       string
	size      int64
	width     *float64
	height    *float64
	sanitized SanitizeReport
}

// validate decides the real format from the bytes; headers alone are never trusted.
func (d *downloaded) validate(nodeID string) (*checked, error) {
	invalid := func(err error) (*checked, error) {
		return nil, &Error{Code: CodeInvalidContent, NodeID: nodeID, Err: err}
	}
	if err := checkDeclaredType(d.contentType); err != nil {
		return invalid(err)
	}
	head := d.sink.head

	if f, ok := sniff(head); ok {
		w, h, dimsOK := dimensions(f, head)
		if !dimsOK {
			return invalid(ErrUnsupportedContent)
		}
		wf, hf := float64(w), float64(h)
		return &checked{format: f, sha: d.sink.sum(), size: d.sink.n, width: &wf, height: &hf}, nil
	}
	if !looksLikeSVG(head) {
		return invalid(ErrUnsupportedContent)
	}

	raw, err := d.snap.ReadBack(maxSVGBytes + 1)
	if err != nil {
		return invalid(err)
	}
	clean, report, err := sanitizeSVG(raw)
	if err != nil {
		return invalid(err)
	}
	c := &checked{format: formatSVG, svg: clean, sha: sumHex(clean), size: int64(len(clean)), sanitized: report}
	if w, h, ok := svgSize(clean); ok {
		c.width, c.height = &w, &h
	}
	return c, nil
}

// store names and commits validated content, or merges it into an identical asset already stored.
func (r *run) store(ctx context.Context, t *task, d *downloaded) error {
	c, err := d.validate(t.nodeID)
	if err != nil {
		d.discard()
		return err
	}
	if err := ctx.Err(); err != nil {
		d.discard()
		return err
	}

	r.mu.Lock()
	if existing, ok := r.bySHA[c.sha]; ok {
		existing.asset.Sources = mergeSources(existing.asset.Sources, t.sources)
		if !existing.fixed && t.order < existing.order {
			existing.order, existing.name = t.order, t.name
		}
		r.mu.Unlock()
		d.discard()
		return nil
	}
	provisional := c.sha[:16] + "." + c.format.Extension
	asset := &Asset{
		ID: assetID(c.sha), Kind: kindOf(c.format), Format: c.format.Name, MediaType: c.format.MediaType,
		Path: "assets/" + provisional, SizeBytes: c.size, SHA256: c.sha, Width: c.width, Height: c.height,
		Sanitized: c.sanitized.Changed(), Sources: mergeSources(nil, t.sources),
	}
	r.bySHA[c.sha] = &entry{asset: asset, order: t.order, name: t.name, provisional: provisional, report: c.sanitized}
	r.paths[asset.Path] = c.sha
	r.mu.Unlock()

	if err := r.commit(d, c, provisional); err != nil {
		r.mu.Lock()
		delete(r.bySHA, c.sha)
		delete(r.paths, asset.Path)
		r.mu.Unlock()
		d.discard()
		return &Error{Code: CodeDownloadFailed, NodeID: t.nodeID, Err: err}
	}
	return nil
}

func kindOf(f Format) string {
	if f == formatSVG {
		return "svg"
	}
	return "image"
}

// commit moves the streamed file into place, or writes the sanitized SVG in its stead.
func (r *run) commit(d *downloaded, c *checked, name string) error {
	if c.svg == nil {
		return d.snap.CommitAs(name)
	}
	d.snap.Abort()
	d.budget.release(d.sink.n)
	if !d.budget.reserve(int64(len(c.svg))) {
		return ErrBudgetExceeded
	}
	return d.dir.WriteBytes(workspace.Assets, name, c.svg)
}

func firstOf(ids []string) string {
	if len(ids) == 0 {
		return ""
	}
	return ids[0]
}

// SaveReferences downloads Figma's render of each screen into reference/, one PNG per node ID.
func (p *Pipeline) SaveReferences(ctx context.Context, dir *workspace.Dir, userID, fileKey string, urls map[string]string) ([]Reference, error) {
	ids := make([]string, 0, len(urls))
	for id := range urls {
		ids = append(ids, id)
	}
	sort.Strings(ids)

	b := &budget{limit: p.cfg.MaxImportBytes}
	refs := make([]Reference, len(ids))
	err := parallel(ctx, len(ids), p.cfg.Concurrency, func(ctx context.Context, i int) error {
		ref, err := p.saveReference(ctx, dir, b, userID, fileKey, ids[i], urls[ids[i]])
		refs[i] = ref
		return err
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

// referenceFile is a safe, unique file name for a node ID such as "12:34" or "I5:1;2:3".
func referenceFile(nodeID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(nodeID) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	name := strings.Trim(b.String(), "-")
	if len(name) > 30 {
		name = name[:30]
	}
	if name == "" {
		name = "screen"
	}
	return name + "-" + sumHex([]byte(nodeID))[:6] + ".png"
}

func (p *Pipeline) saveReference(ctx context.Context, dir *workspace.Dir, b *budget, userID, fileKey, nodeID, url string) (Reference, error) {
	refresh := func(ctx context.Context) (string, error) {
		renders, err := p.figma.RenderNodes(ctx, userID, fileKey, []string{nodeID}, figma.RenderOptions{Format: figma.FormatPNG, Scale: 1})
		if err != nil {
			return "", err
		}
		return renders.URLs[nodeID], nil
	}
	d, err := p.download(ctx, dir, workspace.Reference, b, p.cfg.MaxAssetBytes, url, nodeID, refresh)
	if err != nil {
		return Reference{}, err
	}
	c, err := d.validate(nodeID)
	if err == nil && c.format != formatPNG {
		err = &Error{Code: CodeInvalidContent, NodeID: nodeID, Err: ErrUnsupportedContent}
	}
	if err != nil {
		d.discard()
		return Reference{}, err
	}
	name := referenceFile(nodeID)
	if err := d.snap.CommitAs(name); err != nil {
		d.snap.Abort()
		return Reference{}, &Error{Code: CodeDownloadFailed, NodeID: nodeID, Err: err}
	}
	return Reference{
		NodeID: nodeID, Path: workspace.Reference + "/" + name, MediaType: c.format.MediaType,
		SizeBytes: c.size, SHA256: c.sha, Width: c.width, Height: c.height,
	}, nil
}

// parallel runs fn for 0..count-1 on a bounded pool and cancels the rest on the first error.
func parallel(ctx context.Context, count, workers int, fn func(ctx context.Context, i int) error) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errs := make([]error, count)
	sem := make(chan struct{}, max(workers, 1))
	var wg sync.WaitGroup
	for i := 0; i < count; i++ {
		select {
		case sem <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			defer func() { <-sem }()
			if err := fn(ctx, i); err != nil {
				errs[i] = err
				cancel()
			}
		}(i)
	}
	wg.Wait()

	var cancelled error
	for _, err := range errs {
		switch {
		case err == nil:
		case errors.Is(err, context.Canceled):
			cancelled = err
		default:
			return err
		}
	}
	if cancelled != nil {
		return cancelled
	}
	return context.Cause(ctx)
}
