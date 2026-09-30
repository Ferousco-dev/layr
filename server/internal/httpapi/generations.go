package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"time"

	"github.com/ferousco-dev/layr/server/internal/designapi"
	"github.com/ferousco-dev/layr/server/internal/generation"
	"github.com/ferousco-dev/layr/server/internal/project"
)

type GenerationService interface {
	Start(ctx context.Context, userID, projectID, provider string, screenIDs []string) (generation.Generation, error)
	Get(ctx context.Context, userID, projectID, id string) (generation.Generation, error)
}

type generationRoutes struct {
	svc     GenerationService
	require func(http.HandlerFunc) http.HandlerFunc
	budget  func(http.HandlerFunc) http.HandlerFunc
	log     *slog.Logger
}

// maxSelectedScreens bounds one request; a design cannot have more screens than the import limit anyway.
const maxSelectedScreens = 500

var generationIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// generationMessages are the only texts a person sees for a failed job.
var generationMessages = map[string]string{
	generation.CodeNotAvailable:   "Writing the code is not available yet. Your design and key are ready for when it is.",
	generation.CodeDesignNotReady: "This project has no finished design. Import one first.",
	generation.CodeKeyMissing:     "The saved key for this provider could not be found. Add it again.",
	generation.CodeInterrupted:    "The server stopped while this was running. Start it again.",
	generation.CodeTimeout:        "This took too long and was stopped. Try again.",
	generation.CodeInternal:       "Something went wrong. Try again.",
}

func (g *generationRoutes) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/projects/{projectID}/generations", g.require(methods(map[string]http.HandlerFunc{
		http.MethodPost: g.budget(g.start),
	})))
	mux.HandleFunc("/api/v1/projects/{projectID}/generations/{generationID}", g.require(methods(map[string]http.HandlerFunc{
		http.MethodGet: g.get,
	})))
}

func generationJSON(gen generation.Generation) map[string]any {
	steps := make([]map[string]any, 0, len(gen.Steps))
	done := 0
	for _, s := range gen.Steps {
		if s.Status == generation.StepDone {
			done++
		}
		steps = append(steps, map[string]any{"id": s.ID, "label": s.Label, "status": s.Status, "detail": s.Detail})
	}
	var apiErr any
	if gen.ErrorCode != "" {
		apiErr = map[string]string{"code": gen.ErrorCode, "message": generationMessages[gen.ErrorCode]}
	}
	var completed any
	if gen.CompletedAt != nil {
		completed = gen.CompletedAt.UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"id": gen.ID, "project_id": gen.ProjectID, "provider": gen.Provider, "provider_label": generation.ProviderLabel(gen.Provider),
		"status": gen.Status, "screen_ids": screenIDs(gen), "steps": steps, "progress": map[string]int{"done": done, "total": len(gen.Steps)}, "error": apiErr,
		"created_at": gen.CreatedAt.UTC().Format(time.RFC3339), "updated_at": gen.UpdatedAt.UTC().Format(time.RFC3339), "completed_at": completed,
	}
}

func (g *generationRoutes) start(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	projectID, err := project.NormalizeID(r.PathValue("projectID"))
	if err != nil {
		failure(w, r, http.StatusBadRequest, "INVALID_PROJECT_ID", "The project ID is not valid.")
		return
	}
	var body struct {
		Provider  *string   `json:"provider"`
		ScreenIDs *[]string `json:"screen_ids"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Provider == nil {
		failure(w, r, http.StatusBadRequest, "INVALID_PROVIDER", "Choose an AI provider.")
		return
	}
	var screenIDs []string
	if body.ScreenIDs != nil {
		screenIDs = *body.ScreenIDs
		if len(screenIDs) == 0 || len(screenIDs) > maxSelectedScreens {
			failure(w, r, http.StatusBadRequest, "INVALID_SELECTION", "Choose at least one screen.")
			return
		}
		for _, id := range screenIDs {
			if !screenIDPattern.MatchString(id) {
				failure(w, r, http.StatusBadRequest, "INVALID_SELECTION", "Choose at least one screen.")
				return
			}
		}
	}
	gen, err := g.svc.Start(r.Context(), user.ID, projectID, *body.Provider, screenIDs)
	if err != nil {
		g.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"data": generationJSON(gen)})
}

func (g *generationRoutes) get(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	projectID, err := project.NormalizeID(r.PathValue("projectID"))
	if err != nil {
		failure(w, r, http.StatusBadRequest, "INVALID_PROJECT_ID", "The project ID is not valid.")
		return
	}
	id := r.PathValue("generationID")
	if !generationIDPattern.MatchString(id) {
		failure(w, r, http.StatusNotFound, "GENERATION_NOT_FOUND", "That generation does not exist.")
		return
	}
	gen, err := g.svc.Get(r.Context(), user.ID, projectID, id)
	if err != nil {
		g.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": generationJSON(gen)})
}

func (g *generationRoutes) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, generation.ErrUnknownProvider):
		failure(w, r, http.StatusBadRequest, "INVALID_PROVIDER", "That AI provider is not supported.")
	case errors.Is(err, generation.ErrInvalidSelection):
		failure(w, r, http.StatusBadRequest, "INVALID_SELECTION", "Choose at least one screen that exists in this design.")
	case errors.Is(err, generation.ErrNoKey):
		failure(w, r, http.StatusBadRequest, "KEY_REQUIRED", "Add an API key for that provider first.")
	case errors.Is(err, generation.ErrDesignNotReady), errors.Is(err, designapi.ErrNotReady), errors.Is(err, designapi.ErrDesignNotFound), errors.Is(err, designapi.ErrExpired):
		failure(w, r, http.StatusConflict, "DESIGN_NOT_READY", generationMessages[generation.CodeDesignNotReady])
	case errors.Is(err, generation.ErrConflict):
		failure(w, r, http.StatusConflict, "GENERATION_RUNNING", "A generation is already running for this project.")
	case errors.Is(err, designapi.ErrProjectNotFound), errors.Is(err, project.ErrNotFound):
		failure(w, r, http.StatusNotFound, "PROJECT_NOT_FOUND", "The project does not exist.")
	case errors.Is(err, generation.ErrNotFound):
		failure(w, r, http.StatusNotFound, "GENERATION_NOT_FOUND", "That generation does not exist.")
	default:
		unexpected(w, r, g.log, "generation.request_failed", err)
	}
}

// screenIDs reports the chosen screens, or null when the job covers every screen.
func screenIDs(gen generation.Generation) any {
	if gen.ScreenIDs == nil {
		return nil
	}
	return gen.ScreenIDs
}
