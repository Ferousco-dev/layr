package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/netip"
	"net/url"
	"time"

	"github.com/ferousco-dev/layr/server/internal/auth"
	"github.com/ferousco-dev/layr/server/internal/httpapi/middleware"
)

const (
	rateWindow = time.Minute
	// Defaults used when AuthOptions.Limits leaves a budget at zero.
	rateLimit     = 20
	apiIPLimit    = 300
	apiWriteLimit = 30
	importLimit   = 6
)

// Limits are per-minute budgets: Login and API per address, Write per user.
type Limits struct {
	Login int64
	API   int64
	Write int64
	// Import bounds import starts and frame selections per user.
	Import int64
}

func (l Limits) withDefaults() Limits {
	if l.Login == 0 {
		l.Login = rateLimit
	}
	if l.API == 0 {
		l.API = apiIPLimit
	}
	if l.Write == 0 {
		l.Write = apiWriteLimit
	}
	if l.Import == 0 {
		l.Import = importLimit
	}
	return l
}

type AuthService interface {
	Begin(ctx context.Context) (string, error)
	Consume(ctx context.Context, state string) error
	Complete(ctx context.Context, code, state string) (auth.Session, error)
	Authenticate(ctx context.Context, token string) (auth.User, error)
	Logout(ctx context.Context, token string) error
}

type AuthOptions struct {
	Account      AccountService
	Generations  GenerationService
	Plans        GenerationPlanService
	Design       DesignService
	Imports      ImportService
	Projects     ProjectService
	Service      AuthService
	Limiter      middleware.Limiter
	CookieName   string
	CookieSecure bool
	Limits       Limits
	// TrustedProxies lists the proxy networks whose X-Forwarded-For is believed.
	TrustedProxies []netip.Prefix
}

type authRoutes struct {
	AuthOptions
	frontend string
	log      *slog.Logger
}

type userKey struct{}

// CurrentUser returns the authenticated Layr user placed in the context by RequireAuth.
func CurrentUser(ctx context.Context) (auth.User, bool) {
	u, ok := ctx.Value(userKey{}).(auth.User)
	return u, ok
}

func (a *authRoutes) register(mux *http.ServeMux) {
	limited := func(name string, next http.HandlerFunc) http.HandlerFunc {
		return middleware.RateLimit(a.Limiter, name, a.Limits.withDefaults().Login, rateWindow, middleware.ClientIP(a.TrustedProxies), next).ServeHTTP
	}
	mux.HandleFunc("/auth/figma", method(http.MethodGet, limited("auth-start", a.start)))
	mux.HandleFunc("/auth/figma/callback", method(http.MethodGet, limited("auth-callback", a.callback)))
	mux.HandleFunc("/auth/logout", method(http.MethodPost, a.logout))
	mux.HandleFunc("/api/v1/me", method(http.MethodGet, a.api(a.me)))
	if a.Projects != nil {
		(&projectRoutes{svc: a.Projects, require: a.api, log: a.log}).register(mux)
	}
	if a.Account != nil {
		(&accountRoutes{svc: a.Account, require: a.api, clearCookie: a.expiredCookie, log: a.log}).register(mux)
	}
	if a.Generations != nil {
		(&generationRoutes{svc: a.Generations, require: a.api, budget: a.importBudget, log: a.log}).register(mux)
	}
	if a.Plans != nil {
		(&generationPlanRoutes{svc: a.Plans, require: a.api, log: a.log}).register(mux)
	}
	if a.Design != nil {
		(&designRoutes{svc: a.Design, require: a.api, log: a.log}).register(mux)
	}
	if a.Imports != nil {
		(&importRoutes{svc: a.Imports, require: a.api, budget: a.importBudget, log: a.log}).register(mux)
	}
}

func (a *authRoutes) start(w http.ResponseWriter, r *http.Request) {
	target, err := a.Service.Begin(r.Context())
	if err != nil {
		failure(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required service is unavailable. Try again shortly.")
		return
	}
	http.Redirect(w, r, target, http.StatusFound)
}

func (a *authRoutes) callback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	state := q.Get("state")

	if q.Get("error") != "" {
		_ = a.Service.Consume(r.Context(), state)
		a.fail(w, r, "FIGMA_AUTH_FAILED")
		return
	}
	code := q.Get("code")
	if code == "" || state == "" {
		a.fail(w, r, "OAUTH_CALLBACK_FAILED")
		return
	}

	session, err := a.Service.Complete(r.Context(), code, state)
	if err != nil {
		a.fail(w, r, callbackCode(err))
		return
	}
	http.SetCookie(w, a.cookie(session.Token, session.ExpiresAt))
	http.Redirect(w, r, a.frontend+"/", http.StatusFound)
}

// fail sends the browser back to the fixed frontend origin with a stable code only.
func (a *authRoutes) fail(w http.ResponseWriter, r *http.Request, code string) {
	a.log.WarnContext(r.Context(), "auth.callback.failed",
		slog.String("request_id", middleware.RequestID(r.Context())), slog.String("code", code))
	http.Redirect(w, r, a.frontend+"/?auth_error="+url.QueryEscape(code), http.StatusFound)
}

func callbackCode(err error) string {
	switch {
	case errors.Is(err, auth.ErrStateInvalid):
		return "OAUTH_STATE_INVALID"
	case errors.Is(err, auth.ErrFigmaAuthFailed):
		return "FIGMA_AUTH_FAILED"
	case errors.Is(err, auth.ErrIdentityFailed):
		return "FIGMA_IDENTITY_FAILED"
	case errors.Is(err, auth.ErrDependency):
		return "DEPENDENCY_UNAVAILABLE"
	default:
		return "OAUTH_CALLBACK_FAILED"
	}
}

func (a *authRoutes) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(a.CookieName); err == nil {
		if err := a.Service.Logout(r.Context(), c.Value); err != nil {
			failure(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required service is unavailable. Try again shortly.")
			return
		}
	}
	http.SetCookie(w, a.expiredCookie())
	w.WriteHeader(http.StatusNoContent)
}

// api applies the per-address limit, authentication, then the per-user write limit.
func (a *authRoutes) api(next http.HandlerFunc) http.HandlerFunc {
	perUser := userKeyOf
	write := func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next(w, r)
			return
		}
		middleware.RateLimit(a.Limiter, "api-write", a.Limits.withDefaults().Write, rateWindow, perUser, next).ServeHTTP(w, r)
	}
	authed := a.require(write)
	return middleware.RateLimit(a.Limiter, "api-ip", a.Limits.withDefaults().API, rateWindow, middleware.ClientIP(a.TrustedProxies), authed).ServeHTTP
}

func userKeyOf(r *http.Request) string {
	u, _ := CurrentUser(r.Context())
	return u.ID
}

// importBudget caps import starts and selections per user, since each one costs Figma calls.
func (a *authRoutes) importBudget(next http.HandlerFunc) http.HandlerFunc {
	return middleware.RateLimit(a.Limiter, "import", a.Limits.withDefaults().Import, rateWindow, userKeyOf, next).ServeHTTP
}

func (a *authRoutes) require(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(a.CookieName)
		if err != nil {
			failure(w, r, http.StatusUnauthorized, "AUTH_REQUIRED", "Sign in to continue.")
			return
		}

		user, err := a.Service.Authenticate(r.Context(), c.Value)
		switch {
		case errors.Is(err, auth.ErrSessionInvalid):
			http.SetCookie(w, a.expiredCookie())
			failure(w, r, http.StatusUnauthorized, "INVALID_SESSION", "Your session is no longer valid. Sign in again.")
			return
		case err != nil:
			failure(w, r, http.StatusServiceUnavailable, "DEPENDENCY_UNAVAILABLE", "A required service is unavailable. Try again shortly.")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), userKey{}, user)))
	}
}

func (a *authRoutes) me(w http.ResponseWriter, r *http.Request) {
	u, _ := CurrentUser(r.Context())
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
		"id":         u.ID,
		"email":      optional(u.Email),
		"name":       u.DisplayName,
		"avatar_url": optional(u.AvatarURL),
	}})
}

func optional(v string) any {
	if v == "" {
		return nil
	}
	return v
}

// cookie is HttpOnly and SameSite=Lax so the Figma redirect can set it but cross-site POSTs cannot send it.
func (a *authRoutes) cookie(token string, expires time.Time) *http.Cookie {
	return &http.Cookie{
		Name: a.CookieName, Value: token, Path: "/", Expires: expires,
		HttpOnly: true, Secure: a.CookieSecure, SameSite: http.SameSiteLaxMode,
	}
}

func (a *authRoutes) expiredCookie() *http.Cookie {
	c := a.cookie("", time.Unix(0, 0))
	c.MaxAge = -1
	return c
}
