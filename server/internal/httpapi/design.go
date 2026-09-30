package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/ferousco-dev/layr/server/internal/designapi"
	"github.com/ferousco-dev/layr/server/internal/httpapi/middleware"
)

// DesignService is the read side of a project's design; every method proves ownership first.
type DesignService interface {
	Design(ctx context.Context, userID, projectID string) (*designapi.Design, error)
	Screens(ctx context.Context, userID, projectID string, q designapi.Query) (*designapi.ScreenPage, error)
	Screen(ctx context.Context, userID, projectID, screenID string) (*designapi.ScreenDetail, error)
	Flow(ctx context.Context, userID, projectID, flowID string) (*designapi.FlowDetail, error)
	Tokens(ctx context.Context, userID, projectID string) (*designapi.DesignTokens, error)
	Assets(ctx context.Context, userID, projectID string) ([]designapi.AssetItem, error)
	Asset(ctx context.Context, userID, projectID, assetID string) (*designapi.AssetFile, error)
	Preview(ctx context.Context, userID, projectID, screenID string) (*designapi.PreviewFile, error)
}

type designRoutes struct {
	svc     DesignService
	require func(http.HandlerFunc) http.HandlerFunc
	log     *slog.Logger
}

var (
	screenIDPattern = regexp.MustCompile(`^screen_[0-9a-f]{16}$`)
	flowIDPattern   = regexp.MustCompile(`^section_[0-9a-f]{16}$`)
	assetIDPattern  = regexp.MustCompile(`^asset_[0-9a-f]{16}$`)
)

func (d *designRoutes) register(mux *http.ServeMux) {
	get := func(h http.HandlerFunc) http.HandlerFunc {
		return d.require(methods(map[string]http.HandlerFunc{http.MethodGet: h}))
	}
	mux.HandleFunc("/api/v1/projects/{projectID}/design", get(d.design))
	mux.HandleFunc("/api/v1/projects/{projectID}/design/tokens", get(d.tokens))
	mux.HandleFunc("/api/v1/projects/{projectID}/design/assets", get(d.assets))
	mux.HandleFunc("/api/v1/projects/{projectID}/design/assets/{assetID}/file", get(d.assetFile))
	mux.HandleFunc("/api/v1/projects/{projectID}/design/screens", get(d.screens))
	mux.HandleFunc("/api/v1/projects/{projectID}/design/screens/{screenID}", get(d.screen))
	mux.HandleFunc("/api/v1/projects/{projectID}/design/screens/{screenID}/preview", get(d.preview))
	mux.HandleFunc("/api/v1/projects/{projectID}/design/flows/{flowID}", get(d.flow))
}

func (d *designRoutes) design(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	out, err := d.svc.Design(r.Context(), user.ID, r.PathValue("projectID"))
	if err != nil {
		d.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (d *designRoutes) assets(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	out, err := d.svc.Assets(r.Context(), user.ID, r.PathValue("projectID"))
	if err != nil {
		d.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (d *designRoutes) assetFile(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("assetID")
	if !assetIDPattern.MatchString(id) {
		failure(w, r, http.StatusNotFound, "ASSET_NOT_FOUND", "The asset does not exist.")
		return
	}
	user, _ := CurrentUser(r.Context())
	a, err := d.svc.Asset(r.Context(), user.ID, r.PathValue("projectID"), id)
	if err != nil {
		d.fail(w, r, err)
		return
	}
	defer a.Close()

	h := w.Header()
	h.Set("ETag", a.ETag)
	h.Set("Vary", "Cookie")
	h.Set("Cache-Control", previewCaching(r, a.DesignVersion))
	if matchesETag(r.Header.Get("If-None-Match"), a.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", a.MediaType)
	h.Set("Content-Length", strconv.FormatInt(a.Size, 10))
	h.Set("Content-Disposition", "inline")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, a.File)
}

func (d *designRoutes) tokens(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	out, err := d.svc.Tokens(r.Context(), user.ID, r.PathValue("projectID"))
	if err != nil {
		d.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (d *designRoutes) screens(w http.ResponseWriter, r *http.Request) {
	q, ok := parseQuery(r)
	if !ok {
		failure(w, r, http.StatusBadRequest, "INVALID_REQUEST", "The limit, offset, search or flow is not valid.")
		return
	}
	user, _ := CurrentUser(r.Context())
	page, err := d.svc.Screens(r.Context(), user.ID, r.PathValue("projectID"), q)
	if err != nil {
		d.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": page.Screens, "total": page.Total, "limit": page.Limit, "offset": page.Offset,
	})
}

func parseQuery(r *http.Request) (designapi.Query, bool) {
	v := r.URL.Query()
	q := designapi.Query{Search: v.Get("q"), FlowID: v.Get("flow")}
	if q.FlowID != "" && !flowIDPattern.MatchString(q.FlowID) {
		return q, false
	}
	for name, dst := range map[string]*int{"limit": &q.Limit, "offset": &q.Offset} {
		if raw := v.Get(name); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 || (name == "limit" && n == 0) {
				return q, false
			}
			*dst = n
		}
	}
	return q, true
}

func (d *designRoutes) screen(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("screenID")
	if !screenIDPattern.MatchString(id) {
		failure(w, r, http.StatusNotFound, "SCREEN_NOT_FOUND", "The screen does not exist.")
		return
	}
	user, _ := CurrentUser(r.Context())
	out, err := d.svc.Screen(r.Context(), user.ID, r.PathValue("projectID"), id)
	if err != nil {
		d.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

func (d *designRoutes) flow(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("flowID")
	if !flowIDPattern.MatchString(id) {
		failure(w, r, http.StatusNotFound, "FLOW_NOT_FOUND", "The flow does not exist.")
		return
	}
	user, _ := CurrentUser(r.Context())
	out, err := d.svc.Flow(r.Context(), user.ID, r.PathValue("projectID"), id)
	if err != nil {
		d.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": out})
}

// preview streams the stored reference image. The response is private: previews are proprietary.
func (d *designRoutes) preview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("screenID")
	if !screenIDPattern.MatchString(id) {
		failure(w, r, http.StatusNotFound, "SCREEN_NOT_FOUND", "The screen does not exist.")
		return
	}
	user, _ := CurrentUser(r.Context())
	p, err := d.svc.Preview(r.Context(), user.ID, r.PathValue("projectID"), id)
	if err != nil {
		d.fail(w, r, err)
		return
	}
	defer p.Close()

	h := w.Header()
	h.Set("ETag", p.ETag)
	h.Set("Vary", "Cookie")
	h.Set("Cache-Control", previewCaching(r, p.DesignVersion))
	if matchesETag(r.Header.Get("If-None-Match"), p.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set("Content-Type", p.MediaType)
	h.Set("Content-Length", strconv.FormatInt(p.Size, 10))
	h.Set("Content-Disposition", "inline")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, p.File)
}

// previewCaching keeps previews private, and long-lived only when the URL carries the current design version.
func previewCaching(r *http.Request, version string) string {
	if v := r.URL.Query().Get("v"); v != "" && v == version {
		return "private, max-age=86400, immutable"
	}
	return "private, no-cache"
}

// matchesETag implements If-None-Match with weak comparison.
func matchesETag(header, etag string) bool {
	for _, candidate := range strings.Split(header, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}

// fail maps read-layer errors; a foreign project looks exactly like a missing one.
func (d *designRoutes) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, designapi.ErrProjectNotFound):
		failure(w, r, http.StatusNotFound, "PROJECT_NOT_FOUND", "The project does not exist.")
	case errors.Is(err, designapi.ErrDesignNotFound):
		failure(w, r, http.StatusNotFound, "DESIGN_NOT_FOUND", "This project has no imported design yet.")
	case errors.Is(err, designapi.ErrNotReady):
		failure(w, r, http.StatusConflict, "DESIGN_NOT_READY", "The design is not ready yet.")
	case errors.Is(err, designapi.ErrExpired):
		failure(w, r, http.StatusGone, "DESIGN_DATA_EXPIRED", "The design data has expired. Import the design again.")
	case errors.Is(err, designapi.ErrScreenNotFound):
		failure(w, r, http.StatusNotFound, "SCREEN_NOT_FOUND", "The screen does not exist.")
	case errors.Is(err, designapi.ErrFlowNotFound):
		failure(w, r, http.StatusNotFound, "FLOW_NOT_FOUND", "The flow does not exist.")
	case errors.Is(err, designapi.ErrAssetNotFound):
		failure(w, r, http.StatusNotFound, "ASSET_NOT_FOUND", "The asset does not exist.")
	case errors.Is(err, designapi.ErrPreviewUnavailable):
		failure(w, r, http.StatusNotFound, "PREVIEW_NOT_AVAILABLE", "No preview is available for this screen.")
	case errors.Is(err, designapi.ErrInvalidQuery):
		failure(w, r, http.StatusBadRequest, "INVALID_REQUEST", "The limit, offset, search or flow is not valid.")
	case errors.Is(err, designapi.ErrInvalidDesign):
		d.log.ErrorContext(r.Context(), "design.invalid", slog.String("request_id", middleware.RequestID(r.Context())), slog.String("code", "DESIGN_IR_INVALID"))
		failure(w, r, http.StatusInternalServerError, "DESIGN_IR_INVALID", "The stored design could not be read. Import the design again.")
	default:
		unexpected(w, r, d.log, "design.failed", err)
	}
}
