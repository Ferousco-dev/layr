package designapi

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/ferousco-dev/layr/server/internal/designir"
	"github.com/ferousco-dev/layr/server/internal/imports"
	"github.com/ferousco-dev/layr/server/internal/project"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

const (
	// maxSummaryScreens keeps the design summary small; the screens endpoint pages through the rest.
	maxSummaryScreens  = 500
	maxSummaryWarnings = 200
	DefaultPageSize    = 100
	MaxPageSize        = 500
	maxQueryLength     = 100
)

// Projects checks project ownership; project.Service satisfies it.
type Projects interface {
	Get(ctx context.Context, ownerID, id string) (project.Project, error)
}

// Imports reads import metadata scoped to the owner.
type Imports interface {
	Latest(ctx context.Context, ownerID, projectID string) (imports.Import, error)
	LatestCompleted(ctx context.Context, ownerID, projectID string) (imports.Import, error)
}

// Workspaces opens an import's temporary workspace for reading.
type Workspaces interface {
	Existing(importID string) (*workspace.Dir, error)
}

type Config struct {
	Limits       designir.Limits
	MaxIRBytes   int64
	CacheEntries int
}

type Service struct {
	projects Projects
	imports  Imports
	ws       Workspaces
	loader   *loader
	log      *slog.Logger
}

func New(projects Projects, imports Imports, ws Workspaces, log *slog.Logger, cfg Config) *Service {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Service{projects: projects, imports: imports, ws: ws, log: log, loader: newLoader(ws, cfg.Limits, cfg.MaxIRBytes, cfg.CacheEntries)}
}

// state is what the owner's project currently has: a finished design, an import in flight, or both.
type state struct {
	projectID string
	current   *imports.Import
	latest    *imports.Import
}

// resolve proves ownership first, then finds the current design and the newest import.
func (s *Service) resolve(ctx context.Context, userID, projectID string) (*state, error) {
	id, err := project.NormalizeID(projectID)
	if err != nil {
		return nil, ErrProjectNotFound
	}
	if _, err := s.projects.Get(ctx, userID, id); err != nil {
		if errors.Is(err, project.ErrNotFound) {
			return nil, ErrProjectNotFound
		}
		return nil, err
	}

	st := &state{projectID: id}
	if imp, err := s.imports.LatestCompleted(ctx, userID, id); err == nil {
		st.current = &imp
	} else if !errors.Is(err, imports.ErrNotFound) {
		return nil, err
	}
	if imp, err := s.imports.Latest(ctx, userID, id); err == nil {
		st.latest = &imp
	} else if !errors.Is(err, imports.ErrNotFound) {
		return nil, err
	}
	if st.current == nil && st.latest == nil {
		return nil, ErrDesignNotFound
	}
	return st, nil
}

// ready loads the current design or explains why there is none.
func (s *Service) ready(ctx context.Context, userID, projectID string) (*state, *index, error) {
	st, err := s.resolve(ctx, userID, projectID)
	if err != nil {
		return nil, nil, err
	}
	if st.current == nil {
		return nil, nil, ErrNotReady
	}
	ix, err := s.loader.load(st.current.ID)
	if err != nil {
		return nil, nil, err
	}
	return st, ix, nil
}

// Design returns the project's design summary, or the state of the import that will produce it.
func (s *Service) Design(ctx context.Context, userID, projectID string) (*Design, error) {
	st, err := s.resolve(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	out := &Design{ProjectID: st.projectID, Screens: []Screen{}, Flows: []Flow{}, Warnings: []Warning{}, Selection: Selection{Modes: []string{}}}
	out.PendingImport = pending(st)

	if st.current == nil {
		s.notReady(out, st.latest)
		return out, nil
	}

	out.ImportID = st.current.ID
	out.Source = &Source{Provider: "figma", FileName: st.current.FileName, SourceVersion: st.current.Version}
	ix, err := s.loader.load(st.current.ID)
	switch {
	case errors.Is(err, ErrExpired):
		out.Status = StatusExpired
		out.Counts = &Counts{Screens: st.current.ScreenCount}
		return out, nil
	case err != nil:
		return nil, err
	}
	s.fill(out, ix)
	return out, nil
}

// notReady describes an import that has not produced a design yet.
func (s *Service) notReady(out *Design, latest *imports.Import) {
	out.ImportID = latest.ID
	out.PendingImport = nil
	if latest.FileName != "" {
		out.Source = &Source{Provider: "figma", FileName: latest.FileName, SourceVersion: latest.Version}
	}
	switch latest.Status {
	case imports.StatusAwaitingSelection:
		out.Status = StatusAwaitingSelection
	case imports.StatusFailed:
		out.Status = StatusFailed
		out.Error = &Error{Code: latest.ErrorCode, Message: imports.Message(latest.ErrorCode)}
	default:
		out.Status = StatusImporting
	}
}

// pending reports a newer import beside the current design, so a failed refresh never hides it.
func pending(st *state) *PendingImport {
	if st.current == nil || st.latest == nil || st.latest.ID == st.current.ID {
		return nil
	}
	p := &PendingImport{ImportID: st.latest.ID}
	switch st.latest.Status {
	case imports.StatusAwaitingSelection:
		p.Status = StatusAwaitingSelection
	case imports.StatusFailed:
		p.Status = StatusFailed
		p.Error = &Error{Code: st.latest.ErrorCode, Message: imports.Message(st.latest.ErrorCode)}
	case imports.StatusCompleted:
		return nil
	default:
		p.Status = StatusImporting
	}
	return p
}

func (s *Service) fill(out *Design, ix *index) {
	ir := ix.ir
	out.Status, out.DesignVersion = StatusReady, ix.version
	out.Source.Provider = ir.Source.Provider
	out.DesignSystem = ix.designSystem()

	shown := min(len(ir.Screens), maxSummaryScreens)
	out.Screens = make([]Screen, 0, shown)
	for i := 0; i < shown; i++ {
		out.Screens = append(out.Screens, ix.screenDTO(out.ProjectID, i))
	}
	out.ScreensTruncated = shown < len(ir.Screens)
	for i := range ir.Sections {
		out.Flows = append(out.Flows, ix.flowDTO(i))
	}

	withWarnings := 0
	for i, w := range ir.Warnings {
		if i < maxSummaryWarnings {
			out.Warnings = append(out.Warnings, mapWarning(w))
		}
	}
	for _, s := range ir.Screens {
		if len(ix.warnings[s.ID]) > 0 {
			withWarnings++
		}
	}

	ungrouped := 0
	for _, s := range ir.Screens {
		if ix.flowOf[s.ID] == "" {
			ungrouped++
		}
	}
	out.Counts = &Counts{
		Screens: len(ir.Screens), Flows: len(ir.Sections), UngroupedScreens: ungrouped, Warnings: len(ir.Warnings),
		ScreensWithWarnings: withWarnings, SharedComponents: ix.sharedComponents(),
	}
	out.Selection.Modes = []string{"one", "selected", "all"}
	if len(ir.Sections) > 0 {
		out.Selection.Modes = []string{"one", "selected", "flow", "all"}
	}
}

// Query filters the screen list.
type Query struct {
	Limit  int
	Offset int
	Search string
	FlowID string
}

// Screens lists screen summaries in design order, with optional name search and flow filter.
func (s *Service) Screens(ctx context.Context, userID, projectID string, q Query) (*ScreenPage, error) {
	if q.Limit == 0 {
		q.Limit = DefaultPageSize
	}
	if q.Limit < 1 || q.Limit > MaxPageSize || q.Offset < 0 || len(q.Search) > maxQueryLength {
		return nil, ErrInvalidQuery
	}
	st, ix, err := s.ready(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}

	var allowed map[string]bool
	if q.FlowID != "" {
		pos, ok := ix.flowPos[q.FlowID]
		if !ok {
			return nil, ErrFlowNotFound
		}
		allowed = map[string]bool{}
		for _, id := range ix.ir.Sections[pos].ScreenIDs {
			allowed[id] = true
		}
	}
	needle := strings.ToLower(strings.TrimSpace(q.Search))

	matches := make([]int, 0, len(ix.ir.Screens))
	for i, sc := range ix.ir.Screens {
		if allowed != nil && !allowed[sc.ID] {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(sc.Name), needle) {
			continue
		}
		matches = append(matches, i)
	}

	page := &ScreenPage{Screens: []Screen{}, Total: len(matches), Limit: q.Limit, Offset: q.Offset}
	for _, i := range matches[min(q.Offset, len(matches)):min(q.Offset+q.Limit, len(matches))] {
		page.Screens = append(page.Screens, ix.screenDTO(st.projectID, i))
	}
	return page, nil
}

func (ix *index) screen(id string) (int, bool) {
	i, ok := ix.position[id]
	return i, ok
}

// Screen returns one screen's details.
func (s *Service) Screen(ctx context.Context, userID, projectID, screenID string) (*ScreenDetail, error) {
	st, ix, err := s.ready(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	i, ok := ix.screen(screenID)
	if !ok {
		return nil, ErrScreenNotFound
	}
	sc := ix.ir.Screens[i]

	d := &ScreenDetail{
		Screen: ix.screenDTO(st.projectID, i), NodeCount: countNodes(&sc.Root), AssetCount: len(sc.AssetIDs),
		Components: []ComponentRef{}, Warnings: []Warning{}, DesignVersion: ix.version,
	}
	if sc.Root.Layout != nil {
		d.RootLayout = sc.Root.Layout.Mode
	}
	if pos, ok := ix.flowPos[d.FlowID]; ok {
		d.FlowName = ix.ir.Sections[pos].Name
	}
	for _, id := range sc.ComponentIDs {
		c := ix.components[id]
		d.Components = append(d.Components, ComponentRef{ID: id, Name: c.Name, InstanceCount: c.InstanceCount, Shared: len(c.ScreenIDs) > 1})
	}
	for _, w := range ix.warnings[sc.ID] {
		d.Warnings = append(d.Warnings, mapWarning(w))
	}
	return d, nil
}

// Flow returns one flow with its screens.
func (s *Service) Flow(ctx context.Context, userID, projectID, flowID string) (*FlowDetail, error) {
	st, ix, err := s.ready(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	pos, ok := ix.flowPos[flowID]
	if !ok {
		return nil, ErrFlowNotFound
	}
	out := &FlowDetail{Flow: ix.flowDTO(pos), Screens: []Screen{}}
	for _, id := range ix.ir.Sections[pos].ScreenIDs {
		if i, ok := ix.screen(id); ok {
			out.Screens = append(out.Screens, ix.screenDTO(st.projectID, i))
		}
	}
	return out, nil
}
