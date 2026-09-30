package assets

import "errors"

// Stable codes stored on a failed import.
const (
	CodeDownloadFailed = "ASSET_DOWNLOAD_FAILED"
	CodeInvalidContent = "ASSET_INVALID_CONTENT"
	CodeTooLarge       = "ASSET_TOO_LARGE"
	CodeBudgetExceeded = "ASSET_BUDGET_EXCEEDED"
	CodeURLBlocked     = "ASSET_URL_BLOCKED"
	CodeResolveFailed  = "ASSET_RESOLVE_FAILED"
)

var (
	ErrTooLarge       = errors.New("asset exceeds the size limit")
	ErrBudgetExceeded = errors.New("import asset budget exceeded")
	ErrURLBlocked     = errors.New("asset url is not allowed")
	ErrDownload       = errors.New("asset download failed")
	// ErrExpired means the provider URL was rejected in a way that suggests it expired.
	ErrExpired = errors.New("asset url rejected")
)

// Error is a fatal asset failure. It never carries a URL, because Figma URLs are signed.
type Error struct {
	Code   string
	NodeID string
	Err    error
}

func (e *Error) Error() string {
	msg := "asset: " + e.Code
	if e.NodeID != "" {
		msg += " (node " + e.NodeID + ")"
	}
	return msg
}

func (e *Error) Unwrap() error { return e.Err }

// CodeOf returns the asset failure code of err, or empty if it is not an asset error.
func CodeOf(err error) string {
	var ae *Error
	if errors.As(err, &ae) {
		return ae.Code
	}
	return ""
}

// Warning is a non-fatal finding recorded in the manifest.
type Warning struct {
	Code     string `json:"code"`
	NodeID   string `json:"node_id,omitempty"`
	ImageRef string `json:"image_ref,omitempty"`
	Message  string `json:"message"`
}

const (
	WarnVideoUnsupported   = "video_paint_unsupported"
	WarnPatternUnsupported = "pattern_paint_unsupported"
	WarnAnimatedImage      = "animated_image_static_frame"
	WarnImageUnresolved    = "image_ref_unresolved"
	WarnVectorUnresolved   = "vector_export_unresolved"
	WarnSVGSanitized       = "svg_sanitized"
	WarnTooManyAssets      = "asset_limit_reached"
)
