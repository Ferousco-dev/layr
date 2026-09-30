package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/ferousco-dev/layr/server/internal/imports"
	"github.com/ferousco-dev/layr/server/internal/project"
)

type ImportService interface {
	Start(ctx context.Context, userID, projectID, rawURL string) (imports.Import, error)
	Latest(ctx context.Context, userID, projectID string) (imports.Import, error)
	Get(ctx context.Context, userID, projectID, importID string) (imports.Import, error)
	Select(ctx context.Context, userID, projectID, importID string, sel imports.Selection) (imports.Import, error)
	Refresh(ctx context.Context, userID, projectID string) (imports.Import, error)
	RetryAfter(userID string) time.Duration
}

type importRoutes struct {
	svc     ImportService
	require func(http.HandlerFunc) http.HandlerFunc
	budget  func(http.HandlerFunc) http.HandlerFunc
	log     *slog.Logger
}

func (i *importRoutes) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/projects/{projectID}/import", i.require(methods(map[string]http.HandlerFunc{
		http.MethodPost: i.budget(i.start),
		http.MethodGet:  i.latest,
	})))
	mux.HandleFunc("/api/v1/projects/{projectID}/import/refresh", i.require(methods(map[string]http.HandlerFunc{
		http.MethodPost: i.budget(i.refresh),
	})))
	mux.HandleFunc("/api/v1/projects/{projectID}/imports/{importID}", i.require(methods(map[string]http.HandlerFunc{
		http.MethodGet: i.get,
	})))
	mux.HandleFunc("/api/v1/projects/{projectID}/imports/{importID}/select", i.require(methods(map[string]http.HandlerFunc{
		http.MethodPost: i.budget(i.selectFrame),
	})))
}

func (i *importRoutes) start(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FigmaURL *string `json:"figma_url"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.FigmaURL == nil {
		failure(w, r, http.StatusBadRequest, "INVALID_FIGMA_URL", imports.Message("INVALID_FIGMA_URL"))
		return
	}

	user, _ := CurrentUser(r.Context())
	imp, err := i.svc.Start(r.Context(), user.ID, r.PathValue("projectID"), *body.FigmaURL)
	if err != nil {
		i.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"data": importJSON(imp, i.svc.RetryAfter(user.ID))})
}

// refresh runs the project's import again from Figma; it takes no input, so nothing can be smuggled in.
func (i *importRoutes) refresh(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	imp, err := i.svc.Refresh(r.Context(), user.ID, r.PathValue("projectID"))
	if err != nil {
		i.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"data": importJSON(imp, i.svc.RetryAfter(user.ID))})
}

func (i *importRoutes) selectFrame(w http.ResponseWriter, r *http.Request) {
	var body struct {
		NodeID  *string  `json:"node_id"`
		NodeIDs []string `json:"node_ids"`
		All     bool     `json:"all"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	sel, ok := selectionFrom(body.NodeID, body.NodeIDs, body.All)
	if !ok {
		failure(w, r, http.StatusBadRequest, "INVALID_SELECTION", imports.Message("INVALID_SELECTION"))
		return
	}

	user, _ := CurrentUser(r.Context())
	imp, err := i.svc.Select(r.Context(), user.ID, r.PathValue("projectID"), r.PathValue("importID"), sel)
	if err != nil {
		i.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"data": importJSON(imp, i.svc.RetryAfter(user.ID))})
}

// selectionFrom accepts exactly one of node_id, node_ids or all.
func selectionFrom(nodeID *string, nodeIDs []string, all bool) (imports.Selection, bool) {
	modes := 0
	sel := imports.Selection{}
	if nodeID != nil {
		modes++
		sel.NodeIDs = []string{*nodeID}
	}
	if len(nodeIDs) > 0 {
		modes++
		sel.NodeIDs = nodeIDs
	}
	if all {
		modes++
		sel.All = true
	}
	if modes != 1 {
		return imports.Selection{}, false
	}
	for _, id := range sel.NodeIDs {
		if id == "" {
			return imports.Selection{}, false
		}
	}
	return sel, true
}

func (i *importRoutes) latest(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	imp, err := i.svc.Latest(r.Context(), user.ID, r.PathValue("projectID"))
	if err != nil {
		i.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": importJSON(imp, i.svc.RetryAfter(user.ID))})
}

func (i *importRoutes) get(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	imp, err := i.svc.Get(r.Context(), user.ID, r.PathValue("projectID"), r.PathValue("importID"))
	if err != nil {
		i.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": importJSON(imp, i.svc.RetryAfter(user.ID))})
}

// fail maps importer errors; a foreign project or import looks exactly like a missing one.
func (i *importRoutes) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, imports.ErrInvalidURL):
		failure(w, r, http.StatusBadRequest, "INVALID_FIGMA_URL", imports.Message("INVALID_FIGMA_URL"))
	case errors.Is(err, imports.ErrInvalidID):
		failure(w, r, http.StatusBadRequest, "INVALID_ID", imports.Message("INVALID_ID"))
	case errors.Is(err, imports.ErrInvalidSelection):
		failure(w, r, http.StatusBadRequest, "INVALID_SELECTION", imports.Message("INVALID_SELECTION"))
	case errors.Is(err, imports.ErrProjectNotFound), errors.Is(err, project.ErrNotFound):
		failure(w, r, http.StatusNotFound, "PROJECT_NOT_FOUND", "The project does not exist.")
	case errors.Is(err, imports.ErrNotFound):
		failure(w, r, http.StatusNotFound, "IMPORT_NOT_FOUND", imports.Message("IMPORT_NOT_FOUND"))
	case errors.Is(err, imports.ErrConflict):
		failure(w, r, http.StatusConflict, "IMPORT_CONFLICT", imports.Message("IMPORT_CONFLICT"))
	case errors.Is(err, imports.ErrRateLimited):
		var rl *imports.RateLimitedError
		errors.As(err, &rl)
		w.Header().Set("Retry-After", strconv.Itoa(int(rl.RetryAfter.Seconds())+1))
		failure(w, r, http.StatusTooManyRequests, "FIGMA_RATE_LIMITED", imports.RateLimitMessage(rl.RetryAfter))
	case errors.Is(err, imports.ErrBusy):
		w.Header().Set("Retry-After", "5")
		failure(w, r, http.StatusServiceUnavailable, "IMPORT_BUSY", imports.Message("IMPORT_BUSY"))
	default:
		unexpected(w, r, i.log, "import.request_failed", err)
	}
}

func nodeIDsOrNil(imp imports.Import) any {
	if len(imp.NodeIDs) == 0 {
		return nil
	}
	return imp.NodeIDs
}

func importMessage(code string) string { return imports.Message(code) }

// importJSON exposes safe metadata only: never raw Figma data, temporary URLs or file paths.
func importJSON(imp imports.Import, wait time.Duration) map[string]any {
	out := map[string]any{
		"id":                 imp.ID,
		"project_id":         imp.ProjectID,
		"status":             imp.Status,
		"figma_file_key":     imp.FileKey,
		"figma_file_name":    optional(imp.FileName),
		"figma_version":      optional(imp.Version),
		"figma_node_id":      optional(imp.NodeID),
		"figma_node_name":    optional(imp.NodeName),
		"figma_node_ids":     nodeIDsOrNil(imp),
		"screens":            imp.ScreenCount,
		"requires_selection": imp.Status == imports.StatusAwaitingSelection,
		"created_at":         imp.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at":         imp.UpdatedAt.UTC().Format(time.RFC3339Nano),
		"completed_at":       nil,
	}
	if imp.CompletedAt != nil {
		out["completed_at"] = imp.CompletedAt.UTC().Format(time.RFC3339Nano)
	}
	switch imp.Status {
	case imports.StatusAwaitingSelection:
		frames := make([]map[string]string, 0, len(imp.Candidates))
		for _, f := range imp.Candidates {
			frames = append(frames, map[string]string{"id": f.ID, "name": f.Name, "type": f.Type, "page": f.Page})
		}
		out["frames"] = frames
	case imports.StatusCompleted:
		out["reference_render"] = map[string]any{"node_id": imp.NodeID, "format": imp.RenderFormat, "scale": imp.RenderScale}
		out["assets"] = map[string]int{"count": imp.AssetCount, "warnings": imp.WarningCount}
		out["design"] = map[string]int{"screens": imp.ScreenCount, "nodes": imp.DesignNodes, "warnings": imp.DesignWarnings}
	case imports.StatusFailed:
		apiErr := map[string]any{"code": imp.ErrorCode, "message": importMessage(imp.ErrorCode)}
		if imp.ErrorCode == "FIGMA_RATE_LIMITED" {
			apiErr["message"] = imports.RateLimitMessage(wait)
			apiErr["retry_after_seconds"] = int(wait.Seconds())
		}
		out["error"] = apiErr
	}
	return out
}
