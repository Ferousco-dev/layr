package designapi

import "errors"

// Errors that map to public codes; none of them carry paths, SQL or provider text.
var (
	ErrProjectNotFound    = errors.New("project not found")
	ErrDesignNotFound     = errors.New("design not found")
	ErrNotReady           = errors.New("design not ready")
	ErrExpired            = errors.New("design data expired")
	ErrScreenNotFound     = errors.New("screen not found")
	ErrFlowNotFound       = errors.New("flow not found")
	ErrPreviewUnavailable = errors.New("preview not available")
	ErrAssetNotFound      = errors.New("asset not found")
	ErrInvalidDesign      = errors.New("stored design is invalid")
	ErrInvalidQuery       = errors.New("query invalid")
)

// ErrVersionUnavailable means the requested design version is not the design the project has now.
var ErrVersionUnavailable = errors.New("design version unavailable")
