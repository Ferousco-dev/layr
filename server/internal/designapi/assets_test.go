package designapi_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ferousco-dev/layr/server/internal/designapi"
	"github.com/ferousco-dev/layr/server/internal/designapi/designtest"
	"github.com/ferousco-dev/layr/server/internal/designir"
)

func assetDesign(t *testing.T) *designtest.Env {
	t.Helper()
	w, h := 342.5, 492.0
	ir := designtest.BuildIR(3, false)
	ir.Assets = []designir.Asset{
		{ID: "asset_00000000000000b2", Kind: "svg", Format: "svg", MediaType: "image/svg+xml", Path: "assets/logo-mark-d4e5f6.svg", SizeBytes: 12, SHA256: strings.Repeat("b", 64), ScreenIDs: []string{designtest.ScreenID(1)}},
		{ID: "asset_00000000000000a1", Kind: "image", Format: "png", MediaType: "image/png", Path: "assets/hero-a1b2c3.png", SizeBytes: 8, SHA256: strings.Repeat("a", 64), Width: &w, Height: &h, ScreenIDs: []string{designtest.ScreenID(1), designtest.ScreenID(2)}},
		{ID: "asset_00000000000000c3", Kind: "image", Format: "pdf", MediaType: "application/pdf", Path: "assets/paper-aaaaaa.pdf", SizeBytes: 3, SHA256: strings.Repeat("c", 64), ScreenIDs: []string{}},
		{ID: "asset_00000000000000d4", Kind: "image", Format: "png", MediaType: "image/png", Path: "assets/missing-eeeeee.png", SizeBytes: 3, SHA256: strings.Repeat("d", 64), ScreenIDs: []string{}},
		{ID: "asset_00000000000000e5", Kind: "image", Format: "png", MediaType: "image/png", Path: "assets/linked-ffffff.png", SizeBytes: 3, SHA256: strings.Repeat("e", 64), ScreenIDs: []string{}},
	}
	e := designtest.NewWith(t, ir)
	e.WriteAsset("hero-a1b2c3.png", []byte("PNGDATA!"))
	e.WriteAsset("logo-mark-d4e5f6.svg", []byte("<svg></svg>"))
	outside := filepath.Join(t.TempDir(), "secret.png")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(e.Dir.Path(), "assets", "linked-ffffff.png")); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestAssetsListsEverythingViewableWithReadableNames(t *testing.T) {
	e := assetDesign(t)
	got, err := e.Service(100).Assets(ctx, designtest.OwnerA, designtest.ProjectA)
	if err != nil {
		t.Fatal(err)
	}
	names := []string{}
	for _, a := range got {
		names = append(names, a.Kind+":"+a.Name)
	}
	if strings.Join(names, ",") != "image:hero,image:linked,image:missing,svg:logo mark" {
		t.Fatalf("assets = %v (a PDF must not be listed; names drop the content tag)", names)
	}
	hero := got[0]
	if hero.Width == nil || *hero.Width != 342.5 || len(hero.ScreenIDs) != 2 || hero.SizeBytes != 8 || !strings.Contains(hero.URL, "/design/assets/asset_00000000000000a1/file?v=dv_") {
		t.Fatalf("hero = %+v", hero)
	}
	for _, a := range got {
		if a.ScreenIDs == nil {
			t.Fatalf("screen ids must be a list, not null: %+v", a)
		}
	}
}

func TestAssetFilesStreamOnlyFromTheOwnersWorkspace(t *testing.T) {
	e := assetDesign(t)
	svc := e.Service(100)

	f, err := svc.Asset(ctx, designtest.OwnerA, designtest.ProjectA, "asset_00000000000000a1")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	data, _ := io.ReadAll(f.File)
	if string(data) != "PNGDATA!" || f.MediaType != "image/png" || f.ETag != `"`+strings.Repeat("a", 64)+`"` || f.Size != 8 {
		t.Fatalf("file = %q %+v", data, f)
	}

	if _, err := svc.Asset(ctx, designtest.OwnerB, designtest.ProjectA, "asset_00000000000000a1"); !errors.Is(err, designapi.ErrProjectNotFound) {
		t.Fatalf("a stranger: %v", err)
	}
	for name, id := range map[string]string{"unknown": "asset_ffffffffffffffff", "not viewable": "asset_00000000000000c3", "file missing": "asset_00000000000000d4", "symlink out of the workspace": "asset_00000000000000e5"} {
		if _, err := svc.Asset(ctx, designtest.OwnerA, designtest.ProjectA, id); !errors.Is(err, designapi.ErrAssetNotFound) {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestTokensIncludeEveryTextStyleAndShadow(t *testing.T) {
	weight, tracking, lh := 700.0, -0.5, 48.0
	ir := designtest.BuildIR(2, false)
	ir.Tokens = designir.Tokens{
		Typography: []designir.TypographyToken{
			{ID: "t1", FontFamily: "Inter", FontWeight: &weight, FontSize: 32, LineHeight: &designir.LineHeight{Mode: "px", Value: &lh}, LetterSpacing: &tracking, Count: 3},
			{ID: "t2", FontFamily: "Inter", FontSize: 14, LineHeight: &designir.LineHeight{Mode: "auto"}, Count: 9},
		},
		Shadows: []designir.ShadowToken{{ID: "s1", Type: "drop", OffsetX: 0, OffsetY: 4, Radius: 8, Spread: 1, Color: &designir.Color{R: 0, G: 0, B: 0, A: 0.25}, Count: 2}},
	}
	e := designtest.NewWith(t, ir)
	got, err := e.Service(100).Tokens(ctx, designtest.OwnerA, designtest.ProjectA)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Typography) != 2 || got.Typography[0].Size != 14 || got.Typography[0].LineHeight != "auto" || got.Typography[1].LineHeight != "48px" || *got.Typography[1].LetterSpacing != -0.5 {
		t.Fatalf("typography = %+v", got.Typography)
	}
	sh := got.Shadows[0]
	if len(got.Shadows) != 1 || sh.Y != 4 || sh.Blur != 8 || sh.Hex != "#000000" || sh.Alpha != 0.25 {
		t.Fatalf("shadows = %+v", got.Shadows)
	}
}
