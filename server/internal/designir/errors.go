package designir

import (
	"fmt"
	"strings"
)

// Stable codes for design IR failures.
const (
	CodeInvalidInput = "DESIGN_IR_INVALID_INPUT"
	CodeRootMissing  = "DESIGN_IR_ROOT_MISSING"
	CodeLimit        = "DESIGN_IR_LIMIT_EXCEEDED"
	CodeValidation   = "DESIGN_IR_VALIDATION_FAILED"
	CodeSerialize    = "DESIGN_IR_SERIALIZATION_FAILED"
)

// Error is a typed design IR failure; Problems lists what was wrong, without design content.
type Error struct {
	Code     string
	Problems []string
}

func (e *Error) Error() string {
	if len(e.Problems) == 0 {
		return e.Code
	}
	shown := e.Problems
	if len(shown) > 5 {
		shown = shown[:5]
	}
	return fmt.Sprintf("%s: %s", e.Code, strings.Join(shown, "; "))
}

// CodeOf returns the design IR code of err, or empty.
func CodeOf(err error) string {
	if e, ok := err.(*Error); ok {
		return e.Code
	}
	return ""
}
