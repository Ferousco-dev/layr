// Package designir defines Design IR v1: Layr's framework-independent model of a design.
package designir

// SchemaVersion is bumped for any change that readers must handle.
const SchemaVersion = 1

// Node types. The vocabulary is small and says nothing about React, CSS or Figma.
const (
	TypeDocument     = "document"
	TypeCanvas       = "canvas"
	TypeSection      = "section"
	TypeFrame        = "frame"
	TypeGroup        = "group"
	TypeText         = "text"
	TypeImage        = "image"
	TypeVector       = "vector"
	TypeShape        = "shape"
	TypeComponent    = "component"
	TypeComponentSet = "component_set"
	TypeInstance     = "instance"
	TypeUnknown      = "unknown"
)

// DesignIR is a whole imported design: many screens sharing components, assets and tokens.
type DesignIR struct {
	SchemaVersion int            `json:"schema_version"`
	Source        Source         `json:"source"`
	Screens       []Screen       `json:"screens"`
	Sections      []Section      `json:"sections"`
	Components    []Component    `json:"components"`
	ComponentSets []ComponentSet `json:"component_sets"`
	Styles        []Style        `json:"styles"`
	Assets        []Asset        `json:"assets"`
	Tokens        Tokens         `json:"tokens"`
	Warnings      []Warning      `json:"warnings"`
	Stats         Stats          `json:"stats"`
}

// Source says where the design came from. It never holds credentials, signed URLs or host paths.
type Source struct {
	Provider string   `json:"provider"`
	FileKey  string   `json:"file_key"`
	FileName string   `json:"file_name,omitempty"`
	Version  string   `json:"version,omitempty"`
	ImportID string   `json:"import_id,omitempty"`
	NodeIDs  []string `json:"node_ids"`
}

// Screen is one page-sized design. Screens can be generated independently of each other.
type Screen struct {
	ID           string     `json:"id"`
	SourceNodeID string     `json:"source_node_id"`
	Name         string     `json:"name"`
	Page         string     `json:"page,omitempty"`
	SectionID    string     `json:"section_id,omitempty"`
	Width        float64    `json:"width"`
	Height       float64    `json:"height"`
	Reference    *Reference `json:"reference,omitempty"`
	ComponentIDs []string   `json:"component_ids"`
	AssetIDs     []string   `json:"asset_ids"`
	Root         Node       `json:"root"`
}

// Reference is the Figma render of a screen, kept for visual verification.
type Reference struct {
	Path   string   `json:"path"`
	Width  *float64 `json:"width,omitempty"`
	Height *float64 `json:"height,omitempty"`
	SHA256 string   `json:"sha256,omitempty"`
}

// Section groups screens that the designer grouped, such as a flow.
type Section struct {
	ID           string   `json:"id"`
	SourceNodeID string   `json:"source_node_id"`
	Name         string   `json:"name"`
	ScreenIDs    []string `json:"screen_ids"`
}

// Component is a reusable design shared by instances, possibly across screens.
type Component struct {
	ID          string `json:"id"`
	SourceID    string `json:"source_id"`
	Key         string `json:"key,omitempty"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	SetID       string `json:"set_id,omitempty"`
	Remote      bool   `json:"remote,omitempty"`
	// DefinitionScreenID and DefinitionNodeID locate the master; both are empty when it lives outside the imported screens.
	DefinitionScreenID string   `json:"definition_screen_id,omitempty"`
	DefinitionNodeID   string   `json:"definition_node_id,omitempty"`
	InstanceCount      int      `json:"instance_count"`
	ScreenIDs          []string `json:"screen_ids"`
}

type ComponentSet struct {
	ID       string `json:"id"`
	SourceID string `json:"source_id"`
	Key      string `json:"key,omitempty"`
	Name     string `json:"name"`
}

// Style is a named Figma style (color, text, effect or grid) that nodes can reference.
type Style struct {
	ID          string `json:"id"`
	SourceID    string `json:"source_id"`
	Key         string `json:"key,omitempty"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
}

// Asset describes a stored file by logical ID; paths are relative to the import workspace.
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
	ScreenIDs []string `json:"screen_ids"`
}

// Warning records something approximated, unsupported or missing. Code is stable.
type Warning struct {
	Code         string `json:"code"`
	SourceNodeID string `json:"source_node_id,omitempty"`
	ScreenID     string `json:"screen_id,omitempty"`
	Message      string `json:"message"`
}

type Stats struct {
	Screens    int `json:"screens"`
	Nodes      int `json:"nodes"`
	Components int `json:"components"`
	Assets     int `json:"assets"`
	Warnings   int `json:"warnings"`
}

// SourceRef ties a node back to the provider node it came from.
type SourceRef struct {
	Provider string `json:"provider"`
	NodeID   string `json:"node_id"`
	Type     string `json:"type"`
}

// Node is one layer of the design. Optional facts are omitted rather than guessed.
type Node struct {
	ID         string         `json:"id"`
	Source     SourceRef      `json:"source"`
	Name       string         `json:"name"`
	Type       string         `json:"type"`
	Shape      string         `json:"shape,omitempty"`
	Visible    bool           `json:"visible"`
	Geometry   Geometry       `json:"geometry"`
	Sizing     *Sizing        `json:"sizing,omitempty"`
	Position   *Position      `json:"position,omitempty"`
	Layout     *Layout        `json:"layout,omitempty"`
	Appearance *Appearance    `json:"appearance,omitempty"`
	Text       *Text          `json:"text,omitempty"`
	Asset      *AssetUse      `json:"asset,omitempty"`
	Component  *ComponentRole `json:"component,omitempty"`
	Instance   *Instance      `json:"instance,omitempty"`
	StyleRefs  []StyleRef     `json:"style_refs,omitempty"`
	// ClipChildren means content outside the node's bounds is hidden.
	ClipChildren bool `json:"clip_children,omitempty"`
	// Mask means this node masks its following siblings; masks are preserved, not applied.
	Mask bool `json:"mask,omitempty"`
	// CollapsedChildren counts descendants folded into a single exported vector asset.
	CollapsedChildren int    `json:"collapsed_children,omitempty"`
	Children          []Node `json:"children"`
}

// ComponentRole marks a node as a component master or a set.
type ComponentRole struct {
	ComponentID string `json:"component_id,omitempty"`
	SetID       string `json:"set_id,omitempty"`
}

// Instance links a node to the component it was made from, with what differs from the master.
type Instance struct {
	ComponentID       string             `json:"component_id,omitempty"`
	ComponentSourceID string             `json:"component_source_id"`
	Properties        []InstanceProperty `json:"properties,omitempty"`
	Overrides         []Override         `json:"overrides,omitempty"`
}

type InstanceProperty struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

type Override struct {
	SourceNodeID string   `json:"source_node_id"`
	Fields       []string `json:"fields"`
}

type StyleRef struct {
	Role    string `json:"role"`
	StyleID string `json:"style_id"`
}

// Geometry holds size and parent-relative position in design pixels.
type Geometry struct {
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	// X and Y are relative to the parent's top-left corner; zero for a screen root.
	X float64 `json:"x"`
	Y float64 `json:"y"`
	// Rotation is in degrees, counter-clockwise as Figma shows it; Width and Height are unrotated.
	Rotation float64 `json:"rotation,omitempty"`
	// Absolute is the axis-aligned bounds in the source design space, kept for visual checks.
	Absolute *Rect `json:"absolute,omitempty"`
}

type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

const (
	SizeFixed = "fixed"
	SizeHug   = "hug"
	SizeFill  = "fill"
)

// Sizing says how each axis gets its size.
type Sizing struct {
	Horizontal Axis `json:"horizontal"`
	Vertical   Axis `json:"vertical"`
}

type Axis struct {
	Mode string   `json:"mode"`
	Min  *float64 `json:"min,omitempty"`
	Max  *float64 `json:"max,omitempty"`
}

const (
	PositionFlow     = "flow"
	PositionAbsolute = "absolute"
)

// Position describes how a node sits inside its parent.
type Position struct {
	Mode        string       `json:"mode"`
	Constraints *Constraints `json:"constraints,omitempty"`
	// Align is the child's own cross-axis alignment when it differs from the parent's ("stretch").
	Align string `json:"align,omitempty"`
	// Grow is the share of free space along the parent's main axis.
	Grow float64 `json:"grow,omitempty"`
}

type Constraints struct {
	Horizontal string `json:"horizontal"`
	Vertical   string `json:"vertical"`
}

const (
	LayoutNone       = "none"
	LayoutHorizontal = "horizontal"
	LayoutVertical   = "vertical"
	LayoutGrid       = "grid"
)

// Layout describes how a container arranges its children.
type Layout struct {
	Mode      string   `json:"mode"`
	Gap       *float64 `json:"gap,omitempty"`
	CrossGap  *float64 `json:"cross_gap,omitempty"`
	Justify   string   `json:"justify,omitempty"`
	Align     string   `json:"align,omitempty"`
	Padding   *Padding `json:"padding,omitempty"`
	Wrap      bool     `json:"wrap,omitempty"`
	WrapAlign string   `json:"wrap_align,omitempty"`
}

type Padding struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

// Appearance holds everything that paints a node.
type Appearance struct {
	Opacity   *float64 `json:"opacity,omitempty"`
	BlendMode string   `json:"blend_mode,omitempty"`
	Fills     []Paint  `json:"fills,omitempty"`
	Stroke    *Stroke  `json:"stroke,omitempty"`
	Radius    *Radius  `json:"radius,omitempty"`
	Effects   []Effect `json:"effects,omitempty"`
}

// Color channels are 0 to 255, alpha is 0 to 1.
type Color struct {
	R int     `json:"r"`
	G int     `json:"g"`
	B int     `json:"b"`
	A float64 `json:"a"`
}

const (
	PaintSolid           = "solid"
	PaintLinearGradient  = "linear_gradient"
	PaintRadialGradient  = "radial_gradient"
	PaintAngularGradient = "angular_gradient"
	PaintDiamondGradient = "diamond_gradient"
	PaintImage           = "image"
	PaintVideo           = "video"
	PaintPattern         = "pattern"
	PaintUnknown         = "unknown"
)

type Paint struct {
	Type       string      `json:"type"`
	SourceType string      `json:"source_type,omitempty"`
	Visible    bool        `json:"visible"`
	Opacity    *float64    `json:"opacity,omitempty"`
	BlendMode  string      `json:"blend_mode,omitempty"`
	Color      *Color      `json:"color,omitempty"`
	Gradient   *Gradient   `json:"gradient,omitempty"`
	Image      *ImagePaint `json:"image,omitempty"`
}

type Gradient struct {
	Stops []Stop `json:"stops"`
	// Handles are the gradient's anchor points in the node's unit space (0 to 1), as provided.
	Handles []Point `json:"handles,omitempty"`
}

type Stop struct {
	Position float64 `json:"position"`
	Color    Color   `json:"color"`
}

type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// ImagePaint points at a stored asset; Missing means the asset could not be found.
type ImagePaint struct {
	AssetID       string      `json:"asset_id,omitempty"`
	Missing       bool        `json:"missing,omitempty"`
	ScaleMode     string      `json:"scale_mode,omitempty"`
	Transform     [][]float64 `json:"transform,omitempty"`
	Rotation      float64     `json:"rotation,omitempty"`
	ScalingFactor *float64    `json:"scaling_factor,omitempty"`
	Filters       []Filter    `json:"filters,omitempty"`
}

type Filter struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
}

type Stroke struct {
	Paints     []Paint   `json:"paints"`
	Weight     float64   `json:"weight"`
	Sides      *Padding  `json:"sides,omitempty"`
	Align      string    `json:"align,omitempty"`
	Dashes     []float64 `json:"dashes,omitempty"`
	Cap        string    `json:"cap,omitempty"`
	Join       string    `json:"join,omitempty"`
	MiterAngle float64   `json:"miter_angle,omitempty"`
}

type Radius struct {
	TopLeft     float64 `json:"top_left"`
	TopRight    float64 `json:"top_right"`
	BottomRight float64 `json:"bottom_right"`
	BottomLeft  float64 `json:"bottom_left"`
}

const (
	EffectDropShadow     = "drop_shadow"
	EffectInnerShadow    = "inner_shadow"
	EffectLayerBlur      = "layer_blur"
	EffectBackgroundBlur = "background_blur"
	EffectUnknown        = "unknown"
)

type Effect struct {
	Type       string  `json:"type"`
	SourceType string  `json:"source_type,omitempty"`
	Visible    bool    `json:"visible"`
	Radius     float64 `json:"radius,omitempty"`
	Spread     float64 `json:"spread,omitempty"`
	OffsetX    float64 `json:"offset_x,omitempty"`
	OffsetY    float64 `json:"offset_y,omitempty"`
	Color      *Color  `json:"color,omitempty"`
	BlendMode  string  `json:"blend_mode,omitempty"`
	ShowBehind bool    `json:"show_behind_node,omitempty"`
}

// Text is text content exactly as written, with its style and any per-range differences.
type Text struct {
	Characters string    `json:"characters"`
	Style      TextStyle `json:"style"`
	Runs       []Run     `json:"runs,omitempty"`
}

// Run overrides the base style for characters [Start, End), counted in Unicode code points.
type Run struct {
	Start int       `json:"start"`
	End   int       `json:"end"`
	Style TextStyle `json:"style"`
}

type TextStyle struct {
	FontFamily         string      `json:"font_family,omitempty"`
	FontPostScriptName string      `json:"font_postscript_name,omitempty"`
	FontWeight         *float64    `json:"font_weight,omitempty"`
	FontSize           *float64    `json:"font_size,omitempty"`
	Italic             *bool       `json:"italic,omitempty"`
	LineHeight         *LineHeight `json:"line_height,omitempty"`
	LetterSpacing      *float64    `json:"letter_spacing_px,omitempty"`
	Align              string      `json:"align,omitempty"`
	VerticalAlign      string      `json:"vertical_align,omitempty"`
	Case               string      `json:"case,omitempty"`
	Decoration         string      `json:"decoration,omitempty"`
	AutoResize         string      `json:"auto_resize,omitempty"`
	ParagraphSpacing   *float64    `json:"paragraph_spacing,omitempty"`
	ParagraphIndent    *float64    `json:"paragraph_indent,omitempty"`
	Hyperlink          *Hyperlink  `json:"hyperlink,omitempty"`
	Fills              []Paint     `json:"fills,omitempty"`
}

const (
	LineHeightAuto    = "auto"
	LineHeightPixels  = "pixels"
	LineHeightPercent = "percent"
)

// LineHeight keeps Figma's unit: auto, pixels, or percent of the font size.
type LineHeight struct {
	Mode  string   `json:"mode"`
	Value *float64 `json:"value,omitempty"`
}

type Hyperlink struct {
	Type   string `json:"type"`
	URL    string `json:"url,omitempty"`
	NodeID string `json:"node_id,omitempty"`
}

// AssetUse says a node is drawn by a stored asset (an exported SVG).
type AssetUse struct {
	AssetID string `json:"asset_id,omitempty"`
	Missing bool   `json:"missing,omitempty"`
	Role    string `json:"role"`
}

// Tokens are the recurring exact values found in the design, with how often each is used.
type Tokens struct {
	Colors     []ColorToken      `json:"colors"`
	Typography []TypographyToken `json:"typography"`
	Spacing    []NumberToken     `json:"spacing"`
	Radii      []NumberToken     `json:"radii"`
	Shadows    []ShadowToken     `json:"shadows"`
}

type ColorToken struct {
	ID    string `json:"id"`
	Value Color  `json:"value"`
	Count int    `json:"count"`
}

type TypographyToken struct {
	ID            string      `json:"id"`
	FontFamily    string      `json:"font_family"`
	FontWeight    *float64    `json:"font_weight,omitempty"`
	FontSize      float64     `json:"font_size"`
	LineHeight    *LineHeight `json:"line_height,omitempty"`
	LetterSpacing *float64    `json:"letter_spacing_px,omitempty"`
	Count         int         `json:"count"`
}

type NumberToken struct {
	ID    string  `json:"id"`
	Value float64 `json:"value"`
	Count int     `json:"count"`
}

type ShadowToken struct {
	ID      string  `json:"id"`
	Type    string  `json:"type"`
	OffsetX float64 `json:"offset_x"`
	OffsetY float64 `json:"offset_y"`
	Radius  float64 `json:"radius"`
	Spread  float64 `json:"spread"`
	Color   *Color  `json:"color,omitempty"`
	Count   int     `json:"count"`
}
