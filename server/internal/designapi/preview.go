package designapi

import (
	"bytes"
	"context"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/ferousco-dev/layr/server/internal/workspace"
)

// referencePath accepts exactly "reference/<safe-name>.png"; nothing else can name a preview file.
var referencePath = regexp.MustCompile(`^reference/([a-z0-9][a-z0-9._-]{0,58}\.png)$`)

// previewFile returns the file name inside reference/ for a stored path, or "" if it is not one.
func previewFile(path string) string {
	m := referencePath.FindStringSubmatch(path)
	if m == nil || strings.Contains(m[1], "..") {
		return ""
	}
	return m[1]
}

var pngSignature = []byte("\x89PNG\r\n\x1a\n")

// PreviewFile is an opened reference image ready to stream. The caller must Close it.
type PreviewFile struct {
	File          *os.File
	Size          int64
	ETag          string
	MediaType     string
	DesignVersion string
}

func (p *PreviewFile) Close() error { return p.File.Close() }

// Preview resolves a screen to its stored reference image after the ownership chain is proven.
func (s *Service) Preview(ctx context.Context, userID, projectID, screenID string) (*PreviewFile, error) {
	st, ix, err := s.ready(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	i, ok := ix.screen(screenID)
	if !ok {
		return nil, ErrScreenNotFound
	}
	ref := ix.ir.Screens[i].Reference
	if ref == nil {
		return nil, ErrPreviewUnavailable
	}
	name := previewFile(ref.Path)
	if name == "" {
		return nil, ErrPreviewUnavailable
	}
	// The workspace comes from the import row the caller was authorised for, never from the file.
	dir, err := s.ws.Existing(st.current.ID)
	if err != nil {
		return nil, missingWorkspace(err)
	}
	f, info, err := dir.OpenRegular(workspace.Reference, name)
	if err != nil {
		return nil, ErrPreviewUnavailable
	}
	if !isPNG(f) {
		_ = f.Close()
		return nil, ErrPreviewUnavailable
	}

	etag := `"` + ref.SHA256 + `"`
	if ref.SHA256 == "" {
		etag = `"` + info.ModTime().UTC().Format("20060102150405.000000000") + `"`
	}
	return &PreviewFile{File: f, Size: info.Size(), ETag: etag, MediaType: "image/png", DesignVersion: ix.version}, nil
}

func missingWorkspace(err error) error {
	if os.IsNotExist(err) {
		return ErrExpired
	}
	return ErrPreviewUnavailable
}

// isPNG checks the signature and rewinds, so a file renamed to .png is never served as an image.
func isPNG(f *os.File) bool {
	head := make([]byte, len(pngSignature))
	if _, err := io.ReadFull(f, head); err != nil || !bytes.Equal(head, pngSignature) {
		return false
	}
	_, err := f.Seek(0, io.SeekStart)
	return err == nil
}
