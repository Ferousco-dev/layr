// Package account owns what a person keeps in Layr besides their Figma identity: AI provider keys and the right to delete everything.
package account

import (
	"context"
	"errors"
	"log/slog"
	"regexp"
	"time"
)

const purposePrefix = "ai-key/"

// Provider names accepted by the API; the same list is enforced by the database.
const (
	Anthropic = "anthropic"
	OpenAI    = "openai"
	XAI       = "xai"
)

var (
	ErrUnknownProvider = errors.New("unknown provider")
	ErrInvalidKey      = errors.New("api key not valid")
	ErrKeyNotFound     = errors.New("api key not found")
	ErrUserNotFound    = errors.New("user not found")
)

// rule is the shape a provider's keys are known to have; it catches typos, not fake keys.
type rule struct {
	prefix string
	min    int
	max    int
}

var rules = map[string]rule{
	Anthropic: {prefix: "sk-ant-", min: 20, max: 300},
	OpenAI:    {prefix: "sk-", min: 20, max: 300},
	XAI:       {prefix: "xai-", min: 20, max: 300},
}

var keyChars = regexp.MustCompile(`^[A-Za-z0-9_\-.]+$`)

// Providers lists the supported providers in display order.
func Providers() []string { return []string{Anthropic, OpenAI, XAI} }

// ValidateKey checks the provider and the key's shape without contacting the provider.
func ValidateKey(provider, key string) error {
	r, ok := rules[provider]
	if !ok {
		return ErrUnknownProvider
	}
	if len(key) < r.min || len(key) > r.max || len(key) < len(r.prefix) || key[:len(r.prefix)] != r.prefix || !keyChars.MatchString(key) {
		return ErrInvalidKey
	}
	return nil
}

// Hint is the only part of a key that is ever shown again: its last four characters.
func Hint(key string) string {
	if len(key) < 4 {
		return ""
	}
	return key[len(key)-4:]
}

type KeyStatus struct {
	Provider  string
	Saved     bool
	Hint      string
	UpdatedAt time.Time
}

type Profile struct {
	FigmaStatus string
	Keys        []KeyStatus
}

type Store interface {
	Keys(ctx context.Context, userID string) ([]KeyStatus, error)
	SaveKey(ctx context.Context, userID, provider string, ciphertext []byte, hint string, now time.Time) (KeyStatus, error)
	DeleteKey(ctx context.Context, userID, provider string) (bool, error)
	KeyCiphertext(ctx context.Context, userID, provider string) ([]byte, error)
	FigmaStatus(ctx context.Context, userID string) (string, error)
	ImportIDs(ctx context.Context, userID string) ([]string, error)
	DeleteUser(ctx context.Context, userID string) (bool, error)
}

type Sealer interface {
	Seal(plaintext []byte, purpose string) ([]byte, error)
	Open(sealed []byte, purpose string) ([]byte, error)
}

// Workspaces removes the temporary files of imports that belong to a deleted account.
type Workspaces interface {
	Cleanup(importID string) error
}

type Service struct {
	store  Store
	sealer Sealer
	files  Workspaces
	log    *slog.Logger
	now    func() time.Time
}

func New(store Store, sealer Sealer, files Workspaces, log *slog.Logger) *Service {
	return &Service{store: store, sealer: sealer, files: files, log: log, now: func() time.Time { return time.Now().UTC() }}
}

// Profile returns the Figma connection state and which AI providers have a key saved.
func (s *Service) Profile(ctx context.Context, userID string) (Profile, error) {
	keys, err := s.store.Keys(ctx, userID)
	if err != nil {
		return Profile{}, err
	}
	status, err := s.store.FigmaStatus(ctx, userID)
	if err != nil {
		return Profile{}, err
	}
	return Profile{FigmaStatus: status, Keys: withAllProviders(keys)}, nil
}

func withAllProviders(saved []KeyStatus) []KeyStatus {
	byProvider := map[string]KeyStatus{}
	for _, k := range saved {
		byProvider[k.Provider] = k
	}
	out := make([]KeyStatus, 0, len(rules))
	for _, p := range Providers() {
		k, ok := byProvider[p]
		if !ok {
			k = KeyStatus{Provider: p}
		}
		out = append(out, k)
	}
	return out
}

// SaveKey encrypts and stores a provider key, replacing any earlier one; the plaintext is never kept.
func (s *Service) SaveKey(ctx context.Context, userID, provider, key string) (KeyStatus, error) {
	if err := ValidateKey(provider, key); err != nil {
		return KeyStatus{}, err
	}
	sealed, err := s.sealer.Seal([]byte(key), purposePrefix+provider)
	if err != nil {
		return KeyStatus{}, err
	}
	saved, err := s.store.SaveKey(ctx, userID, provider, sealed, Hint(key), s.now())
	if err != nil {
		return KeyStatus{}, err
	}
	s.log.InfoContext(ctx, "account.key_saved", slog.String("user_id", userID), slog.String("provider", provider))
	return saved, nil
}

// DeleteKey removes a saved key; it is not an error if none was saved.
func (s *Service) DeleteKey(ctx context.Context, userID, provider string) error {
	if _, ok := rules[provider]; !ok {
		return ErrUnknownProvider
	}
	if _, err := s.store.DeleteKey(ctx, userID, provider); err != nil {
		return err
	}
	s.log.InfoContext(ctx, "account.key_deleted", slog.String("user_id", userID), slog.String("provider", provider))
	return nil
}

// OpenKey decrypts a saved key for server-side use by code generation; it is never exposed over HTTP.
func (s *Service) OpenKey(ctx context.Context, userID, provider string) (string, error) {
	if _, ok := rules[provider]; !ok {
		return "", ErrUnknownProvider
	}
	sealed, err := s.store.KeyCiphertext(ctx, userID, provider)
	if err != nil {
		return "", err
	}
	plain, err := s.sealer.Open(sealed, purposePrefix+provider)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

// DeleteAccount permanently removes the person and everything that belongs to them, then clears their temporary files.
func (s *Service) DeleteAccount(ctx context.Context, userID string) error {
	importIDs, err := s.store.ImportIDs(ctx, userID)
	if err != nil {
		return err
	}
	deleted, err := s.store.DeleteUser(ctx, userID)
	if err != nil {
		return err
	}
	if !deleted {
		return ErrUserNotFound
	}
	for _, id := range importIDs {
		if err := s.files.Cleanup(id); err != nil {
			s.log.WarnContext(ctx, "account.workspace_cleanup_failed", slog.String("import_id", id))
		}
	}
	s.log.InfoContext(ctx, "account.deleted", slog.String("user_id", userID), slog.Int("imports", len(importIDs)))
	return nil
}
