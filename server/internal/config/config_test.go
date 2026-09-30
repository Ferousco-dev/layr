package config

import (
	"strings"
	"testing"
	"time"
)

func env(v map[string]string) Lookup {
	return func(k string) (string, bool) { x, ok := v[k]; return x, ok }
}
func valid() map[string]string {
	return map[string]string{
		"HTTP_ADDRESS":              ":8080",
		"POSTGRES_URL":              "postgres://u:top-secret@db/x",
		"REDIS_URL":                 "redis://:hidden@redis/0",
		"FIGMA_CLIENT_ID":           "client-id",
		"FIGMA_CLIENT_SECRET":       "client-secret",
		"FIGMA_REDIRECT_URI":        "http://localhost:8080/auth/figma/callback",
		"CREDENTIAL_ENCRYPTION_KEY": "bG9jYWwtZGV2LW9ubHktcmVwbGFjZS10aGlzLWtleSE=",
	}
}
func TestLoadDefaults(t *testing.T) {
	c, e := Load(env(valid()))
	if e != nil {
		t.Fatal(e)
	}
	if c.HTTP.ShutdownTimeout != 30*time.Second || c.Postgres.MaxConnections != 10 || c.Redis.PoolSize != 10 {
		t.Fatalf("unexpected defaults: %#v", c)
	}
}
func TestLoadDoesNotExposeSecrets(t *testing.T) {
	v := valid()
	v["HTTP_READ_TIMEOUT"] = "not-a-duration-top-secret"
	_, e := Load(env(v))
	if e == nil || strings.Contains(e.Error(), "top-secret") {
		t.Fatalf("unsafe error: %v", e)
	}
}
func TestLoadRequiresCriticalValues(t *testing.T) {
	v := valid()
	delete(v, "POSTGRES_URL")
	_, e := Load(env(v))
	if e == nil || !strings.Contains(e.Error(), "POSTGRES_URL") {
		t.Fatalf("unexpected: %v", e)
	}
}
func TestProbeCannotExceedAggregate(t *testing.T) {
	v := valid()
	v["READINESS_TIMEOUT"] = "400ms"
	_, e := Load(env(v))
	if e == nil {
		t.Fatal("expected bounds error")
	}
}

func TestLoadEnvironmentDefaultsForDevelopment(t *testing.T) {
	c, e := Load(env(valid()))

	if e != nil {
		t.Fatal(e)
	}
	if c.Env != EnvDevelopment || c.FrontendOrigin != "http://localhost:3000" || c.WorkspaceRoot != "/tmp/layr/jobs" {
		t.Fatalf("unexpected environment defaults: %#v", c)
	}
}

func TestLoadProductionRequiresExplicitValues(t *testing.T) {
	for _, missing := range []string{"FRONTEND_URL", "TEMP_WORKSPACE_ROOT"} {
		v := valid()
		v["APP_ENV"] = "production"
		v["FRONTEND_URL"] = "https://app.layr.dev"
		v["TEMP_WORKSPACE_ROOT"] = "/var/lib/layr/jobs"
		delete(v, missing)

		_, e := Load(env(v))

		if e == nil || !strings.Contains(e.Error(), missing) {
			t.Fatalf("%s: unexpected: %v", missing, e)
		}
	}
}

func TestLoadRejectsInvalidEnvironmentValues(t *testing.T) {
	cases := map[string]string{
		"APP_ENV":             "staging",
		"FRONTEND_URL":        "https://app.layr.dev/path",
		"TEMP_WORKSPACE_ROOT": "relative/dir",
	}
	for name, bad := range cases {
		v := valid()
		v[name] = bad

		_, e := Load(env(v))

		if e == nil || !strings.Contains(e.Error(), name) {
			t.Fatalf("%s: unexpected: %v", name, e)
		}
	}
}

func TestLoadNormalizesFrontendOrigin(t *testing.T) {
	v := valid()
	v["FRONTEND_URL"] = "https://app.layr.dev/"

	c, e := Load(env(v))

	if e != nil || c.FrontendOrigin != "https://app.layr.dev" {
		t.Fatalf("origin = %q, err = %v", c.FrontendOrigin, e)
	}
}

func TestLoadAuthDefaults(t *testing.T) {
	c, e := Load(env(valid()))

	if e != nil {
		t.Fatal(e)
	}
	a := c.Auth
	if a.CookieName != "layr_session" || !a.CookieSecure || a.SessionTTL != 7*24*time.Hour || a.StateTTL != 10*time.Minute || len(a.CredentialKey) != 32 {
		t.Fatalf("unexpected auth defaults: %#v", a)
	}
}

func TestLoadAuthRejectsInvalidValues(t *testing.T) {
	cases := map[string]string{
		"CREDENTIAL_ENCRYPTION_KEY": "c2hvcnQ=",
		"FIGMA_REDIRECT_URI":        "not a url",
		"SESSION_COOKIE_SECURE":     "maybe",
		"SESSION_TTL":               "9999h",
		"OAUTH_STATE_TTL":           "2h",
	}
	for name, bad := range cases {
		v := valid()
		v[name] = bad

		_, e := Load(env(v))

		if e == nil || !strings.Contains(e.Error(), name) || strings.Contains(e.Error(), bad) {
			t.Fatalf("%s: unexpected: %v", name, e)
		}
	}
}

func TestLoadAuthRequiresFigmaCredentials(t *testing.T) {
	for _, name := range []string{"FIGMA_CLIENT_ID", "FIGMA_CLIENT_SECRET", "FIGMA_REDIRECT_URI", "CREDENTIAL_ENCRYPTION_KEY"} {
		v := valid()
		delete(v, name)

		_, e := Load(env(v))

		if e == nil || !strings.Contains(e.Error(), name) {
			t.Fatalf("%s: unexpected: %v", name, e)
		}
	}
}

func TestLoadProductionRejectsInsecureAuth(t *testing.T) {
	base := func() map[string]string {
		v := valid()
		v["APP_ENV"] = "production"
		v["FRONTEND_URL"] = "https://app.layr.dev"
		v["TEMP_WORKSPACE_ROOT"] = "/var/lib/layr/jobs"
		v["FIGMA_REDIRECT_URI"] = "https://api.layr.dev/auth/figma/callback"
		return v
	}
	if _, e := Load(env(base())); e != nil {
		t.Fatal(e)
	}

	insecureCookie := base()
	insecureCookie["SESSION_COOKIE_SECURE"] = "false"
	if _, e := Load(env(insecureCookie)); e == nil {
		t.Fatal("production must reject insecure cookies")
	}

	plainRedirect := base()
	plainRedirect["FIGMA_REDIRECT_URI"] = "http://api.layr.dev/auth/figma/callback"
	if _, e := Load(env(plainRedirect)); e == nil {
		t.Fatal("production must reject http redirect URI")
	}
}

func TestSecretIsRedactedWhenFormatted(t *testing.T) {
	if got := Secret("hunter2").String(); got != "[REDACTED]" {
		t.Fatalf("got %q", got)
	}
}

func TestLoadRateLimits(t *testing.T) {
	c, e := Load(env(valid()))
	if e != nil || c.HTTP.RateLimits != (RateLimits{Login: 20, API: 300, Write: 30, Import: 6}) {
		t.Fatalf("defaults = %#v, err = %v", c.HTTP.RateLimits, e)
	}

	v := valid()
	v["RATE_LIMIT_API_PER_MINUTE"] = "1000"
	v["RATE_LIMIT_WRITE_PER_MINUTE"] = "5"
	c, e = Load(env(v))
	if e != nil || c.HTTP.RateLimits.API != 1000 || c.HTTP.RateLimits.Write != 5 {
		t.Fatalf("custom = %#v, err = %v", c.HTTP.RateLimits, e)
	}

	for _, bad := range []string{"0", "-1", "many", "999999"} {
		v["RATE_LIMIT_LOGIN_PER_MINUTE"] = bad
		if _, e := Load(env(v)); e == nil || !strings.Contains(e.Error(), "RATE_LIMIT_LOGIN_PER_MINUTE") {
			t.Fatalf("%q: unexpected: %v", bad, e)
		}
	}
}

func TestLoadImportSettings(t *testing.T) {
	c, e := Load(env(valid()))
	if e != nil || c.Import.WorkspaceTTL != 24*time.Hour || c.Import.Timeout != 5*time.Minute || c.Import.FigmaRequestsPerMinute != 8 ||
		c.Import.MaxSnapshotBytes != 128<<20 || c.Import.MaxWorkspaceBytes != 512<<20 {
		t.Fatalf("defaults = %#v, err = %v", c.Import, e)
	}

	for name, bad := range map[string]string{
		"TEMP_WORKSPACE_TTL": "0s", "IMPORT_TIMEOUT": "1h", "MAX_SNAPSHOT_BYTES": "5", "MAX_WORKSPACE_BYTES": "abc",
	} {
		v := valid()
		v[name] = bad
		if _, e := Load(env(v)); e == nil || !strings.Contains(e.Error(), name) {
			t.Fatalf("%s=%s: unexpected: %v", name, bad, e)
		}
	}

	v := valid()
	v["MAX_SNAPSHOT_BYTES"] = "1073741824"
	v["MAX_WORKSPACE_BYTES"] = "1048576"
	if _, e := Load(env(v)); e == nil || !strings.Contains(e.Error(), "MAX_WORKSPACE_BYTES") {
		t.Fatalf("workspace smaller than a snapshot must fail: %v", e)
	}
}

func TestLoadAssetLimits(t *testing.T) {
	c, e := Load(env(valid()))
	if e != nil || c.Import.MaxAssetBytes != 50<<20 || c.Import.MaxImportAssetBytes != 256<<20 || c.Import.AssetConcurrency != 4 || c.Import.MaxAssets != 300 {
		t.Fatalf("defaults = %#v, err = %v", c.Import, e)
	}

	for name, bad := range map[string]string{
		"MAX_ASSET_SIZE_BYTES": "10", "MAX_IMPORT_ASSET_BYTES": "nope", "ASSET_DOWNLOAD_CONCURRENCY": "0", "MAX_IMPORT_ASSETS": "5000",
	} {
		v := valid()
		v[name] = bad
		if _, e := Load(env(v)); e == nil || !strings.Contains(e.Error(), name) {
			t.Fatalf("%s=%s: unexpected: %v", name, bad, e)
		}
	}

	v := valid()
	v["MAX_ASSET_SIZE_BYTES"] = "268435456"
	if _, e := Load(env(v)); e == nil || !strings.Contains(e.Error(), "MAX_ASSET_SIZE_BYTES") {
		t.Fatalf("an asset larger than the snapshot limit must fail: %v", e)
	}
	v = valid()
	v["MAX_IMPORT_ASSET_BYTES"] = "1073741824"
	if _, e := Load(env(v)); e == nil || !strings.Contains(e.Error(), "MAX_IMPORT_ASSET_BYTES") {
		t.Fatalf("an asset budget larger than the workspace must fail: %v", e)
	}
}

func TestLoadDesignLimits(t *testing.T) {
	c, e := Load(env(valid()))
	if e != nil || c.Import.MaxIRBytes != 64<<20 || c.Import.MaxIRNodes != 200000 || c.Import.MaxScreens != 500 {
		t.Fatalf("defaults = %#v, err = %v", c.Import, e)
	}
	for name, bad := range map[string]string{"MAX_IR_BYTES": "10", "MAX_IR_NODES": "5", "MAX_SCREENS": "501"} {
		v := valid()
		v[name] = bad
		if _, e := Load(env(v)); e == nil || !strings.Contains(e.Error(), name) {
			t.Fatalf("%s=%s: unexpected: %v", name, bad, e)
		}
	}
	v := valid()
	v["MAX_IR_BYTES"] = "268435456"
	if _, e := Load(env(v)); e == nil || !strings.Contains(e.Error(), "MAX_IR_BYTES") {
		t.Fatalf("an IR limit above the snapshot limit must fail: %v", e)
	}
}

func TestLoadTrustedProxyCIDRs(t *testing.T) {
	v := valid()
	v["TRUSTED_PROXY_CIDRS"] = "10.0.0.0/8, 192.168.1.0/24"

	c, e := Load(env(v))

	if e != nil || len(c.HTTP.TrustedProxies) != 2 {
		t.Fatalf("proxies = %v, err = %v", c.HTTP.TrustedProxies, e)
	}
	v["TRUSTED_PROXY_CIDRS"] = "10.0.0.0/99"
	if _, e := Load(env(v)); e == nil || !strings.Contains(e.Error(), "TRUSTED_PROXY_CIDRS") {
		t.Fatalf("unexpected: %v", e)
	}
}
