package genplan

import (
	"go/build"
	"strings"
	"testing"
)

// Planning reads the stored Design IR only: no Figma, no importer, no sign-in and no AI provider.
func TestPlannerNeverDependsOnFigmaOrProviders(t *testing.T) {
	for _, dir := range []string{".", "pgstore"} {
		pkg, err := build.ImportDir(dir, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range pkg.Imports {
			for _, banned := range []string{"/internal/figma", "/internal/assets", "/internal/auth", "/internal/account", "/internal/credential", "/internal/generation", "/internal/imports"} {
				if strings.HasSuffix(imp, banned) {
					t.Errorf("%s imports %s", pkg.ImportPath, imp)
				}
			}
		}
	}
}
