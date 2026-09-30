package assets

import (
	"regexp"
	"strings"
	"testing"
)

var safeName = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)

const testHash = "a81f2c9e0b7d4c3f5a6e7d8c9b0a1f2e3d4c5b6a7f8e9d0c1b2a3f4e5d6c7b8a"

func TestFileNameSanitizesAndStaysDeterministic(t *testing.T) {
	cases := []struct{ in, want string }{
		{"Logo", "logo-a81f2c.svg"},
		{"Hero Image", "hero-image-a81f2c.svg"},
		{"Arrow  Right!!", "arrow-right-a81f2c.svg"},
		{"../../etc/passwd", "etc-passwd-a81f2c.svg"},
		{"foo/bar", "foo-bar-a81f2c.svg"},
		{`foo\bar`, "foo-bar-a81f2c.svg"},
		{"...", "vector-a81f2c.svg"},
		{"", "vector-a81f2c.svg"},
		{"   ", "vector-a81f2c.svg"},
		{"日本語 ロゴ", "vector-a81f2c.svg"},
		{"Logo 🚀 v2", "logo-v2-a81f2c.svg"},
		{"con", "con-a81f2c.svg"},
		{"a\x00b\nc", "a-b-c-a81f2c.svg"},
		{`"quoted" 'name' $(rm -rf) ; ls`, "quoted-name-rm-rf-ls-a81f2c.svg"},
		{"-leading-dash", "leading-dash-a81f2c.svg"},
		{strings.Repeat("a", 500), strings.Repeat("a", 40) + "-a81f2c.svg"},
	}
	for _, tc := range cases {
		got := fileName(tc.in, "vector", testHash, "svg", 6)
		if got != tc.want {
			t.Errorf("%q: got %q, want %q", tc.in, got, tc.want)
		}
		if !safeName.MatchString(got) {
			t.Errorf("%q: %q is not a safe workspace name", tc.in, got)
		}
		if again := fileName(tc.in, "vector", testHash, "svg", 6); again != got {
			t.Errorf("%q: not deterministic", tc.in)
		}
	}
}

func TestSameNameDifferentContentDoesNotCollide(t *testing.T) {
	other := "b12e3d4c5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0c1d2e3f4a5b6c7d8e9f0a1b2c"

	a, b := fileName("Icon", "vector", testHash, "svg", 6), fileName("Icon", "vector", other, "svg", 6)

	if a == b {
		t.Fatalf("both %q", a)
	}
}

func TestAssetIDDerivesFromContent(t *testing.T) {
	if assetID(testHash) != "asset_a81f2c9e0b7d4c3f" || assetID(testHash) != assetID(testHash) {
		t.Fatalf("id = %q", assetID(testHash))
	}
}
