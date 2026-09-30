package genplan

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"time"

	"github.com/ferousco-dev/layr/server/internal/designapi"
	"github.com/ferousco-dev/layr/server/internal/project"
)

var versionPattern = regexp.MustCompile(`^dv_[0-9a-f]{16}$`)

// Designs pins a project's design to an exact version; designapi.Service satisfies it.
type Designs interface {
	Pin(ctx context.Context, userID, projectID, version string) (*designapi.Pinned, error)
}

// Record is a stored plan.
type Record struct {
	Plan      *Plan
	Status    string
	ImportID  string
	CreatedAt time.Time
}

// Store persists immutable plans. Every call is scoped to the owner.
type Store interface {
	Create(ctx context.Context, ownerID, projectID, importID string, plan *Plan, raw []byte, now time.Time) (Record, error)
	Get(ctx context.Context, ownerID, projectID, id string) (Record, error)
}

type Service struct {
	designs Designs
	store   Store
	lim     Limits
	log     *slog.Logger
	now     func() time.Time
}

func NewService(designs Designs, store Store, lim Limits, log *slog.Logger) *Service {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return &Service{designs: designs, store: store, lim: lim.withDefaults(), log: log, now: func() time.Time { return time.Now().UTC() }}
}

// Create plans a selection of the owner's design and stores the immutable result.
// The owner comes from the session and the dependencies from the design; the caller supplies neither.
func (s *Service) Create(ctx context.Context, userID, projectID string, req Request) (Record, error) {
	id, err := project.NormalizeID(projectID)
	if err != nil {
		return Record{}, fail(CodeProjectNotFound, "")
	}
	if !versionPattern.MatchString(req.DesignVersion) {
		return Record{}, fail(CodeInvalidDesignVersion, "design version is not valid")
	}
	pin, err := s.designs.Pin(ctx, userID, id, req.DesignVersion)
	if err != nil {
		return Record{}, mapDesignError(err)
	}
	if err := ctx.Err(); err != nil {
		return Record{}, err
	}
	start := time.Now()
	plan, err := Build(pin.Design, req, s.lim)
	if err != nil {
		s.log.WarnContext(ctx, "generation_plan.rejected", slog.String("user_id", userID), slog.String("project_id", id),
			slog.String("design_version", req.DesignVersion), slog.String("selection_mode", req.Selection.Mode), slog.String("code", CodeOf(err)))
		return Record{}, err
	}
	raw, err := json.Marshal(plan)
	if err != nil {
		return Record{}, fail(CodePlanInvalid, "the plan could not be encoded")
	}
	if len(raw) > s.lim.MaxPlanBytes {
		return Record{}, fail(CodePlanTooLarge, "the plan is larger than allowed")
	}
	rec, err := s.store.Create(ctx, userID, id, pin.ImportID, plan, raw, s.now())
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Record{}, fail(CodeProjectNotFound, "")
		}
		return Record{}, err
	}
	s.log.InfoContext(ctx, "generation_plan.created",
		slog.String("user_id", userID), slog.String("project_id", id), slog.String("design_version", plan.DesignVersion),
		slog.String("plan_id", rec.Plan.ID), slog.String("selection_mode", plan.Selection.Mode),
		slog.Int("screen_count", plan.Summary.Screens), slog.Int("component_count", plan.Summary.SharedComponents),
		slog.Int("asset_count", plan.Summary.Assets), slog.Int("unit_count", plan.Summary.Units),
		slog.Int("dependency_count", plan.Summary.Dependencies), slog.Int64("planning_duration_ms", time.Since(start).Milliseconds()))
	return rec, nil
}

// Get returns one of the owner's plans, checked again before it is handed out.
func (s *Service) Get(ctx context.Context, userID, projectID, planID string) (Record, error) {
	id, err := project.NormalizeID(projectID)
	if err != nil {
		return Record{}, fail(CodeProjectNotFound, "")
	}
	rec, err := s.store.Get(ctx, userID, id, planID)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return Record{}, fail(CodePlanNotFound, "")
		}
		return Record{}, err
	}
	if err := Validate(rec.Plan, nil); err != nil {
		s.log.ErrorContext(ctx, "generation_plan.stored_invalid", slog.String("plan_id", planID), slog.String("code", CodeOf(err)))
		return Record{}, fail(CodePlanInvalid, "")
	}
	return rec, nil
}

func mapDesignError(err error) error {
	switch {
	case errors.Is(err, designapi.ErrProjectNotFound), errors.Is(err, project.ErrNotFound):
		return fail(CodeProjectNotFound, "")
	case errors.Is(err, designapi.ErrNotReady), errors.Is(err, designapi.ErrDesignNotFound):
		return fail(CodeDesignNotReady, "")
	case errors.Is(err, designapi.ErrVersionUnavailable), errors.Is(err, designapi.ErrExpired):
		return fail(CodeVersionUnavailable, "")
	}
	return err
}
