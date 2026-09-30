// Package pgstore persists generation plans with explicit SQL.
package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/ferousco-dev/layr/server/internal/genplan"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Create checks ownership and inserts the plan; a project that is not the owner's has no row to select.
func (s *Store) Create(ctx context.Context, ownerID, projectID, importID string, plan *genplan.Plan, raw []byte, now time.Time) (genplan.Record, error) {
	var id string
	err := s.pool.QueryRow(ctx, `
INSERT INTO generation_plans (project_id, import_id, design_version, fingerprint, selection_mode, target_framework, target_language, status, schema_version, plan, created_at, updated_at)
SELECT p.id, $3, $4, $5, $6, $7, $8, 'planned', $9, $10, $11, $11 FROM projects p
WHERE p.id = $1 AND p.user_id = $2 AND p.deleted_at IS NULL
RETURNING id`,
		projectID, ownerID, importID, plan.DesignVersion, plan.Fingerprint, plan.Selection.Mode,
		plan.Target.Framework, plan.Target.Language, plan.SchemaVersion, raw, now.UTC()).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return genplan.Record{}, genplan.ErrNotFound
	}
	if err != nil {
		return genplan.Record{}, err
	}
	stored := *plan
	stored.ID, stored.ProjectID = id, projectID
	return genplan.Record{Plan: &stored, Status: genplan.StatusPlanned, ImportID: importID, CreatedAt: now.UTC()}, nil
}

// Get returns a plan only when its project belongs to the owner.
func (s *Store) Get(ctx context.Context, ownerID, projectID, id string) (genplan.Record, error) {
	var raw []byte
	rec := genplan.Record{}
	var planID, project string
	err := s.pool.QueryRow(ctx, `
SELECT gp.id, gp.project_id, gp.import_id, gp.status, gp.plan, gp.created_at
FROM generation_plans gp JOIN projects p ON p.id = gp.project_id
WHERE gp.id = $3 AND gp.project_id = $1 AND p.user_id = $2 AND p.deleted_at IS NULL`,
		projectID, ownerID, id).Scan(&planID, &project, &rec.ImportID, &rec.Status, &raw, &rec.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return genplan.Record{}, genplan.ErrNotFound
	}
	if err != nil {
		return genplan.Record{}, err
	}
	var plan genplan.Plan
	if err := json.Unmarshal(raw, &plan); err != nil {
		return genplan.Record{}, err
	}
	plan.ID, plan.ProjectID = planID, project
	rec.Plan = &plan
	return rec, nil
}
