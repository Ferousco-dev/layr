// Package generation runs a code-generation job and records each step as it really happens.
package generation

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/ferousco-dev/layr/server/internal/account"
	"github.com/ferousco-dev/layr/server/internal/designapi"
)

// Job and step states.
const (
	StatusRunning   = "running"
	StatusCompleted = "completed"
	StatusFailed    = "failed"

	StepPending = "pending"
	StepRunning = "running"
	StepDone    = "done"
	StepFailed  = "failed"
)

// Failure codes stored on a job.
const (
	CodeNotAvailable   = "GENERATION_NOT_AVAILABLE"
	CodeDesignNotReady = "DESIGN_NOT_READY"
	CodeKeyMissing     = "KEY_REQUIRED"
	CodeInterrupted    = "GENERATION_INTERRUPTED"
	CodeInternal       = "INTERNAL_ERROR"
	CodeTimeout        = "GENERATION_TIMEOUT"
)

var (
	ErrNotFound         = errors.New("generation not found")
	ErrConflict         = errors.New("a generation is already running")
	ErrNoKey            = errors.New("no api key saved for that provider")
	ErrUnknownProvider  = errors.New("unknown provider")
	ErrDesignNotReady   = errors.New("design not ready")
	ErrInvalidSelection = errors.New("screen selection not valid")
	// ErrStageUnavailable marks a stage whose work does not exist yet.
	ErrStageUnavailable = errors.New("stage not available")
)

const (
	runTimeout     = 5 * time.Minute
	abandonedAfter = 10 * time.Minute
)

type Step struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	Status     string     `json:"status"`
	Detail     string     `json:"detail,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
}

type Generation struct {
	ID        string
	ProjectID string
	Provider  string
	// ScreenIDs are the screens chosen for this job; nil means every screen of the design.
	ScreenIDs   []string
	Status      string
	Steps       []Step
	ErrorCode   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	CompletedAt *time.Time
}

type Store interface {
	Create(ctx context.Context, ownerID, projectID, provider string, screenIDs []string, steps []Step, now time.Time) (Generation, error)
	Get(ctx context.Context, ownerID, projectID, id string) (Generation, error)
	SaveProgress(ctx context.Context, id string, steps []Step, now time.Time) error
	Finish(ctx context.Context, id, status, errorCode string, steps []Step, now time.Time) error
	FailAbandoned(ctx context.Context, before time.Time, code string, now time.Time) (int, error)
}

// Designs is the slice of the design API a job reads; jobs never touch Figma.
type Designs interface {
	Design(ctx context.Context, userID, projectID string) (*designapi.Design, error)
	Tokens(ctx context.Context, userID, projectID string) (*designapi.DesignTokens, error)
	ScreenIDs(ctx context.Context, userID, projectID string) ([]string, error)
}

// Keys opens a person's saved key on the server; the key never leaves the process.
type Keys interface {
	OpenKey(ctx context.Context, userID, provider string) (string, error)
}

// Job is what a stage can see and add to.
type Job struct {
	UserID    string
	ProjectID string
	Provider  string
	// ScreenIDs are the chosen screens; nil means all.
	ScreenIDs []string
	Design    *designapi.Design
	Tokens    *designapi.DesignTokens
}

// Stage is one real piece of work; it returns a short human detail when it succeeds.
type Stage struct {
	ID    string
	Label func(provider string) string
	Run   func(ctx context.Context, job *Job) (string, error)
}

type Service struct {
	store   Store
	designs Designs
	keys    Keys
	stages  []Stage
	log     *slog.Logger
	now     func() time.Time

	base   context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

func NewService(store Store, designs Designs, keys Keys, log *slog.Logger) *Service {
	return NewServiceWithStages(store, designs, keys, DefaultStages(designs, keys), log)
}

// NewServiceWithStages lets tests, and later milestones, supply their own stages.
func NewServiceWithStages(store Store, designs Designs, keys Keys, stages []Stage, log *slog.Logger) *Service {
	base, cancel := context.WithCancel(context.Background())
	return &Service{store: store, designs: designs, keys: keys, stages: stages, log: log, now: func() time.Time { return time.Now().UTC() }, base: base, cancel: cancel}
}

// Close stops running jobs and waits for them, up to the context's deadline.
func (s *Service) Close(ctx context.Context) {
	s.cancel()
	done := make(chan struct{})
	go func() { s.wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

// Sweep fails jobs whose process died, so a project is never stuck "generating".
func (s *Service) Sweep(ctx context.Context) {
	now := s.now()
	if n, err := s.store.FailAbandoned(ctx, now.Add(-abandonedAfter), CodeInterrupted, now); err == nil && n > 0 {
		s.log.WarnContext(ctx, "generation.abandoned_failed", slog.Int("count", n))
	}
}

// Start checks the provider, the saved key and the design, records the plan, and runs the job in the background.
func (s *Service) Start(ctx context.Context, userID, projectID, provider string, screenIDs []string) (Generation, error) {
	if !validProvider(provider) {
		return Generation{}, ErrUnknownProvider
	}
	if _, err := s.keys.OpenKey(ctx, userID, provider); err != nil {
		if errors.Is(err, account.ErrKeyNotFound) {
			return Generation{}, ErrNoKey
		}
		return Generation{}, err
	}
	design, err := s.designs.Design(ctx, userID, projectID)
	if err != nil {
		return Generation{}, err
	}
	if design.Status != designapi.StatusReady {
		return Generation{}, ErrDesignNotReady
	}

	if screenIDs != nil {
		var err error
		if screenIDs, err = s.checkSelection(ctx, userID, projectID, screenIDs); err != nil {
			return Generation{}, err
		}
	}

	steps := make([]Step, len(s.stages))
	for i, st := range s.stages {
		steps[i] = Step{ID: st.ID, Label: st.Label(provider), Status: StepPending}
	}
	gen, err := s.store.Create(ctx, userID, projectID, provider, screenIDs, steps, s.now())
	if err != nil {
		return Generation{}, err
	}

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.run(gen, &Job{UserID: userID, ProjectID: projectID, Provider: provider, ScreenIDs: screenIDs, Design: design})
	}()
	return gen, nil
}

// checkSelection accepts only screens that exist in the design, without repeats; choosing every screen means "all".
func (s *Service) checkSelection(ctx context.Context, userID, projectID string, ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, ErrInvalidSelection
	}
	all, err := s.designs.ScreenIDs(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	known := make(map[string]bool, len(all))
	for _, id := range all {
		known[id] = true
	}
	seen := make(map[string]bool, len(ids))
	chosen := make([]string, 0, len(ids))
	for _, id := range ids {
		if !known[id] {
			return nil, ErrInvalidSelection
		}
		if !seen[id] {
			seen[id] = true
			chosen = append(chosen, id)
		}
	}
	if len(chosen) == len(all) {
		return nil, nil
	}
	return chosen, nil
}

// Get returns one generation of a project the person owns.
func (s *Service) Get(ctx context.Context, userID, projectID, id string) (Generation, error) {
	return s.store.Get(ctx, userID, projectID, id)
}

func validProvider(p string) bool {
	for _, known := range account.Providers() {
		if p == known {
			return true
		}
	}
	return false
}

// run works through the stages, saving every change so a poll always sees what really happened.
func (s *Service) run(gen Generation, job *Job) {
	ctx, cancel := context.WithTimeout(s.base, runTimeout)
	defer cancel()
	// The job edits its own copy; the caller of Start keeps the original untouched.
	steps := append([]Step(nil), gen.Steps...)
	log := s.log.With(slog.String("generation_id", gen.ID), slog.String("provider", gen.Provider))

	finish := func(status, code string) {
		// The final write must survive shutdown, so it does not use the job's context.
		fin, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := s.store.Finish(fin, gen.ID, status, code, steps, s.now()); err != nil {
			log.Error("generation.finish_failed", slog.String("code", CodeInternal))
			return
		}
		log.Info("generation.finished", slog.String("status", status), slog.String("error_code", code))
	}
	defer func() {
		if r := recover(); r != nil {
			log.Error("generation.panic", slog.String("code", CodeInternal))
			finish(StatusFailed, CodeInternal)
		}
	}()

	for i, stage := range s.stages {
		started := s.now()
		steps[i].Status, steps[i].StartedAt = StepRunning, &started
		if err := s.store.SaveProgress(ctx, gen.ID, steps, started); err != nil {
			finish(StatusFailed, codeFor(ctx, err))
			return
		}

		detail, err := stage.Run(ctx, job)
		ended := s.now()
		steps[i].FinishedAt, steps[i].Detail = &ended, detail
		if err != nil {
			steps[i].Status = StepFailed
			finish(StatusFailed, codeFor(ctx, err))
			return
		}
		steps[i].Status = StepDone
		if err := s.store.SaveProgress(ctx, gen.ID, steps, ended); err != nil {
			finish(StatusFailed, codeFor(ctx, err))
			return
		}
	}
	finish(StatusCompleted, "")
}

// codeFor maps a stage error to a stable public code.
func codeFor(ctx context.Context, err error) string {
	switch {
	case errors.Is(err, ErrStageUnavailable):
		return CodeNotAvailable
	case errors.Is(err, ErrDesignNotReady), errors.Is(err, designapi.ErrNotReady), errors.Is(err, designapi.ErrExpired):
		return CodeDesignNotReady
	case errors.Is(err, account.ErrKeyNotFound):
		return CodeKeyMissing
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return CodeTimeout
	case errors.Is(ctx.Err(), context.Canceled):
		return CodeInterrupted
	}
	return CodeInternal
}
