// Package project owns the Project domain: naming rules, ownership-scoped operations and pagination.
package project

import (
	"context"
	"encoding/base64"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	MaxNameRunes = 120
	DefaultLimit = 20
	MaxLimit     = 100
	// Retention is how long a deleted project can still be restored.
	Retention = 30 * 24 * time.Hour
)

var (
	ErrNotFound    = errors.New("project not found")
	ErrInvalidID   = errors.New("project id invalid")
	ErrInvalidName = errors.New("project name invalid")
	ErrInvalidPage = errors.New("pagination invalid")
)

type Project struct {
	ID        string
	UserID    string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Cursor struct {
	UpdatedAt time.Time
	ID        string
}

type Page struct {
	Items []Project
	Next  string
}

// Repository methods always take the owner so no query can reach another user's row.
type Repository interface {
	Create(ctx context.Context, ownerID, name string, now time.Time) (Project, error)
	Get(ctx context.Context, ownerID, id string) (Project, error)
	List(ctx context.Context, ownerID string, after *Cursor, limit int) ([]Project, error)
	Rename(ctx context.Context, ownerID, id, name string, now time.Time) (Project, error)
	Delete(ctx context.Context, ownerID, id string, now time.Time) error
	Restore(ctx context.Context, ownerID, id string) (Project, error)
	PurgeExpired(ctx context.Context, before time.Time) error
}

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}

func (s *Service) Create(ctx context.Context, ownerID, name string) (Project, error) {
	clean, err := ValidateName(name)
	if err != nil {
		return Project{}, err
	}
	return s.repo.Create(ctx, ownerID, clean, s.now())
}

func (s *Service) Get(ctx context.Context, ownerID, id string) (Project, error) {
	id, err := NormalizeID(id)
	if err != nil {
		return Project{}, err
	}
	return s.repo.Get(ctx, ownerID, id)
}

// List returns one page, fetching an extra row to learn whether another page exists.
func (s *Service) List(ctx context.Context, ownerID string, limit int, cursor string) (Page, error) {
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		return Page{}, ErrInvalidPage
	}
	after, err := decodeCursor(cursor)
	if err != nil {
		return Page{}, err
	}

	rows, err := s.repo.List(ctx, ownerID, after, limit+1)
	if err != nil {
		return Page{}, err
	}
	if len(rows) <= limit {
		return Page{Items: rows}, nil
	}
	rows = rows[:limit]
	return Page{Items: rows, Next: encodeCursor(rows[limit-1])}, nil
}

func (s *Service) Rename(ctx context.Context, ownerID, id, name string) (Project, error) {
	id, err := NormalizeID(id)
	if err != nil {
		return Project{}, err
	}
	clean, err := ValidateName(name)
	if err != nil {
		return Project{}, err
	}
	return s.repo.Rename(ctx, ownerID, id, clean, s.now())
}

// Delete hides the project for Retention, then lazily purges anything past that window.
func (s *Service) Delete(ctx context.Context, ownerID, id string) error {
	id, err := NormalizeID(id)
	if err != nil {
		return err
	}
	now := s.now()
	if err := s.repo.Delete(ctx, ownerID, id, now); err != nil {
		return err
	}
	_ = s.repo.PurgeExpired(ctx, now.Add(-Retention))
	return nil
}

func (s *Service) Restore(ctx context.Context, ownerID, id string) (Project, error) {
	id, err := NormalizeID(id)
	if err != nil {
		return Project{}, err
	}
	return s.repo.Restore(ctx, ownerID, id)
}

// ValidateName trims the name and rejects empty, oversized, non-UTF-8 or control-character names.
func ValidateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !utf8.ValidString(name) {
		return "", ErrInvalidName
	}
	count := utf8.RuneCountInString(name)
	if count < 1 || count > MaxNameRunes {
		return "", ErrInvalidName
	}
	for _, r := range name {
		if unicode.IsControl(r) {
			return "", ErrInvalidName
		}
	}
	return name, nil
}

// NormalizeID accepts only canonical hyphenated UUIDs and lower-cases them.
func NormalizeID(id string) (string, error) {
	if len(id) != 36 {
		return "", ErrInvalidID
	}
	for i, c := range id {
		dash := i == 8 || i == 13 || i == 18 || i == 23
		hex := c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F'
		if (dash && c != '-') || (!dash && !hex) {
			return "", ErrInvalidID
		}
	}
	return strings.ToLower(id), nil
}

func encodeCursor(p Project) string {
	raw := strconv.FormatInt(p.UpdatedAt.UnixMicro(), 10) + "." + p.ID
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(cursor string) (*Cursor, error) {
	if cursor == "" {
		return nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, ErrInvalidPage
	}
	micros, id, ok := strings.Cut(string(raw), ".")
	if !ok {
		return nil, ErrInvalidPage
	}
	n, err := strconv.ParseInt(micros, 10, 64)
	if err != nil {
		return nil, ErrInvalidPage
	}
	id, err = NormalizeID(id)
	if err != nil {
		return nil, ErrInvalidPage
	}
	return &Cursor{UpdatedAt: time.UnixMicro(n).UTC(), ID: id}, nil
}
