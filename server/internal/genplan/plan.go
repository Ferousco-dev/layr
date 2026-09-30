// Package genplan turns a Design IR and a screen selection into a validated, immutable generation plan.
package genplan

import "fmt"

// SchemaVersion is the version of the plan document.
const SchemaVersion = 1

// Selection modes.
const (
	ModeOne      = "one"
	ModeSelected = "selected"
	ModeFlow     = "flow"
	ModeAll      = "all"
)

// Unit types; each one exists only because the Design IR can justify it.
const (
	UnitFoundation       = "foundation"
	UnitDesignFoundation = "design_foundation"
	UnitComponent        = "component"
	UnitScreen           = "screen"
	UnitIntegration      = "integration"
	UnitVerification     = "verification"
)

// unitRank orders unit types the way work flows.
var unitRank = map[string]int{
	UnitFoundation: 0, UnitDesignFoundation: 1, UnitComponent: 2, UnitScreen: 3, UnitIntegration: 4, UnitVerification: 5,
}

// Plan statuses.
const (
	StatusPlanned = "planned"
	StatusInvalid = "invalid"
)

// Supported target.
const (
	FrameworkNextJS    = "nextjs"
	LanguageTypeScript = "typescript"
)

// Selection is what the person asks for; the server decides everything else.
type Selection struct {
	Mode      string   `json:"mode"`
	ScreenID  string   `json:"screen_id,omitempty"`
	ScreenIDs []string `json:"screen_ids,omitempty"`
	FlowID    string   `json:"flow_id,omitempty"`
}

// Target is the code the plan will be turned into.
type Target struct {
	Framework string `json:"framework"`
	Language  string `json:"language"`
}

// PlanSelection records what a selection resolved to, in design order.
type PlanSelection struct {
	Mode      string   `json:"mode"`
	FlowID    string   `json:"flow_id,omitempty"`
	ScreenIDs []string `json:"screen_ids"`
}

// IRRef points at a node of the pinned Design IR instead of copying it.
type IRRef struct {
	ScreenID string `json:"screen_id"`
	NodeID   string `json:"node_id"`
	Origin   string `json:"origin"`
}

// Reference identifies a screen's reference render logically; it is never a path or a URL.
type Reference struct {
	ID     string   `json:"id"`
	SHA256 string   `json:"sha256,omitempty"`
	Width  *float64 `json:"width,omitempty"`
	Height *float64 `json:"height,omitempty"`
}

// TokenCounts says how much shared design data the design foundation covers.
type TokenCounts struct {
	Colors     int `json:"colors"`
	Typography int `json:"typography"`
	Spacing    int `json:"spacing"`
	Radii      int `json:"radii"`
	Shadows    int `json:"shadows"`
	Styles     int `json:"styles"`
}

// Unit is one piece of future work.
type Unit struct {
	ID           string       `json:"id"`
	Type         string       `json:"type"`
	Name         string       `json:"name"`
	ScreenID     string       `json:"screen_id,omitempty"`
	ComponentID  string       `json:"component_id,omitempty"`
	SetID        string       `json:"set_id,omitempty"`
	IR           *IRRef       `json:"ir,omitempty"`
	ComponentIDs []string     `json:"component_ids,omitempty"`
	AssetIDs     []string     `json:"asset_ids,omitempty"`
	Reference    *Reference   `json:"reference,omitempty"`
	Tokens       *TokenCounts `json:"tokens,omitempty"`
}

// Edge says Unit cannot start until DependsOn is finished.
type Edge struct {
	Unit      string `json:"unit"`
	DependsOn string `json:"depends_on"`
}

// AssetRef is an asset the plan needs; identity and checksum only, never a path.
type AssetRef struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Format    string `json:"format"`
	MediaType string `json:"media_type"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
}

// Warning is a plan-level note that did not stop planning.
type Warning struct {
	Code     string `json:"code"`
	ScreenID string `json:"screen_id,omitempty"`
	Message  string `json:"message"`
}

// Summary holds structural counts only; it says nothing about cost.
type Summary struct {
	Screens          int `json:"screens"`
	SharedComponents int `json:"shared_components"`
	Assets           int `json:"assets"`
	Units            int `json:"units"`
	Dependencies     int `json:"dependencies"`
	Stages           int `json:"stages"`
	MaxParallel      int `json:"max_parallel"`
}

// Plan is the generation plan v1. It is immutable once created: a different choice makes a new plan.
type Plan struct {
	SchemaVersion int           `json:"schema_version"`
	ID            string        `json:"id,omitempty"`
	ProjectID     string        `json:"project_id,omitempty"`
	DesignVersion string        `json:"design_version"`
	Fingerprint   string        `json:"fingerprint"`
	Selection     PlanSelection `json:"selection"`
	Target        Target        `json:"target"`
	Units         []Unit        `json:"units"`
	Dependencies  []Edge        `json:"dependencies"`
	Order         []string      `json:"order"`
	Stages        [][]string    `json:"stages"`
	Assets        []AssetRef    `json:"assets"`
	Warnings      []Warning     `json:"warnings"`
	Summary       Summary       `json:"summary"`
}

// Request is what the planner needs from a caller.
type Request struct {
	DesignVersion string
	Selection     Selection
	Target        Target
}

// Limits bound how much work and output one plan may take.
type Limits struct {
	MaxUnits     int
	MaxWork      int
	MaxPlanBytes int
}

// DefaultLimits are generous for professional files and stop pathological input.
var DefaultLimits = Limits{MaxUnits: 20000, MaxWork: 4_000_000, MaxPlanBytes: 16 << 20}

func (l Limits) withDefaults() Limits {
	if l.MaxUnits <= 0 {
		l.MaxUnits = DefaultLimits.MaxUnits
	}
	if l.MaxWork <= 0 {
		l.MaxWork = DefaultLimits.MaxWork
	}
	if l.MaxPlanBytes <= 0 {
		l.MaxPlanBytes = DefaultLimits.MaxPlanBytes
	}
	return l
}

// UnitID builds the stable identifier of a unit; anything unsafe in the source ID is replaced by a hash.
func UnitID(kind, sourceID string) string {
	return kind + "_" + safeID(sourceID)
}

func (p *Plan) String() string {
	return fmt.Sprintf("plan(%s, %d units)", p.DesignVersion, len(p.Units))
}
