package assets

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"sort"

	"github.com/ferousco-dev/layr/server/internal/workspace"
)

// SchemaVersion is the manifest contract version shared with the Design IR and code generation.
const SchemaVersion = 1

const manifestName = "manifest.json"

// Source ties an asset to the design node that uses it. Identity lives in the asset, usage here.
type Source struct {
	NodeID       string  `json:"node_id"`
	ImageRef     string  `json:"image_ref,omitempty"`
	Role         string  `json:"role"`
	ScaleMode    string  `json:"scale_mode,omitempty"`
	Rotation     float64 `json:"rotation,omitempty"`
	HasTransform bool    `json:"has_image_transform,omitempty"`
}

// Asset is one stored file. Path is relative to the workspace, never a host path.
type Asset struct {
	ID        string   `json:"id"`
	Kind      string   `json:"kind"`
	Format    string   `json:"format"`
	MediaType string   `json:"media_type"`
	Path      string   `json:"path"`
	SizeBytes int64    `json:"size_bytes"`
	SHA256    string   `json:"sha256"`
	Width     *float64 `json:"width,omitempty"`
	Height    *float64 `json:"height,omitempty"`
	Sanitized bool     `json:"sanitized,omitempty"`
	Sources   []Source `json:"sources"`
}

// Reference is the Figma render of one imported screen, kept apart from website assets.
type Reference struct {
	NodeID    string   `json:"node_id"`
	Path      string   `json:"path"`
	MediaType string   `json:"media_type"`
	SizeBytes int64    `json:"size_bytes"`
	SHA256    string   `json:"sha256"`
	Width     *float64 `json:"width,omitempty"`
	Height    *float64 `json:"height,omitempty"`
}

// ManifestSource names the file and the imported nodes; NodeID is the first of NodeIDs.
type ManifestSource struct {
	FileKey string   `json:"file_key"`
	NodeID  string   `json:"node_id"`
	NodeIDs []string `json:"node_ids"`
}

type Manifest struct {
	SchemaVersion int            `json:"schema_version"`
	ImportID      string         `json:"import_id"`
	Source        ManifestSource `json:"source"`
	Assets        []Asset        `json:"assets"`
	References    []Reference    `json:"references"`
	Warnings      []Warning      `json:"warnings"`
}

func (m *Manifest) AssetByID(id string) (Asset, bool) {
	for _, a := range m.Assets {
		if a.ID == id {
			return a, true
		}
	}
	return Asset{}, false
}

// AssetForImageRef finds the asset that an image fill's imageRef resolved to.
func (m *Manifest) AssetForImageRef(ref string) (Asset, bool) {
	for _, a := range m.Assets {
		for _, s := range a.Sources {
			if s.ImageRef == ref {
				return a, true
			}
		}
	}
	return Asset{}, false
}

// AssetsForNode returns every asset a node paints or exports.
func (m *Manifest) AssetsForNode(nodeID string) []Asset {
	var out []Asset
	for _, a := range m.Assets {
		for _, s := range a.Sources {
			if s.NodeID == nodeID {
				out = append(out, a)
				break
			}
		}
	}
	return out
}

// normalize sorts everything so the file does not depend on goroutine timing.
func (m *Manifest) normalize() {
	for i := range m.Assets {
		src := m.Assets[i].Sources
		sort.SliceStable(src, func(a, b int) bool {
			return src[a].NodeID+"|"+src[a].Role+"|"+src[a].ImageRef < src[b].NodeID+"|"+src[b].Role+"|"+src[b].ImageRef
		})
	}
	sort.SliceStable(m.Assets, func(a, b int) bool { return m.Assets[a].Path < m.Assets[b].Path })
	sort.SliceStable(m.Warnings, func(a, b int) bool {
		x, y := m.Warnings[a], m.Warnings[b]
		return x.Code+"|"+x.NodeID+"|"+x.ImageRef+"|"+x.Message < y.Code+"|"+y.NodeID+"|"+y.ImageRef+"|"+y.Message
	})
	sort.SliceStable(m.References, func(a, b int) bool { return m.References[a].NodeID < m.References[b].NodeID })
	if m.References == nil {
		m.References = []Reference{}
	}
	if m.Source.NodeIDs == nil {
		m.Source.NodeIDs = []string{}
	}
	if m.Assets == nil {
		m.Assets = []Asset{}
	}
	if m.Warnings == nil {
		m.Warnings = []Warning{}
	}
}

// Save writes the manifest atomically; it is only called after every listed file is stored.
func (m *Manifest) Save(dir *workspace.Dir) error {
	m.SchemaVersion = SchemaVersion
	m.normalize()
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(m); err != nil {
		return err
	}
	return dir.WriteBytes(workspace.Assets, manifestName, buf.Bytes())
}

// LoadManifest reads an earlier manifest; a missing or unreadable one simply means "start fresh".
func LoadManifest(dir *workspace.Dir) (*Manifest, error) {
	b, err := dir.ReadFile(workspace.Assets, manifestName)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	if m.SchemaVersion != SchemaVersion {
		return nil, errors.New("manifest schema version not supported")
	}
	return &m, nil
}

// verifyStored checks a listed file still exists with the recorded size and checksum.
func verifyStored(dir *workspace.Dir, a Asset) bool {
	name, ok := baseName(a.Path)
	if !ok {
		return false
	}
	f, err := dir.Open(workspace.Assets, name)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil || n != a.SizeBytes {
		return false
	}
	return hex.EncodeToString(h.Sum(nil)) == a.SHA256
}

// baseName extracts the file name from "assets/<name>", refusing anything else.
func baseName(path string) (string, bool) {
	const prefix = workspace.Assets + "/"
	if len(path) <= len(prefix) || path[:len(prefix)] != prefix {
		return "", false
	}
	name := path[len(prefix):]
	for _, c := range name {
		if c == '/' || c == '\\' {
			return "", false
		}
	}
	return name, true
}
