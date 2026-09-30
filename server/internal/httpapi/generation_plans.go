package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/ferousco-dev/layr/server/internal/genplan"
	"github.com/ferousco-dev/layr/server/internal/project"
)

type GenerationPlanService interface {
	Create(ctx context.Context, userID, projectID string, req genplan.Request) (genplan.Record, error)
	Get(ctx context.Context, userID, projectID, planID string) (genplan.Record, error)
}

type generationPlanRoutes struct {
	svc     GenerationPlanService
	require func(http.HandlerFunc) http.HandlerFunc
	log     *slog.Logger
}

// planFailures maps planner codes to what a person sees; none of them repeat design content.
var planFailures = map[string]struct {
	status  int
	message string
}{
	genplan.CodeInvalidSelection:     {http.StatusBadRequest, "That selection is not valid. Choose one screen, several screens, a flow or all screens."},
	genplan.CodeEmptySelection:       {http.StatusBadRequest, "Choose at least one screen."},
	genplan.CodeInvalidDesignVersion: {http.StatusBadRequest, "The design version is not valid."},
	genplan.CodeTargetUnsupported:    {http.StatusBadRequest, "Only Next.js with TypeScript is supported for now."},
	genplan.CodeScreenNotFound:       {http.StatusNotFound, "One of the chosen screens is not in this design."},
	genplan.CodeFlowNotFound:         {http.StatusNotFound, "That flow is not in this design."},
	genplan.CodeProjectNotFound:      {http.StatusNotFound, "The project does not exist."},
	genplan.CodePlanNotFound:         {http.StatusNotFound, "That generation plan does not exist."},
	genplan.CodeDesignNotReady:       {http.StatusConflict, "This project has no finished design. Import one first."},
	genplan.CodeVersionUnavailable:   {http.StatusConflict, "This version of the design is no longer available. Reload the design and choose again."},
	genplan.CodeDependencyInvalid:    {http.StatusUnprocessableEntity, "The design has a broken reference, so it cannot be planned."},
	genplan.CodeDependencyCycle:      {http.StatusUnprocessableEntity, "The design has components that depend on each other, so it cannot be planned."},
	genplan.CodePlanInvalid:          {http.StatusUnprocessableEntity, "The design could not be planned."},
	genplan.CodePlanTooLarge:         {http.StatusUnprocessableEntity, "This selection is too large to plan. Choose fewer screens."},
}

// maxPlanRequestScreens matches the planner's own bound on a selection list.
const maxPlanRequestScreens = 5000

func (g *generationPlanRoutes) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/projects/{projectID}/generation-plans", g.require(methods(map[string]http.HandlerFunc{
		http.MethodPost: g.create,
	})))
	mux.HandleFunc("/api/v1/projects/{projectID}/generation-plans/{planID}", g.require(methods(map[string]http.HandlerFunc{
		http.MethodGet: g.get,
	})))
}

func planJSON(rec genplan.Record) map[string]any {
	p := rec.Plan
	return map[string]any{
		"id": p.ID, "project_id": p.ProjectID, "design_version": p.DesignVersion, "status": rec.Status, "schema_version": p.SchemaVersion,
		"fingerprint": p.Fingerprint, "selection": p.Selection, "target": p.Target, "summary": p.Summary,
		"units": p.Units, "dependencies": p.Dependencies, "order": p.Order, "stages": p.Stages, "assets": p.Assets, "warnings": p.Warnings,
		"created_at": rec.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func (g *generationPlanRoutes) create(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	projectID, err := project.NormalizeID(r.PathValue("projectID"))
	if err != nil {
		failure(w, r, http.StatusBadRequest, "INVALID_PROJECT_ID", "The project ID is not valid.")
		return
	}
	// Only the choice is accepted: unknown fields such as units, dependencies or user_id are rejected by decodeBody.
	var body struct {
		DesignVersion string `json:"design_version"`
		Selection     *struct {
			Mode      string    `json:"mode"`
			ScreenID  string    `json:"screen_id"`
			ScreenIDs *[]string `json:"screen_ids"`
			FlowID    string    `json:"flow_id"`
		} `json:"selection"`
		Target *genplan.Target `json:"target"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Selection == nil {
		g.fail(w, r, &genplan.Error{Code: genplan.CodeInvalidSelection})
		return
	}
	req := genplan.Request{DesignVersion: body.DesignVersion, Selection: genplan.Selection{Mode: body.Selection.Mode, ScreenID: body.Selection.ScreenID, FlowID: body.Selection.FlowID}}
	if body.Selection.ScreenIDs != nil {
		if len(*body.Selection.ScreenIDs) > maxPlanRequestScreens {
			g.fail(w, r, &genplan.Error{Code: genplan.CodeInvalidSelection})
			return
		}
		req.Selection.ScreenIDs = append([]string{}, *body.Selection.ScreenIDs...)
	}
	if body.Target != nil {
		req.Target = *body.Target
	}
	rec, err := g.svc.Create(r.Context(), user.ID, projectID, req)
	if err != nil {
		g.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"data": planJSON(rec)})
}

func (g *generationPlanRoutes) get(w http.ResponseWriter, r *http.Request) {
	user, _ := CurrentUser(r.Context())
	projectID, err := project.NormalizeID(r.PathValue("projectID"))
	if err != nil {
		failure(w, r, http.StatusBadRequest, "INVALID_PROJECT_ID", "The project ID is not valid.")
		return
	}
	id := r.PathValue("planID")
	if !generationIDPattern.MatchString(id) {
		g.fail(w, r, &genplan.Error{Code: genplan.CodePlanNotFound})
		return
	}
	rec, err := g.svc.Get(r.Context(), user.ID, projectID, id)
	if err != nil {
		g.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": planJSON(rec)})
}

func (g *generationPlanRoutes) fail(w http.ResponseWriter, r *http.Request, err error) {
	if f, ok := planFailures[genplan.CodeOf(err)]; ok {
		failure(w, r, f.status, genplan.CodeOf(err), f.message)
		return
	}
	unexpected(w, r, g.log, "generation_plan.request_failed", err)
}
