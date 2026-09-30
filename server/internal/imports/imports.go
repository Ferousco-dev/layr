// Package imports orchestrates a Figma import: URL, ownership, target choice, snapshot and reference render.
package imports

import (
	"context"
	"errors"
	"time"

	"github.com/ferousco-dev/layr/server/internal/assets"
	"github.com/ferousco-dev/layr/server/internal/figma"
	"github.com/ferousco-dev/layr/server/internal/workspace"
)

// Import statuses. Transitions are enforced by the repository, never by callers.
const (
	StatusPending           = "pending"
	StatusProcessing        = "processing"
	StatusAwaitingSelection = "awaiting_selection"
	StatusCompleted         = "completed"
	StatusFailed            = "failed"
)

// Stable failure codes stored on a failed import.
const (
	CodeWorkspaceFailed  = "WORKSPACE_FAILED"
	CodeNodeNotFound     = "FIGMA_NODE_NOT_FOUND"
	CodeUnsupportedNode  = "FIGMA_UNSUPPORTED_NODE"
	CodeNoFrames         = "FIGMA_NO_FRAMES"
	CodeImportFailed     = "FIGMA_IMPORT_FAILED"
	CodeSnapshotTooLarge = "SNAPSHOT_TOO_LARGE"
	CodeCancelled        = "IMPORT_CANCELLED"
	CodeTimeout          = "IMPORT_TIMEOUT"
	CodeSuperseded       = "IMPORT_SUPERSEDED"
	CodeInterrupted      = "IMPORT_INTERRUPTED"
	CodeTooManyScreens   = "TOO_MANY_SCREENS"
	CodeInternal         = "INTERNAL_ERROR"
)

var (
	ErrInvalidURL       = errors.New("figma url invalid")
	ErrProjectNotFound  = errors.New("project not found")
	ErrNotFound         = errors.New("import not found")
	ErrConflict         = errors.New("import conflict")
	ErrInvalidSelection = errors.New("selection is not a candidate")
	ErrInvalidID        = errors.New("id invalid")
	ErrBusy             = errors.New("importer busy")
)

// Frame is the minimal, safe description of an importable node.
type Frame struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Page string `json:"page,omitempty"`
}

type Import struct {
	ID             string
	ProjectID      string
	FileKey        string
	NodeID         string
	NodeIDs        []string
	NodeName       string
	FileName       string
	Version        string
	Status         string
	ErrorCode      string
	Candidates     []Frame
	RenderFormat   string
	RenderScale    float64
	AssetCount     int
	WarningCount   int
	ScreenCount    int
	DesignNodes    int
	DesignWarnings int
	CreatedAt      time.Time
	UpdatedAt      time.Time
	CompletedAt    *time.Time
}

// Result is what a successful import records.
type Result struct {
	FileName       string
	Version        string
	NodeID         string
	NodeIDs        []string
	NodeName       string
	RenderFormat   string
	RenderScale    float64
	AssetCount     int
	WarningCount   int
	ScreenCount    int
	DesignNodes    int
	DesignWarnings int
}

// Selection is the user's choice of screens for an import waiting on one.
type Selection struct {
	NodeIDs []string
	All     bool
}

// Repository methods that take an owner return ErrProjectNotFound or ErrNotFound for foreign rows.
type Repository interface {
	Create(ctx context.Context, ownerID, projectID, fileKey, nodeID string, now, staleBefore time.Time) (Import, error)
	Get(ctx context.Context, ownerID, projectID, importID string) (Import, error)
	Latest(ctx context.Context, ownerID, projectID string) (Import, error)
	LatestCompleted(ctx context.Context, ownerID, projectID string) (Import, error)
	BeginSelection(ctx context.Context, ownerID, projectID, importID string, sel Selection, maxScreens int, now time.Time) (Import, error)
	MarkProcessing(ctx context.Context, importID string, now time.Time) error
	AwaitSelection(ctx context.Context, importID, fileName, version string, frames []Frame, now time.Time) error
	Complete(ctx context.Context, importID string, r Result, now time.Time) error
	Fail(ctx context.Context, importID, code string, now time.Time) error
	RunningIDs(ctx context.Context) (map[string]bool, error)
	// FailAbandoned fails pending or processing imports untouched since before and reports how many it retired.
	FailAbandoned(ctx context.Context, before time.Time, code string, now time.Time) (int, error)
}

// FigmaAPI is the slice of the M1.4 client the importer uses; there is no other network access.
type FigmaAPI interface {
	GetFile(ctx context.Context, userID, fileKey string, opts figma.FileOptions) (*figma.File, error)
	GetFileNodes(ctx context.Context, userID, fileKey string, nodeIDs []string, opts figma.NodesOptions) (*figma.FileNodes, error)
	RenderNodes(ctx context.Context, userID, fileKey string, nodeIDs []string, opts figma.RenderOptions) (*figma.Renders, error)
}

// AssetPipeline downloads the reference render and every asset the target node references.
type AssetPipeline interface {
	SaveReferences(ctx context.Context, dir *workspace.Dir, userID, fileKey string, urls map[string]string) ([]assets.Reference, error)
	Run(ctx context.Context, in assets.Input) (assets.Summary, error)
}

type Workspaces interface {
	Create(importID string) (*workspace.Dir, error)
	Cleanup(importID string) error
	CleanupStale(ttl time.Duration, now time.Time, keep func(importID string) bool) (int, error)
}

// SupportedRoot lists node types that make sense as a whole design to import.
var SupportedRoot = map[string]bool{figma.NodeFrame: true, figma.NodeComponent: true, figma.NodeSection: true}

// maxCandidates bounds the frame list kept for selection.
const maxCandidates = 500

// Candidates returns the visible, importable top-level nodes of every page, in document order.
func Candidates(doc figma.Node) []Frame {
	var out []Frame
	for _, page := range doc.Children {
		for _, n := range page.Children {
			if SupportedRoot[n.Type] && n.IsVisible() {
				out = append(out, Frame{ID: n.ID, Name: n.Name, Type: n.Type, Page: page.Name})
			}
			if len(out) >= maxCandidates {
				return out
			}
		}
	}
	return out
}

// CandidatesAround lists the importable frames near focus: those of the page it names or sits on, else the whole file.
func CandidatesAround(doc figma.Node, focus string) []Frame {
	if focus != "" {
		for _, page := range doc.Children {
			onPage := page.ID == focus
			for _, n := range page.Children {
				onPage = onPage || n.ID == focus
			}
			if !onPage {
				continue
			}
			var out []Frame
			for _, n := range page.Children {
				if SupportedRoot[n.Type] && n.IsVisible() && len(out) < maxCandidates {
					out = append(out, Frame{ID: n.ID, Name: n.Name, Type: n.Type, Page: page.Name})
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return Candidates(doc)
}
