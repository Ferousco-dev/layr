package designapi

import (
	"go/build"
	"strings"
	"testing"
)

func TestDesignAPINeverDependsOnTheFigmaClientOrTheImporterWorkers(t *testing.T) {
	pkg, err := build.ImportDir(".", 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, imp := range pkg.Imports {
		if strings.HasSuffix(imp, "/internal/figma") || strings.HasSuffix(imp, "/internal/assets") || strings.HasSuffix(imp, "/internal/auth") {
			t.Errorf("designapi imports %s; reading a design must need no Figma access", imp)
		}
	}
}
