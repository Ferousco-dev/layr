// Package designapi is the frontend-facing read layer: public DTOs, ownership-checked lookups, safe previews.
package designapi

// Public design statuses.
const (
	StatusReady             = "ready"
	StatusImporting         = "importing"
	StatusAwaitingSelection = "awaiting_selection"
	StatusFailed            = "failed"
	StatusExpired           = "expired"
)

// Design is the project's current design, or the state of the import that is producing it.
type Design struct {
	ProjectID     string         `json:"project_id"`
	ImportID      string         `json:"import_id"`
	DesignVersion string         `json:"design_version,omitempty"`
	Status        string         `json:"status"`
	Source        *Source        `json:"source,omitempty"`
	Counts        *Counts        `json:"counts,omitempty"`
	Screens       []Screen       `json:"screens"`
	Flows         []Flow         `json:"flows"`
	Warnings      []Warning      `json:"warnings"`
	Selection     Selection      `json:"selection"`
	DesignSystem  *DesignSystem  `json:"design_system,omitempty"`
	Error         *Error         `json:"error,omitempty"`
	PendingImport *PendingImport `json:"pending_import,omitempty"`
	// ScreensTruncated is true when Screens holds only the first part; use the screens endpoint for all.
	ScreensTruncated bool `json:"screens_truncated,omitempty"`
}

// Source describes where the design came from, without provider internals.
type Source struct {
	Provider      string `json:"provider"`
	FileName      string `json:"file_name,omitempty"`
	SourceVersion string `json:"source_version,omitempty"`
}

type Counts struct {
	Screens             int `json:"screens"`
	Flows               int `json:"flows"`
	UngroupedScreens    int `json:"ungrouped_screens"`
	Warnings            int `json:"warnings"`
	ScreensWithWarnings int `json:"screens_with_warnings"`
	SharedComponents    int `json:"shared_components"`
}

// Selection lists how the frontend may choose what to work on; screen IDs are the product identity.
type Selection struct {
	Modes []string `json:"modes"`
}

type DesignSystem struct {
	Colors     int `json:"colors"`
	TextStyles int `json:"text_styles"`
	Spacing    int `json:"spacing_values"`
	Radii      int `json:"radii"`
	Shadows    int `json:"shadows"`
}

type PendingImport struct {
	ImportID string `json:"import_id"`
	Status   string `json:"status"`
	Error    *Error `json:"error,omitempty"`
}

type Error struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Screen is the summary of one screen: enough to render a card, nothing of its content.
type Screen struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	SourceNodeID string  `json:"source_node_id"`
	Page         string  `json:"page,omitempty"`
	FlowID       string  `json:"flow_id,omitempty"`
	Index        int     `json:"index"`
	Width        float64 `json:"width"`
	Height       float64 `json:"height"`
	Preview      Preview `json:"preview"`
	WarningCount int     `json:"warning_count"`
}

// Preview points at the authenticated preview endpoint; it never holds a provider URL or a path.
type Preview struct {
	Available bool     `json:"available"`
	Width     *float64 `json:"width,omitempty"`
	Height    *float64 `json:"height,omitempty"`
	MediaType string   `json:"media_type,omitempty"`
	URL       string   `json:"url,omitempty"`
}

// Flow is a group of screens the designer grouped (a Figma section), in design order.
type Flow struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	SourceNodeID string   `json:"source_node_id"`
	ScreenIDs    []string `json:"screen_ids"`
}

type Warning struct {
	Code     string `json:"code"`
	ScreenID string `json:"screen_id,omitempty"`
	Message  string `json:"message"`
}

// ScreenDetail adds what a screen page needs, still without any part of the node tree.
type ScreenDetail struct {
	Screen
	FlowName      string         `json:"flow_name,omitempty"`
	NodeCount     int            `json:"node_count"`
	AssetCount    int            `json:"asset_count"`
	RootLayout    string         `json:"root_layout,omitempty"`
	Components    []ComponentRef `json:"components"`
	Warnings      []Warning      `json:"warnings"`
	DesignVersion string         `json:"design_version"`
}

// ComponentRef names a shared component a screen uses.
type ComponentRef struct {
	ID            string `json:"id"`
	Name          string `json:"name,omitempty"`
	InstanceCount int    `json:"instance_count"`
	Shared        bool   `json:"shared"`
}

// ScreenPage is one page of the screen list.
type ScreenPage struct {
	Screens []Screen `json:"screens"`
	Total   int      `json:"total"`
	Limit   int      `json:"limit"`
	Offset  int      `json:"offset"`
}

// FlowDetail is a flow with its screen summaries.
type FlowDetail struct {
	Flow
	Screens []Screen `json:"screens"`
}

// DesignTokens are the values the design uses, summarised for a project page.
type DesignTokens struct {
	Colors     []ColorSwatch `json:"colors"`
	Fonts      []FontFamily  `json:"fonts"`
	Typography []TextStyle   `json:"typography"`
	Spacing    []float64     `json:"spacing"`
	Radii      []float64     `json:"radii"`
	Shadows    []ShadowStyle `json:"shadows"`
}

// TextStyle is one distinct combination of font, weight, size and spacing used in the design.
type TextStyle struct {
	Family        string   `json:"family"`
	Weight        *float64 `json:"weight,omitempty"`
	Size          float64  `json:"size"`
	LineHeight    string   `json:"line_height,omitempty"`
	LetterSpacing *float64 `json:"letter_spacing,omitempty"`
	Count         int      `json:"count"`
}

// ShadowStyle is one distinct shadow the design uses.
type ShadowStyle struct {
	Type   string  `json:"type"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Blur   float64 `json:"blur"`
	Spread float64 `json:"spread"`
	Hex    string  `json:"hex,omitempty"`
	Alpha  float64 `json:"alpha"`
	Count  int     `json:"count"`
}

// AssetItem is one image or vector Figma provided for the design, ready to show.
type AssetItem struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Kind      string   `json:"kind"`
	Format    string   `json:"format"`
	MediaType string   `json:"media_type"`
	Width     *float64 `json:"width,omitempty"`
	Height    *float64 `json:"height,omitempty"`
	SizeBytes int64    `json:"size_bytes"`
	ScreenIDs []string `json:"screen_ids"`
	URL       string   `json:"url"`
}

// ColorSwatch is one colour and how many times the design uses it.
type ColorSwatch struct {
	Hex   string  `json:"hex"`
	Alpha float64 `json:"alpha"`
	Count int     `json:"count"`
}

// FontFamily is one typeface with the weights the design uses.
type FontFamily struct {
	Family  string `json:"family"`
	Weights []int  `json:"weights"`
	Sizes   []int  `json:"sizes"`
	Count   int    `json:"count"`
}
