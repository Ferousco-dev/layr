package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/ferousco-dev/layr/server/internal/account"
)

// deleteConfirmation is the exact phrase a person must send to delete their account.
const deleteConfirmation = "delete my account"

type AccountService interface {
	Profile(ctx context.Context, userID string) (account.Profile, error)
	SaveKey(ctx context.Context, userID, provider, key string) (account.KeyStatus, error)
	DeleteKey(ctx context.Context, userID, provider string) error
	DeleteAccount(ctx context.Context, userID string) error
}

type accountRoutes struct {
	svc         AccountService
	require     func(http.HandlerFunc) http.HandlerFunc
	clearCookie func() *http.Cookie
	log         *slog.Logger
}

var providerNames = map[string]string{
	account.Anthropic: "Anthropic",
	account.OpenAI:    "OpenAI",
	account.XAI:       "xAI",
}

func (a *accountRoutes) register(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/profile", a.require(methods(map[string]http.HandlerFunc{
		http.MethodGet:    a.profile,
		http.MethodDelete: a.deleteAccount,
	})))
	mux.HandleFunc("/api/v1/profile/ai-keys/{provider}", a.require(methods(map[string]http.HandlerFunc{
		http.MethodPut:    a.saveKey,
		http.MethodDelete: a.deleteKey,
	})))
}

func keyJSON(k account.KeyStatus) map[string]any {
	out := map[string]any{"provider": k.Provider, "saved": k.Saved, "hint": nil, "updated_at": nil}
	if k.Saved {
		out["hint"] = k.Hint
		out["updated_at"] = k.UpdatedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func (a *accountRoutes) profile(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	p, err := a.svc.Profile(r.Context(), u.ID)
	if err != nil {
		unexpected(w, r, a.log, "profile.failed", err)
		return
	}
	keys := make([]map[string]any, 0, len(p.Keys))
	for _, k := range p.Keys {
		keys = append(keys, keyJSON(k))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{
		"user": map[string]any{
			"id":         u.ID,
			"name":       u.DisplayName,
			"email":      optional(u.Email),
			"avatar_url": optional(u.AvatarURL),
		},
		"figma":   map[string]any{"status": p.FigmaStatus},
		"ai_keys": keys,
	}})
}

func (a *accountRoutes) saveKey(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	provider := r.PathValue("provider")
	var body struct {
		APIKey *string `json:"api_key"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.APIKey == nil {
		failure(w, r, http.StatusBadRequest, "INVALID_API_KEY", "Enter an API key.")
		return
	}
	saved, err := a.svc.SaveKey(r.Context(), u.ID, provider, *body.APIKey)
	switch {
	case errors.Is(err, account.ErrUnknownProvider):
		failure(w, r, http.StatusNotFound, "UNKNOWN_PROVIDER", "That AI provider is not supported.")
	case errors.Is(err, account.ErrInvalidKey):
		failure(w, r, http.StatusBadRequest, "INVALID_API_KEY", "That does not look like a valid "+providerNames[provider]+" API key.")
	case err != nil:
		unexpected(w, r, a.log, "profile.key_save_failed", err)
	default:
		writeJSON(w, http.StatusOK, map[string]any{"data": keyJSON(saved)})
	}
}

func (a *accountRoutes) deleteKey(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	err := a.svc.DeleteKey(r.Context(), u.ID, r.PathValue("provider"))
	switch {
	case errors.Is(err, account.ErrUnknownProvider):
		failure(w, r, http.StatusNotFound, "UNKNOWN_PROVIDER", "That AI provider is not supported.")
	case err != nil:
		unexpected(w, r, a.log, "profile.key_delete_failed", err)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}

func (a *accountRoutes) deleteAccount(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	var body struct {
		Confirm string `json:"confirm"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.Confirm != deleteConfirmation {
		failure(w, r, http.StatusBadRequest, "CONFIRMATION_REQUIRED", "Type the confirmation phrase to delete your account.")
		return
	}
	err := a.svc.DeleteAccount(r.Context(), u.ID)
	switch {
	case errors.Is(err, account.ErrUserNotFound):
		failure(w, r, http.StatusNotFound, "ACCOUNT_NOT_FOUND", "This account no longer exists.")
	case err != nil:
		unexpected(w, r, a.log, "profile.delete_failed", err)
	default:
		http.SetCookie(w, a.clearCookie())
		w.WriteHeader(http.StatusNoContent)
	}
}
