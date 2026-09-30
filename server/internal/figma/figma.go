// Package figma is the OAuth and identity client for the Figma REST API.
package figma

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	authorizeURL = "https://www.figma.com/oauth"
	tokenURL     = "https://api.figma.com/v1/oauth/token"
	refreshURL   = "https://api.figma.com/v1/oauth/refresh"
	meURL        = "https://api.figma.com/v1/me"
	maxBody      = 1 << 20
)

// Scopes are what Layr asks for: identity and file content, both read-only.
var Scopes = []string{"current_user:read", "file_content:read"}

var (
	ErrRejected    = errors.New("figma rejected the request")
	ErrUnavailable = errors.New("figma unavailable")
	ErrMalformed   = errors.New("figma response malformed")
)

type Tokens struct {
	Access    string
	Refresh   string
	ExpiresAt time.Time
	UserID    string
}

type Identity struct {
	ID          string
	Email       string
	DisplayName string
	AvatarURL   string
}

type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	HTTP         *http.Client
	Now          func() time.Time
	AuthorizeURL string
	TokenURL     string
	RefreshURL   string
	MeURL        string
}

type Client struct{ cfg Config }

func New(cfg Config) *Client {
	if cfg.HTTP == nil {
		cfg.HTTP = &http.Client{Timeout: 10 * time.Second}
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	cfg.AuthorizeURL = orDefault(cfg.AuthorizeURL, authorizeURL)
	cfg.TokenURL = orDefault(cfg.TokenURL, tokenURL)
	cfg.RefreshURL = orDefault(cfg.RefreshURL, refreshURL)
	cfg.MeURL = orDefault(cfg.MeURL, meURL)
	return &Client{cfg: cfg}
}

func orDefault(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

func (c *Client) AuthorizeURL(state, challenge string) string {
	q := url.Values{}
	q.Set("client_id", c.cfg.ClientID)
	q.Set("redirect_uri", c.cfg.RedirectURI)
	q.Set("scope", strings.Join(Scopes, " "))
	q.Set("state", state)
	q.Set("response_type", "code")
	q.Set("code_challenge", challenge)
	q.Set("code_challenge_method", "S256")
	return c.cfg.AuthorizeURL + "?" + q.Encode()
}

func (c *Client) Exchange(ctx context.Context, code, verifier string) (Tokens, error) {
	form := url.Values{}
	form.Set("redirect_uri", c.cfg.RedirectURI)
	form.Set("code", code)
	form.Set("grant_type", "authorization_code")
	form.Set("code_verifier", verifier)

	t, err := c.tokenCall(ctx, c.cfg.TokenURL, form)
	if err != nil {
		return Tokens{}, err
	}
	if t.Refresh == "" || t.UserID == "" {
		return Tokens{}, ErrMalformed
	}
	return t, nil
}

// Refresh returns a new access token; Figma keeps the refresh token unchanged unless it sends one.
func (c *Client) Refresh(ctx context.Context, refreshToken string) (Tokens, error) {
	form := url.Values{}
	form.Set("refresh_token", refreshToken)
	return c.tokenCall(ctx, c.cfg.RefreshURL, form)
}

func (c *Client) tokenCall(ctx context.Context, endpoint string, form url.Values) (Tokens, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return Tokens{}, ErrUnavailable
	}
	req.SetBasicAuth(c.cfg.ClientID, c.cfg.ClientSecret)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var body struct {
		UserID    string `json:"user_id_string"`
		Access    string `json:"access_token"`
		Refresh   string `json:"refresh_token"`
		ExpiresIn int64  `json:"expires_in"`
	}
	if err := c.do(req, &body); err != nil {
		return Tokens{}, err
	}
	if body.Access == "" || body.ExpiresIn <= 0 {
		return Tokens{}, ErrMalformed
	}
	return Tokens{
		Access:    body.Access,
		Refresh:   body.Refresh,
		ExpiresAt: c.cfg.Now().UTC().Add(time.Duration(body.ExpiresIn) * time.Second),
		UserID:    body.UserID,
	}, nil
}

func (c *Client) Me(ctx context.Context, accessToken string) (Identity, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.cfg.MeURL, nil)
	if err != nil {
		return Identity{}, ErrUnavailable
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	var body struct {
		ID     string `json:"id"`
		Email  string `json:"email"`
		Handle string `json:"handle"`
		ImgURL string `json:"img_url"`
	}
	if err := c.do(req, &body); err != nil {
		return Identity{}, err
	}
	if body.ID == "" {
		return Identity{}, ErrMalformed
	}
	return Identity{ID: body.ID, Email: body.Email, DisplayName: body.Handle, AvatarURL: body.ImgURL}, nil
}

// ProviderError is a rejection from Figma; Reason is a short provider label for diagnostics only.
type ProviderError struct {
	Status int
	Reason string
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("figma rejected the request (status %d: %s)", e.Status, e.Reason)
}

func (e *ProviderError) Unwrap() error { return ErrRejected }

// providerReason extracts Figma's short error label, bounded and stripped of control characters.
func providerReason(body io.Reader) string {
	var v struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Err     string `json:"err"`
	}
	if json.NewDecoder(io.LimitReader(body, 2048)).Decode(&v) != nil {
		return "no reason given"
	}
	reason := v.Error
	if v.Message != "" {
		reason = v.Message
	}
	if reason == "" {
		reason = v.Err
	}
	clean := strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, reason)
	if len(clean) > 120 {
		clean = clean[:120]
	}
	if clean == "" {
		return "no reason given"
	}
	return clean
}

// do maps every provider outcome to a sentinel so raw bodies never reach callers.
func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.cfg.HTTP.Do(req)
	if err != nil {
		return ErrUnavailable
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusBadRequest, resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden:
		return &ProviderError{Status: resp.StatusCode, Reason: providerReason(resp.Body)}
	case resp.StatusCode != http.StatusOK:
		return ErrUnavailable
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxBody)).Decode(out); err != nil {
		return ErrMalformed
	}
	return nil
}
