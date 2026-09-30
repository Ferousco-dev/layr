// Package pgstore holds the import SQL; every state change is guarded by the status it expects.
package pgstore

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/ferousco-dev/layr/server/internal/imports"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// cols lists the selected columns; prefix qualifies them when the query joins another table.
func cols(prefix string) string {
	c := func(expr string) string { return strings.ReplaceAll(expr, "@", prefix) }
	return c(`@id, @project_id, @figma_file_key, COALESCE(@figma_node_id, ''), COALESCE(@figma_node_name, ''),
COALESCE(@figma_file_name, ''), COALESCE(@figma_version, ''), @status, COALESCE(@error_code, ''), @candidates,
COALESCE(@render_format, ''), COALESCE(@render_scale, 0)::float8, COALESCE(@asset_count, 0), COALESCE(@warning_count, 0),
@node_ids, COALESCE(@screen_count, 0), COALESCE(@design_node_count, 0), COALESCE(@design_warning_count, 0),
@created_at, @updated_at, @completed_at`)
}

var (
	columns   = cols("i.")
	returning = ` RETURNING ` + cols("")
)

// Create checks project ownership, retires abandoned imports, then inserts; the running index decides races.
func (s *Store) Create(ctx context.Context, ownerID, projectID, fileKey, nodeID string, now, staleBefore time.Time) (imports.Import, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return imports.Import{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var owned bool
	err = tx.QueryRow(ctx,
		`SELECT true FROM projects WHERE id = $1 AND user_id = $2 AND deleted_at IS NULL`, projectID, ownerID).Scan(&owned)
	if errors.Is(err, pgx.ErrNoRows) {
		return imports.Import{}, imports.ErrProjectNotFound
	}
	if err != nil {
		return imports.Import{}, err
	}

	_, err = tx.Exec(ctx, `
UPDATE figma_imports SET status = 'failed', error_code = $2, updated_at = $3
WHERE project_id = $1 AND (status = 'awaiting_selection' OR (status IN ('pending', 'processing') AND updated_at < $4))`,
		projectID, imports.CodeSuperseded, now.UTC(), staleBefore.UTC())
	if err != nil {
		return imports.Import{}, err
	}

	imp, err := scan(tx.QueryRow(ctx, `
INSERT INTO figma_imports (project_id, figma_file_key, figma_node_id, status, created_at, updated_at)
VALUES ($1, $2, NULLIF($3, ''), 'pending', $4, $4)`+returning, projectID, fileKey, nodeID, now.UTC()))
	if isUnique(err) {
		return imports.Import{}, imports.ErrConflict
	}
	if err != nil {
		return imports.Import{}, err
	}
	return imp, tx.Commit(ctx)
}

func (s *Store) Get(ctx context.Context, ownerID, projectID, importID string) (imports.Import, error) {
	return scan(s.pool.QueryRow(ctx, `
SELECT `+columns+` FROM figma_imports i JOIN projects p ON p.id = i.project_id
WHERE i.id = $1 AND i.project_id = $2 AND p.user_id = $3 AND p.deleted_at IS NULL`, importID, projectID, ownerID))
}

// LatestCompleted is the project's current design: its newest import that finished.
func (s *Store) LatestCompleted(ctx context.Context, ownerID, projectID string) (imports.Import, error) {
	return scan(s.pool.QueryRow(ctx, `
SELECT `+columns+` FROM figma_imports i JOIN projects p ON p.id = i.project_id
WHERE i.project_id = $1 AND p.user_id = $2 AND p.deleted_at IS NULL AND i.status = 'completed'
ORDER BY i.completed_at DESC, i.id DESC LIMIT 1`, projectID, ownerID))
}

func (s *Store) Latest(ctx context.Context, ownerID, projectID string) (imports.Import, error) {
	return scan(s.pool.QueryRow(ctx, `
SELECT `+columns+` FROM figma_imports i JOIN projects p ON p.id = i.project_id
WHERE i.project_id = $1 AND p.user_id = $2 AND p.deleted_at IS NULL
ORDER BY i.created_at DESC, i.id DESC LIMIT 1`, projectID, ownerID))
}

// BeginSelection moves awaiting_selection to processing for candidate screens, under a row lock.
func (s *Store) BeginSelection(ctx context.Context, ownerID, projectID, importID string, sel imports.Selection, maxScreens int, now time.Time) (imports.Import, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return imports.Import{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var status string
	var raw []byte
	err = tx.QueryRow(ctx, `
SELECT i.status, i.candidates FROM figma_imports i JOIN projects p ON p.id = i.project_id
WHERE i.id = $1 AND i.project_id = $2 AND p.user_id = $3 AND p.deleted_at IS NULL
FOR UPDATE OF i`, importID, projectID, ownerID).Scan(&status, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return imports.Import{}, imports.ErrNotFound
	}
	if err != nil {
		return imports.Import{}, err
	}
	if status != imports.StatusAwaitingSelection {
		return imports.Import{}, imports.ErrConflict
	}

	chosen, ok := choose(raw, sel, maxScreens)
	if !ok {
		return imports.Import{}, imports.ErrInvalidSelection
	}
	payload, err := json.Marshal(chosen)
	if err != nil {
		return imports.Import{}, err
	}

	imp, err := scan(tx.QueryRow(ctx, `
UPDATE figma_imports SET status = 'processing', figma_node_id = $2, node_ids = $3, updated_at = $4
WHERE id = $1`+` RETURNING `+cols(""), importID, chosen[0], payload, now.UTC()))
	if isUnique(err) {
		return imports.Import{}, imports.ErrConflict
	}
	if err != nil {
		return imports.Import{}, err
	}
	return imp, tx.Commit(ctx)
}

// choose returns the selected IDs in candidate order, or false if any is not a candidate.
func choose(raw []byte, sel imports.Selection, maxScreens int) ([]string, bool) {
	var candidates []imports.Frame
	if err := json.Unmarshal(raw, &candidates); err != nil || len(candidates) == 0 {
		return nil, false
	}
	known := map[string]bool{}
	for _, c := range candidates {
		known[c.ID] = true
	}
	want := map[string]bool{}
	for _, id := range sel.NodeIDs {
		if !known[id] || want[id] {
			return nil, false
		}
		want[id] = true
	}

	var out []string
	for _, c := range candidates {
		if sel.All || want[c.ID] {
			out = append(out, c.ID)
		}
	}
	if len(out) == 0 || len(out) > maxScreens || (!sel.All && len(out) != len(want)) {
		return nil, false
	}
	return out, true
}

func (s *Store) MarkProcessing(ctx context.Context, importID string, now time.Time) error {
	return s.guarded(ctx, `UPDATE figma_imports SET status = 'processing', updated_at = $2 WHERE id = $1 AND status = 'pending'`, importID, now.UTC())
}

func (s *Store) AwaitSelection(ctx context.Context, importID, fileName, version string, frames []imports.Frame, now time.Time) error {
	payload, err := json.Marshal(frames)
	if err != nil {
		return err
	}
	return s.guarded(ctx, `
UPDATE figma_imports SET status = 'awaiting_selection', candidates = $2, figma_file_name = $3,
       figma_version = NULLIF($4, ''), updated_at = $5
WHERE id = $1 AND status = 'processing'`, importID, payload, fileName, version, now.UTC())
}

func (s *Store) Complete(ctx context.Context, importID string, r imports.Result, now time.Time) error {
	return s.guarded(ctx, `
UPDATE figma_imports SET status = 'completed', figma_file_name = $2, figma_version = NULLIF($3, ''), figma_node_id = $4,
       figma_node_name = $5, render_format = $6, render_scale = $7, asset_count = $8, warning_count = $9,
       node_ids = $10, screen_count = $11, design_node_count = $12, design_warning_count = $13,
       candidates = NULL, error_code = NULL, updated_at = $14, completed_at = $14
WHERE id = $1 AND status = 'processing'`, importID, r.FileName, r.Version, r.NodeID, r.NodeName, r.RenderFormat, r.RenderScale,
		r.AssetCount, r.WarningCount, nodeIDsJSON(r.NodeIDs), r.ScreenCount, r.DesignNodes, r.DesignWarnings, now.UTC())
}

// Fail ends any unfinished import; a repeat or a late call on a finished import changes nothing.
func (s *Store) Fail(ctx context.Context, importID, code string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
UPDATE figma_imports SET status = 'failed', error_code = $2, updated_at = $3
WHERE id = $1 AND status IN ('pending', 'processing', 'awaiting_selection')`, importID, code, now.UTC())
	return err
}

func (s *Store) RunningIDs(ctx context.Context) (map[string]bool, error) {
	rows, err := s.pool.Query(ctx, `SELECT id FROM figma_imports WHERE status IN ('pending', 'processing')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// FailAbandoned fails imports whose process died: still pending or processing long after any live import could be.
func (s *Store) FailAbandoned(ctx context.Context, before time.Time, code string, now time.Time) (int, error) {
	tag, err := s.pool.Exec(ctx, `
UPDATE figma_imports SET status = 'failed', error_code = $1, updated_at = $2
WHERE status IN ('pending', 'processing') AND updated_at < $3`, code, now.UTC(), before.UTC())
	if err != nil {
		return 0, err
	}
	return int(tag.RowsAffected()), nil
}

func nodeIDsJSON(ids []string) []byte {
	if len(ids) == 0 {
		return nil
	}
	b, _ := json.Marshal(ids)
	return b
}

func (s *Store) guarded(ctx context.Context, sql string, args ...any) error {
	tag, err := s.pool.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return imports.ErrConflict
	}
	return nil
}

func scan(row pgx.Row) (imports.Import, error) {
	var imp imports.Import
	var candidates, nodeIDs []byte
	err := row.Scan(&imp.ID, &imp.ProjectID, &imp.FileKey, &imp.NodeID, &imp.NodeName, &imp.FileName, &imp.Version,
		&imp.Status, &imp.ErrorCode, &candidates, &imp.RenderFormat, &imp.RenderScale, &imp.AssetCount, &imp.WarningCount,
		&nodeIDs, &imp.ScreenCount, &imp.DesignNodes, &imp.DesignWarnings, &imp.CreatedAt, &imp.UpdatedAt, &imp.CompletedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return imports.Import{}, imports.ErrNotFound
	}
	if err != nil {
		return imports.Import{}, err
	}
	if len(candidates) > 0 {
		_ = json.Unmarshal(candidates, &imp.Candidates)
	}
	if len(nodeIDs) > 0 {
		_ = json.Unmarshal(nodeIDs, &imp.NodeIDs)
	}
	imp.CreatedAt, imp.UpdatedAt = imp.CreatedAt.UTC(), imp.UpdatedAt.UTC()
	if imp.CompletedAt != nil {
		t := imp.CompletedAt.UTC()
		imp.CompletedAt = &t
	}
	return imp, nil
}

func isUnique(err error) bool {
	var pg *pgconn.PgError
	return errors.As(err, &pg) && pg.Code == "23505"
}
