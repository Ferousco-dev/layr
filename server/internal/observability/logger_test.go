package observability

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestRedactsSensitiveAttributes(t *testing.T) {
	var b bytes.Buffer
	l := New(&b, slog.LevelInfo)
	l.Info("test", slog.String("Authorization", "Bearer seeded-secret"), slog.Group("nested", slog.String("session_token", "seeded-token")))
	if strings.Contains(b.String(), "seeded-") {
		t.Fatal("secret leaked")
	}
	var v map[string]any
	if json.Unmarshal(b.Bytes(), &v) != nil {
		t.Fatal("not JSON")
	}
}
