package assets

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ferousco-dev/layr/server/internal/workspace"
)

func newDir(t *testing.T) *workspace.Dir {
	t.Helper()
	m, err := workspace.NewManager(t.TempDir(), 1<<20, 1<<22)
	if err != nil {
		t.Fatal(err)
	}
	d, err := m.Create(importID)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func sampleManifest() *Manifest {
	w, h := 4.0, 2.0
	return &Manifest{
		ImportID: importID, Source: ManifestSource{FileKey: "K", NodeID: "1:1"},
		Assets: []Asset{
			{ID: "asset_b", Kind: "svg", Format: "svg", MediaType: "image/svg+xml", Path: "assets/z-logo-bbbbbb.svg", SHA256: "b", Sources: []Source{{NodeID: "9:9", Role: "export"}, {NodeID: "1:2", Role: "export"}}},
			{ID: "asset_a", Kind: "image", Format: "png", MediaType: "image/png", Path: "assets/a-hero-aaaaaa.png", SHA256: "a", Width: &w, Height: &h,
				Sources: []Source{{NodeID: "2:2", ImageRef: "ref1", Role: "fill"}, {NodeID: "2:1", ImageRef: "ref1", Role: "fill"}}},
		},
		Warnings: []Warning{{Code: "b_warn", Message: "two"}, {Code: "a_warn", Message: "one"}},
	}
}

func TestManifestSaveIsSortedVersionedAndRoundTrips(t *testing.T) {
	dir := newDir(t)
	m := sampleManifest()

	if err := m.Save(dir); err != nil {
		t.Fatal(err)
	}
	raw, _ := dir.ReadFile(workspace.Assets, "manifest.json")

	var back Manifest
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatal(err)
	}
	if back.SchemaVersion != 1 || back.Assets[0].Path != "assets/a-hero-aaaaaa.png" || back.Assets[1].Path != "assets/z-logo-bbbbbb.svg" {
		t.Fatalf("assets not sorted by path: %+v", back.Assets)
	}
	if back.Assets[0].Sources[0].NodeID != "2:1" || back.Assets[1].Sources[0].NodeID != "1:2" || back.Warnings[0].Code != "a_warn" {
		t.Fatalf("sources or warnings not sorted: %+v", back)
	}
	loaded, err := LoadManifest(dir)
	if err != nil || loaded.ImportID != importID || len(loaded.Assets) != 2 {
		t.Fatalf("load = %+v, %v", loaded, err)
	}
	if strings.Contains(string(raw), `"width": null`) {
		t.Fatal("unknown dimensions must be omitted, not invented")
	}
}

func TestManifestOutputDoesNotDependOnInputOrder(t *testing.T) {
	a, b := sampleManifest(), sampleManifest()
	b.Assets[0], b.Assets[1] = b.Assets[1], b.Assets[0]
	b.Warnings[0], b.Warnings[1] = b.Warnings[1], b.Warnings[0]
	da, db := newDir(t), newDir(t)

	_ = a.Save(da)
	_ = b.Save(db)

	ra, _ := da.ReadFile(workspace.Assets, "manifest.json")
	rb, _ := db.ReadFile(workspace.Assets, "manifest.json")
	if string(ra) != string(rb) {
		t.Fatal("input order changed the file")
	}
}

func TestManifestLookups(t *testing.T) {
	m := sampleManifest()

	if a, ok := m.AssetByID("asset_a"); !ok || a.Format != "png" {
		t.Fatal("AssetByID")
	}
	if _, ok := m.AssetByID("missing"); ok {
		t.Fatal("AssetByID found a ghost")
	}
	if a, ok := m.AssetForImageRef("ref1"); !ok || a.ID != "asset_a" {
		t.Fatal("AssetForImageRef")
	}
	if got := m.AssetsForNode("9:9"); len(got) != 1 || got[0].ID != "asset_b" {
		t.Fatalf("AssetsForNode = %+v", got)
	}
	if got := m.AssetsForNode("nope"); len(got) != 0 {
		t.Fatal("AssetsForNode found a ghost")
	}
}

func TestLoadManifestRejectsUnknownVersionsAndGarbage(t *testing.T) {
	dir := newDir(t)
	if _, err := LoadManifest(dir); err == nil {
		t.Fatal("missing manifest loaded")
	}
	_ = dir.WriteBytes(workspace.Assets, "manifest.json", []byte(`{"schema_version": 99}`))
	if _, err := LoadManifest(dir); err == nil {
		t.Fatal("future schema accepted")
	}
	_ = dir.WriteBytes(workspace.Assets, "manifest.json", []byte(`{not json`))
	if _, err := LoadManifest(dir); err == nil {
		t.Fatal("garbage accepted")
	}
}

func TestVerifyStoredDetectsAnyChange(t *testing.T) {
	dir := newDir(t)
	content := []byte("payload")
	_ = dir.WriteBytes(workspace.Assets, "x-111111.png", content)
	asset := Asset{Path: "assets/x-111111.png", SizeBytes: int64(len(content)), SHA256: sumHex(content)}

	if !verifyStored(dir, asset) {
		t.Fatal("intact file rejected")
	}
	same := asset
	same.SHA256 = sumHex([]byte("other"))
	wrongSize := asset
	wrongSize.SizeBytes++
	if verifyStored(dir, same) || verifyStored(dir, wrongSize) {
		t.Fatal("changed metadata accepted")
	}
	_ = os.WriteFile(filepath.Join(dir.Path(), "assets", "x-111111.png"), []byte("payloaD"), 0o600)
	if verifyStored(dir, asset) {
		t.Fatal("tampered bytes accepted")
	}
	for _, bad := range []string{"assets/../../etc/passwd", "../assets/x.png", "assets/sub/x.png", "/abs/x.png", "assets/", `assets\x.png`} {
		if verifyStored(dir, Asset{Path: bad}) {
			t.Errorf("%q accepted", bad)
		}
	}
}
