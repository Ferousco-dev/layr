// Package config loads and validates the process environment (DES-019).
package config

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type SecretURL string

type Secret string

// String hides the value from accidental formatting.
func (Secret) String() string { return "[REDACTED]" }

type Auth struct {
	FigmaClientID     string
	FigmaClientSecret Secret
	FigmaRedirectURI  string
	CredentialKey     []byte
	CookieName        string
	CookieSecure      bool
	SessionTTL        time.Duration
	StateTTL          time.Duration
}

type HTTP struct {
	Address           string
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	ShutdownTimeout   time.Duration
	MaxBodyBytes      int64
	MaxHeaderBytes    int
	TrustedProxies    []netip.Prefix
	RateLimits        RateLimits
}

// RateLimits are per-minute request budgets.
type RateLimits struct {
	Login  int64
	API    int64
	Write  int64
	Import int64
}

type Postgres struct {
	URL            SecretURL
	MaxConnections int32
}

type Redis struct {
	URL      SecretURL
	PoolSize int
}

const (
	EnvDevelopment = "development"
	EnvProduction  = "production"

	defaultFrontendOrigin = "http://localhost:3000"
	defaultWorkspaceRoot  = "/tmp/layr/jobs"
)

type Config struct {
	Env                   string
	FrontendOrigin        string
	WorkspaceRoot         string
	HTTP                  HTTP
	Import                Import
	Auth                  Auth
	Postgres              Postgres
	Redis                 Redis
	LogLevel              slog.Level
	StartupTimeout        time.Duration
	ReadinessTimeout      time.Duration
	ReadinessProbeTimeout time.Duration
}

type FieldError struct {
	Name string
	Kind string
}

func (e *FieldError) Error() string {
	return fmt.Sprintf("configuration %s: %s", e.Kind, e.Name)
}

type Lookup func(string) (string, bool)

func Load(lookup Lookup) (Config, error) {
	if lookup == nil {
		return Config{}, errors.New("configuration lookup is nil")
	}
	c := Config{}
	var err error
	if c.HTTP.Address, err = required(lookup, "HTTP_ADDRESS"); err != nil {
		return Config{}, err
	}
	if c.Postgres.URL, err = secret(lookup, "POSTGRES_URL"); err != nil {
		return Config{}, err
	}
	if c.Redis.URL, err = secret(lookup, "REDIS_URL"); err != nil {
		return Config{}, err
	}

	if err := loadEnvironment(lookup, &c); err != nil {
		return Config{}, err
	}
	if err := loadImport(lookup, &c); err != nil {
		return Config{}, err
	}
	if err := loadAuth(lookup, &c); err != nil {
		return Config{}, err
	}
	if err := loadDurations(lookup, &c); err != nil {
		return Config{}, err
	}
	if c.HTTP.TrustedProxies, err = trustedProxies(lookup); err != nil {
		return Config{}, err
	}
	if err := loadSizes(lookup, &c); err != nil {
		return Config{}, err
	}
	if err := c.LogLevel.UnmarshalText(
		[]byte(value(lookup, "LOG_LEVEL", "info")),
	); err != nil {
		return Config{}, &FieldError{Name: "LOG_LEVEL", Kind: "malformed"}
	}

	return c, nil
}

// loadEnvironment makes production name its origin and workspace explicitly.
func loadEnvironment(lookup Lookup, c *Config) error {
	c.Env = value(lookup, "APP_ENV", EnvDevelopment)
	if c.Env != EnvDevelopment && c.Env != EnvProduction {
		return &FieldError{Name: "APP_ENV", Kind: "malformed"}
	}

	origin, err := explicitOrDefault(lookup, c.Env, "FRONTEND_URL", defaultFrontendOrigin)
	if err != nil {
		return err
	}
	if c.FrontendOrigin, err = parseOrigin(origin); err != nil {
		return err
	}

	root, err := explicitOrDefault(lookup, c.Env, "TEMP_WORKSPACE_ROOT", defaultWorkspaceRoot)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(root) {
		return &FieldError{Name: "TEMP_WORKSPACE_ROOT", Kind: "malformed"}
	}
	c.WorkspaceRoot = filepath.Clean(root)
	return nil
}

func explicitOrDefault(l Lookup, env, name, fallback string) (string, error) {
	if env == EnvProduction {
		return required(l, name)
	}
	return value(l, name, fallback), nil
}

// parseOrigin accepts only scheme://host[:port], the form browsers send as Origin.
func parseOrigin(raw string) (string, error) {
	u, err := url.Parse(raw)
	bad := &FieldError{Name: "FRONTEND_URL", Kind: "malformed"}
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", bad
	}
	if (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return "", bad
	}
	return u.Scheme + "://" + u.Host, nil
}

// Import bounds the Figma importer's background work and temporary disk use.
type Import struct {
	WorkspaceTTL time.Duration
	Timeout      time.Duration
	// FigmaRequestsPerMinute is Layr's own ceiling on requests to Figma per person.
	FigmaRequestsPerMinute int
	MaxSnapshotBytes       int64
	MaxWorkspaceBytes      int64
	// Asset pipeline limits.
	MaxAssetBytes       int64
	MaxImportAssetBytes int64
	AssetConcurrency    int
	MaxAssets           int
	// Design IR limits.
	MaxIRBytes int64
	MaxIRNodes int
	MaxScreens int
}

func loadImport(l Lookup, c *Config) error {
	var err error
	i := &c.Import
	if i.WorkspaceTTL, err = duration(l, "TEMP_WORKSPACE_TTL", 24*time.Hour, 30*24*time.Hour); err != nil {
		return err
	}
	if i.Timeout, err = duration(l, "IMPORT_TIMEOUT", 5*time.Minute, 10*time.Minute); err != nil {
		return err
	}
	perMinute, err := int64Value(l, "FIGMA_REQUESTS_PER_MINUTE", 8, 1, 600)
	if err != nil {
		return err
	}
	i.FigmaRequestsPerMinute = int(perMinute)
	if i.MaxSnapshotBytes, err = int64Value(l, "MAX_SNAPSHOT_BYTES", 128<<20, 1<<10, 1<<30); err != nil {
		return err
	}
	if i.MaxWorkspaceBytes, err = int64Value(l, "MAX_WORKSPACE_BYTES", 512<<20, 1<<10, 4<<30); err != nil {
		return err
	}
	if i.MaxWorkspaceBytes < i.MaxSnapshotBytes {
		return &FieldError{Name: "MAX_WORKSPACE_BYTES", Kind: "out_of_range"}
	}
	if err := loadAssetLimits(l, i); err != nil {
		return err
	}
	return loadDesignLimits(l, i)
}

func loadDesignLimits(l Lookup, i *Import) error {
	var err error
	if i.MaxIRBytes, err = int64Value(l, "MAX_IR_BYTES", 64<<20, 1<<20, 1<<30); err != nil {
		return err
	}
	nodes, err := int64Value(l, "MAX_IR_NODES", 200000, 100, 2000000)
	if err != nil {
		return err
	}
	// The importer asks Figma for 100 nodes at a time, so the screens of one import are bounded here instead.
	screens, err := int64Value(l, "MAX_SCREENS", 500, 1, 500)
	if err != nil {
		return err
	}
	i.MaxIRNodes, i.MaxScreens = int(nodes), int(screens)
	if i.MaxIRBytes > i.MaxSnapshotBytes {
		return &FieldError{Name: "MAX_IR_BYTES", Kind: "out_of_range"}
	}
	return nil
}

func loadAssetLimits(l Lookup, i *Import) error {
	var err error
	if i.MaxAssetBytes, err = int64Value(l, "MAX_ASSET_SIZE_BYTES", 50<<20, 1<<10, 512<<20); err != nil {
		return err
	}
	if i.MaxImportAssetBytes, err = int64Value(l, "MAX_IMPORT_ASSET_BYTES", 256<<20, 1<<10, 4<<30); err != nil {
		return err
	}
	concurrency, err := int64Value(l, "ASSET_DOWNLOAD_CONCURRENCY", 4, 1, 16)
	if err != nil {
		return err
	}
	maxAssets, err := int64Value(l, "MAX_IMPORT_ASSETS", 300, 1, 2000)
	if err != nil {
		return err
	}
	i.AssetConcurrency, i.MaxAssets = int(concurrency), int(maxAssets)

	// Each asset is stored through the workspace writer, so these caps must fit inside its limits.
	switch {
	case i.MaxAssetBytes > i.MaxSnapshotBytes:
		return &FieldError{Name: "MAX_ASSET_SIZE_BYTES", Kind: "out_of_range"}
	case i.MaxImportAssetBytes < i.MaxAssetBytes || i.MaxImportAssetBytes > i.MaxWorkspaceBytes:
		return &FieldError{Name: "MAX_IMPORT_ASSET_BYTES", Kind: "out_of_range"}
	}
	return nil
}

func loadAuth(lookup Lookup, c *Config) error {
	var err error
	a := &c.Auth
	if a.FigmaClientID, err = required(lookup, "FIGMA_CLIENT_ID"); err != nil {
		return err
	}
	secret, err := required(lookup, "FIGMA_CLIENT_SECRET")
	if err != nil {
		return err
	}
	a.FigmaClientSecret = Secret(secret)

	if a.FigmaRedirectURI, err = redirectURI(lookup, c.Env); err != nil {
		return err
	}
	if a.CredentialKey, err = credentialKey(lookup); err != nil {
		return err
	}

	a.CookieName = value(lookup, "SESSION_COOKIE_NAME", "layr_session")
	if a.CookieSecure, err = cookieSecure(lookup, c.Env); err != nil {
		return err
	}
	if a.SessionTTL, err = duration(lookup, "SESSION_TTL", 7*24*time.Hour, 30*24*time.Hour); err != nil {
		return err
	}
	a.StateTTL, err = duration(lookup, "OAUTH_STATE_TTL", 10*time.Minute, 30*time.Minute)
	return err
}

// redirectURI demands https in production because the authorization code travels in it.
func redirectURI(l Lookup, env string) (string, error) {
	raw, err := required(l, "FIGMA_REDIRECT_URI")
	if err != nil {
		return "", err
	}
	u, perr := url.Parse(raw)
	bad := &FieldError{Name: "FIGMA_REDIRECT_URI", Kind: "malformed"}
	if perr != nil || u.Host == "" || u.Fragment != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", bad
	}
	if env == EnvProduction && u.Scheme != "https" {
		return "", bad
	}
	return raw, nil
}

// credentialKey requires exactly 32 base64 bytes for AES-256.
func credentialKey(l Lookup) ([]byte, error) {
	raw, err := required(l, "CREDENTIAL_ENCRYPTION_KEY")
	if err != nil {
		return nil, err
	}
	key, derr := base64.StdEncoding.DecodeString(raw)
	if derr != nil || len(key) != 32 {
		return nil, &FieldError{Name: "CREDENTIAL_ENCRYPTION_KEY", Kind: "malformed"}
	}
	return key, nil
}

// cookieSecure defaults to true and may only be disabled outside production.
func cookieSecure(l Lookup, env string) (bool, error) {
	secure, err := strconv.ParseBool(value(l, "SESSION_COOKIE_SECURE", "true"))
	if err != nil {
		return false, &FieldError{Name: "SESSION_COOKIE_SECURE", Kind: "malformed"}
	}
	if !secure && env == EnvProduction {
		return false, &FieldError{Name: "SESSION_COOKIE_SECURE", Kind: "out_of_range"}
	}
	return secure, nil
}

// trustedProxies parses the CIDRs whose X-Forwarded-For header may be believed.
func trustedProxies(l Lookup) ([]netip.Prefix, error) {
	raw := value(l, "TRUSTED_PROXY_CIDRS", "")
	var out []netip.Prefix
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(part)
		if err != nil {
			return nil, &FieldError{Name: "TRUSTED_PROXY_CIDRS", Kind: "malformed"}
		}
		out = append(out, prefix)
	}
	return out, nil
}

func loadDurations(lookup Lookup, c *Config) error {
	fields := []struct {
		name     string
		dst      *time.Duration
		fallback time.Duration
		maximum  time.Duration
	}{
		{"HTTP_READ_HEADER_TIMEOUT", &c.HTTP.ReadHeaderTimeout, 5 * time.Second, 5 * time.Second},
		{"HTTP_READ_TIMEOUT", &c.HTTP.ReadTimeout, 15 * time.Second, 15 * time.Second},
		{"HTTP_WRITE_TIMEOUT", &c.HTTP.WriteTimeout, 30 * time.Second, 30 * time.Second},
		{"HTTP_IDLE_TIMEOUT", &c.HTTP.IdleTimeout, 60 * time.Second, 60 * time.Second},
		{"HTTP_SHUTDOWN_TIMEOUT", &c.HTTP.ShutdownTimeout, 30 * time.Second, 5 * time.Minute},
		{"STARTUP_TIMEOUT", &c.StartupTimeout, 10 * time.Second, 10 * time.Second},
		{"READINESS_TIMEOUT", &c.ReadinessTimeout, time.Second, time.Second},
		{"READINESS_PROBE_TIMEOUT", &c.ReadinessProbeTimeout, 500 * time.Millisecond, 500 * time.Millisecond},
	}

	for _, field := range fields {
		parsed, err := duration(
			lookup,
			field.name,
			field.fallback,
			field.maximum,
		)
		if err != nil {
			return err
		}
		*field.dst = parsed
	}

	if c.ReadinessProbeTimeout > c.ReadinessTimeout {
		return &FieldError{
			Name: "READINESS_PROBE_TIMEOUT",
			Kind: "out_of_range",
		}
	}
	return nil
}

func loadSizes(lookup Lookup, c *Config) error {
	body, err := int64Value(lookup, "HTTP_MAX_BODY_BYTES", 1<<20, 1, 64<<20)
	if err != nil {
		return err
	}
	header, err := int64Value(lookup, "HTTP_MAX_HEADER_BYTES", 1<<20, 1, 16<<20)
	if err != nil {
		return err
	}
	postgresPool, err := int64Value(lookup, "POSTGRES_MAX_CONNECTIONS", 10, 1, 1000)
	if err != nil {
		return err
	}
	redisPool, err := int64Value(lookup, "REDIS_POOL_SIZE", 10, 1, 1000)
	if err != nil {
		return err
	}

	if c.HTTP.RateLimits, err = loadRateLimits(lookup); err != nil {
		return err
	}

	c.HTTP.MaxBodyBytes = body
	c.HTTP.MaxHeaderBytes = int(header)
	c.Postgres.MaxConnections = int32(postgresPool)
	c.Redis.PoolSize = int(redisPool)
	return nil
}

func loadRateLimits(l Lookup) (RateLimits, error) {
	var out RateLimits
	fields := []struct {
		name     string
		dst      *int64
		fallback int64
	}{
		{"RATE_LIMIT_LOGIN_PER_MINUTE", &out.Login, 20},
		{"RATE_LIMIT_API_PER_MINUTE", &out.API, 300},
		{"RATE_LIMIT_WRITE_PER_MINUTE", &out.Write, 30},
		{"RATE_LIMIT_IMPORT_PER_MINUTE", &out.Import, 6},
	}
	for _, f := range fields {
		v, err := int64Value(l, f.name, f.fallback, 1, 100000)
		if err != nil {
			return RateLimits{}, err
		}
		*f.dst = v
	}
	return out, nil
}

func required(l Lookup, n string) (string, error) {
	v, ok := l(n)
	if !ok || strings.TrimSpace(v) == "" {
		return "", &FieldError{Name: n, Kind: "missing"}
	}
	return v, nil
}

func secret(l Lookup, n string) (SecretURL, error) {
	v, err := required(l, n)
	return SecretURL(v), err
}

func value(l Lookup, n, d string) string {
	if v, ok := l(n); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return d
}

func duration(l Lookup, n string, d, max time.Duration) (time.Duration, error) {
	v, e := time.ParseDuration(value(l, n, d.String()))
	if e != nil {
		return 0, &FieldError{Name: n, Kind: "malformed"}
	}
	if v <= 0 || v > max {
		return 0, &FieldError{Name: n, Kind: "out_of_range"}
	}
	return v, nil
}

func int64Value(l Lookup, n string, d, min, max int64) (int64, error) {
	raw := value(l, n, strconv.FormatInt(d, 10))
	v, e := strconv.ParseInt(raw, 10, 64)
	if e != nil {
		return 0, &FieldError{Name: n, Kind: "malformed"}
	}
	if v < min || v > max {
		return 0, &FieldError{Name: n, Kind: "out_of_range"}
	}
	return v, nil
}
