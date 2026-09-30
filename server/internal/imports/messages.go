package imports

// messages are the only texts clients see for a stored code; provider detail never reaches them.

var messages = map[string]string{
	"INVALID_FIGMA_URL":           "Paste a Figma design link, for example https://www.figma.com/design/...",
	"INVALID_ID":                  "The project or import ID is not valid.",
	"INVALID_SELECTION":           "Choose one of the listed frames.",
	"IMPORT_NOT_FOUND":            "The import does not exist.",
	"IMPORT_CONFLICT":             "An import is already running for this project, or this one cannot be changed now.",
	"IMPORT_BUSY":                 "The importer is busy. Try again in a few seconds.",
	"FIGMA_AUTH_REQUIRED":         "Reconnect your Figma account and try again.",
	"FIGMA_PERMISSION_DENIED":     "You do not have permission to open this Figma file.",
	"FIGMA_FILE_NOT_FOUND":        "The Figma file was not found.",
	"FIGMA_NODE_NOT_FOUND":        "That frame was not found in the Figma file.",
	"FIGMA_UNSUPPORTED_NODE":      "That part of the file cannot be imported as a design. Choose a frame.",
	"FIGMA_NO_FRAMES":             "The file has no frames to import.",
	"FIGMA_RATE_LIMITED":          "Figma is limiting requests for your account. Wait a while before trying again.",
	"FIGMA_UNAVAILABLE":           "Figma is not responding. Try again shortly.",
	"FIGMA_REQUEST_TIMEOUT":       "Figma took too long to respond. Try again.",
	"FIGMA_IMPORT_FAILED":         "The import could not be completed.",
	"SNAPSHOT_TOO_LARGE":          "This design is too large to import.",
	"ASSET_TOO_LARGE":             "An image in this design is too large to import.",
	"ASSET_BUDGET_EXCEEDED":       "The images in this design are too large to import.",
	"ASSET_INVALID_CONTENT":       "An image in this design could not be read.",
	"ASSET_DOWNLOAD_FAILED":       "An image in this design could not be downloaded. Try again.",
	"ASSET_RESOLVE_FAILED":        "An image in this design could not be located. Try again.",
	"ASSET_URL_BLOCKED":           "An image in this design could not be fetched safely.",
	"IMPORT_TIMEOUT":              "The import took too long and was stopped.",
	"TOO_MANY_SCREENS":            "This selection has too many screens. Choose fewer.",
	"DESIGN_IR_LIMIT_EXCEEDED":    "This design is too large to import.",
	"DESIGN_IR_VALIDATION_FAILED": "The design could not be converted. Try again or choose fewer screens.",
	"DESIGN_IR_INVALID_INPUT":     "The design could not be converted. Try again.",
	"DESIGN_IR_ROOT_MISSING":      "The design has nothing to import.",
	"IMPORT_CANCELLED":            "The import was stopped.",
	"IMPORT_SUPERSEDED":           "A newer import replaced this one.",
	"IMPORT_INTERRUPTED":          "The import was interrupted. Start it again.",
	"WORKSPACE_FAILED":            "The import could not be prepared. Try again.",
	"INTERNAL_ERROR":              "An unexpected error occurred.",
}

// Message returns the public text for an import failure code.
func Message(code string) string {
	if m, ok := messages[code]; ok {
		return m
	}
	return messages["FIGMA_IMPORT_FAILED"]
}
