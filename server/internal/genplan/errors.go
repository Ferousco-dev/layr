package genplan

import (
	"errors"
	"strings"
)

// Stable codes; none of them carry design content, paths or provider text.
const (
	CodeInvalidSelection     = "INVALID_SELECTION"
	CodeScreenNotFound       = "SCREEN_NOT_FOUND"
	CodeFlowNotFound         = "FLOW_NOT_FOUND"
	CodeEmptySelection       = "EMPTY_SELECTION"
	CodeDesignNotReady       = "DESIGN_NOT_READY"
	CodeVersionUnavailable   = "DESIGN_VERSION_UNAVAILABLE"
	CodeTargetUnsupported    = "GENERATION_TARGET_UNSUPPORTED"
	CodeDependencyInvalid    = "GENERATION_DEPENDENCY_INVALID"
	CodeDependencyCycle      = "GENERATION_DEPENDENCY_CYCLE"
	CodePlanInvalid          = "GENERATION_PLAN_INVALID"
	CodePlanTooLarge         = "GENERATION_PLAN_TOO_LARGE"
	CodeProjectNotFound      = "PROJECT_NOT_FOUND"
	CodePlanNotFound         = "GENERATION_PLAN_NOT_FOUND"
	CodeInvalidDesignVersion = "INVALID_DESIGN_VERSION"
)

// Error is a typed planner failure. Detail names units or IDs only.
type Error struct {
	Code   string
	Detail string
}

func (e *Error) Error() string {
	if e.Detail == "" {
		return e.Code
	}
	return e.Code + ": " + e.Detail
}

func fail(code, detail string) *Error { return &Error{Code: code, Detail: detail} }

// CodeOf returns the planner code of err, or empty.
func CodeOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// ErrNotFound is returned by a store when no plan matches the owner, project and ID.
var ErrNotFound = errors.New("generation plan not found")

func joinIDs(ids []string) string {
	if len(ids) > 8 {
		ids = append(ids[:8:8], "...")
	}
	return strings.Join(ids, " -> ")
}
