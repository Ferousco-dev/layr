package designapi

import (
	"context"
	"os"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/ferousco-dev/layr/server/internal/workspace"
)

// assetPath accepts exactly "assets/<safe-name>"; nothing else can name an asset file.
var assetPath = regexp.MustCompile(`^assets/([a-z0-9][a-z0-9._-]{0,62})$`)

// viewableMedia are the only types an asset may be served as.
var viewableMedia = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true, "image/svg+xml": true}

// hashSuffix removes the "-abc123" content tag from a stored file name.
var hashSuffix = regexp.MustCompile(`-[0-9a-f]{6,8}$`)

func assetName(p string) string {
	base := path.Base(p)
	base = strings.TrimSuffix(base, path.Ext(base))
	base = hashSuffix.ReplaceAllString(base, "")
	return strings.NewReplacer("-", " ", "_", " ").Replace(base)
}

// Assets lists every image and vector that Figma provided for the current design.
func (s *Service) Assets(ctx context.Context, userID, projectID string) ([]AssetItem, error) {
	st, ix, err := s.ready(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]AssetItem, 0, len(ix.ir.Assets))
	for _, a := range ix.ir.Assets {
		if !viewableMedia[a.MediaType] || assetPath.FindStringSubmatch(a.Path) == nil {
			continue
		}
		ids := a.ScreenIDs
		if ids == nil {
			ids = []string{}
		}
		out = append(out, AssetItem{
			ID: a.ID, Name: assetName(a.Path), Kind: a.Kind, Format: a.Format, MediaType: a.MediaType, Width: a.Width, Height: a.Height,
			SizeBytes: a.SizeBytes, ScreenIDs: ids,
			URL: "/api/v1/projects/" + st.projectID + "/design/assets/" + a.ID + "/file?v=" + ix.version,
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

// AssetFile is an opened asset ready to stream. The caller must Close it.
type AssetFile struct {
	File          *os.File
	Size          int64
	ETag          string
	MediaType     string
	DesignVersion string
}

func (a *AssetFile) Close() error { return a.File.Close() }

// Asset opens one stored asset after the ownership chain is proven; the path always comes from the stored design.
func (s *Service) Asset(ctx context.Context, userID, projectID, assetID string) (*AssetFile, error) {
	st, ix, err := s.ready(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	for _, a := range ix.ir.Assets {
		if a.ID != assetID {
			continue
		}
		m := assetPath.FindStringSubmatch(a.Path)
		if m == nil || !viewableMedia[a.MediaType] {
			return nil, ErrAssetNotFound
		}
		dir, err := s.ws.Existing(st.current.ID)
		if err != nil {
			return nil, missingWorkspace(err)
		}
		f, info, err := dir.OpenRegular(workspace.Assets, m[1])
		if err != nil {
			return nil, ErrAssetNotFound
		}
		return &AssetFile{File: f, Size: info.Size(), ETag: `"` + a.SHA256 + `"`, MediaType: a.MediaType, DesignVersion: ix.version}, nil
	}
	return nil, ErrAssetNotFound
}
