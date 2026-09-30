package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ferousco-dev/layr/server/internal/httpapi/middleware"
	"github.com/ferousco-dev/layr/server/internal/project"
)

type ProjectService interface {
	Create(ctx context.Context, ownerID, name string) (project.Project, error)
	Get(ctx context.Context, ownerID, id string) (project.Project, error)
	List(ctx context.Context, ownerID string, limit int, cursor string) (project.Page, error)
	Rename(ctx context.Context, ownerID, id, name string) (project.Project, error)
	Delete(ctx context.Context, ownerID, id string) error
	Restore(ctx context.Context, ownerID, id string) (project.Project, error)
}

type projectRoutes struct {
	svc     ProjectService
	require func(http.HandlerFunc) http.HandlerFunc
	log     *slog.Logger
}

func (p *projectRoutes) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/projects", p.require(methods(map[string]http.HandlerFunc{
		http.MethodPost: p.create,
		http.MethodGet:  p.list,
	})))
	mux.HandleFunc("/api/v1/projects/{projectID}", p.require(methods(map[string]http.HandlerFunc{
		http.MethodGet:    p.get,
		http.MethodPatch:  p.rename,
		http.MethodDelete: p.remove,
	})))
	mux.HandleFunc("/api/v1/projects/{projectID}/restore", p.require(methods(map[string]http.HandlerFunc{
		http.MethodPost: p.restore,
	})))
}

// methods dispatches by HTTP method and answers 405 with an Allow header otherwise.
func methods(handlers map[string]http.HandlerFunc) http.HandlerFunc {
	allowed := make([]string, 0, len(handlers))
	for m := range handlers {
		allowed = append(allowed, m)
	}
	sort.Strings(allowed)

	return func(w http.ResponseWriter, r *http.Request) {
		h, ok := handlers[r.Method]
		if !ok {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			failure(w, r, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "This endpoint accepts "+strings.Join(allowed, ", ")+" requests only.")
			return
		}
		h(w, r)
	}
}

type nameBody struct {
	Name *string `json:"name"`
}

func (p *projectRoutes) create(w http.ResponseWriter, r *http.Request) {
	var body nameBody
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Name == nil {
		failure(w, r, http.StatusBadRequest, "INVALID_PROJECT_NAME", "A project name is required.")
		return
	}

	user, _ := CurrentUser(r.Context())
	created, err := p.svc.Create(r.Context(), user.ID, *body.Name)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	p.logged(r, "project.created", user.ID, created.ID)
	writeProject(w, http.StatusCreated, created)
}

func (p *projectRoutes) list(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n == 0 {
			failure(w, r, http.StatusBadRequest, "INVALID_REQUEST", "The limit must be a whole number from 1 to 100.")
			return
		}
		limit = n
	}

	user, _ := CurrentUser(r.Context())
	page, err := p.svc.List(r.Context(), user.ID, limit, r.URL.Query().Get("cursor"))
	if err != nil {
		p.fail(w, r, err)
		return
	}

	items := make([]map[string]any, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, projectJSON(item))
	}
	var next any
	if page.Next != "" {
		next = page.Next
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": items, "next_cursor": next})
}

func (p *projectRoutes) get(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	found, err := p.svc.Get(r.Context(), user.ID, r.PathValue("projectID"))
	if err != nil {
		p.fail(w, r, err)
		return
	}
	writeProject(w, http.StatusOK, found)
}

func (p *projectRoutes) rename(w http.ResponseWriter, r *http.Request) {
	var body nameBody
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Name == nil {
		failure(w, r, http.StatusBadRequest, "INVALID_REQUEST", "Provide at least one field to update.")
		return
	}

	user, _ := CurrentUser(r.Context())
	updated, err := p.svc.Rename(r.Context(), user.ID, r.PathValue("projectID"), *body.Name)
	if err != nil {
		p.fail(w, r, err)
		return
	}
	p.logged(r, "project.updated", user.ID, updated.ID)
	writeProject(w, http.StatusOK, updated)
}

func (p *projectRoutes) remove(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	id := r.PathValue("projectID")
	if err := p.svc.Delete(r.Context(), user.ID, id); err != nil {
		p.fail(w, r, err)
		return
	}
	p.logged(r, "project.deleted", user.ID, id)
	w.WriteHeader(http.StatusNoContent)
}

func (p *projectRoutes) restore(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	restored, err := p.svc.Restore(r.Context(), user.ID, r.PathValue("projectID"))
	if err != nil {
		p.fail(w, r, err)
		return
	}
	p.logged(r, "project.restored", user.ID, restored.ID)
	writeProject(w, http.StatusOK, restored)
}

// fail maps domain errors to stable codes; a foreign project is indistinguishable from a missing one.
func (p *projectRoutes) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, project.ErrInvalidID):
		failure(w, r, http.StatusBadRequest, "INVALID_PROJECT_ID", "The project ID is not valid.")
	case errors.Is(err, project.ErrInvalidName):
		failure(w, r, http.StatusBadRequest, "INVALID_PROJECT_NAME", "The name must be 1 to 120 characters without control characters.")
	case errors.Is(err, project.ErrInvalidPage):
		failure(w, r, http.StatusBadRequest, "INVALID_REQUEST", "The limit or cursor is not valid.")
	case errors.Is(err, project.ErrNotFound):
		failure(w, r, http.StatusNotFound, "PROJECT_NOT_FOUND", "The project does not exist.")
	default:
		unexpected(w, r, p.log, "project.failed", err)
	}
}

func (p *projectRoutes) logged(r *http.Request, operation, userID, projectID string) {
	p.log.InfoContext(r.Context(), operation,
		slog.String("request_id", middleware.RequestID(r.Context())),
		slog.String("user_id", userID),
		slog.String("project_id", projectID))
}

// decodeBody accepts exactly one JSON object with known fields and writes the error itself on failure.
func decodeBody(w http.ResponseWriter, r *http.Request, dst any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	err := dec.Decode(dst)
	if err == nil {
		if _, extra := dec.Token(); !errors.Is(extra, io.EOF) {
			err = errors.New("trailing data")
		}
	}
	if err == nil {
		return true
	}

	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		failure(w, r, http.StatusRequestEntityTooLarge, "REQUEST_TOO_LARGE", "The request body is too large.")
		return false
	}
	failure(w, r, http.StatusBadRequest, "INVALID_REQUEST", "The request body must be one JSON object with known fields.")
	return false
}

func projectJSON(p project.Project) map[string]any {
	return map[string]any{
		"id":         p.ID,
		"name":       p.Name,
		"created_at": p.CreatedAt.UTC().Format(time.RFC3339Nano),
		"updated_at": p.UpdatedAt.UTC().Format(time.RFC3339Nano),
	}
}

func writeProject(w http.ResponseWriter, status int, p project.Project) {
	writeJSON(w, status, map[string]any{"data": projectJSON(p)})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
