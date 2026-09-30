package figma

import "encoding/json"

// Node types Layr recognises; Figma adds new ones over time, so Type stays a plain string.
const (
	NodeDocument         = "DOCUMENT"
	NodeCanvas           = "CANVAS"
	NodeFrame            = "FRAME"
	NodeGroup            = "GROUP"
	NodeSection          = "SECTION"
	NodeComponent        = "COMPONENT"
	NodeComponentSet     = "COMPONENT_SET"
	NodeInstance         = "INSTANCE"
	NodeText             = "TEXT"
	NodeRectangle        = "RECTANGLE"
	NodeEllipse          = "ELLIPSE"
	NodeLine             = "LINE"
	NodeVector           = "VECTOR"
	NodeBooleanOperation = "BOOLEAN_OPERATION"
	NodeStar             = "STAR"
	NodePolygon          = "REGULAR_POLYGON"
	NodeSlice            = "SLICE"
)

var knownNodeTypes = map[string]bool{
	NodeDocument: true, NodeCanvas: true, NodeFrame: true, NodeGroup: true, NodeSection: true,
	NodeComponent: true, NodeComponentSet: true, NodeInstance: true, NodeText: true,
	NodeRectangle: true, NodeEllipse: true, NodeLine: true, NodeVector: true,
	NodeBooleanOperation: true, NodeStar: true, NodePolygon: true, NodeSlice: true,
}

// Color channels are Figma's normalized 0 to 1 floats, kept unconverted.
type Color struct {
	R float64 `json:"r"`
	G float64 `json:"g"`
	B float64 `json:"b"`
	A float64 `json:"a"`
}

type Vector struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Rect struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

type LayoutConstraint struct {
	Vertical   string `json:"vertical"`
	Horizontal string `json:"horizontal"`
}

type ColorStop struct {
	Position float64 `json:"position"`
	Color    Color   `json:"color"`
}

// Paint covers solid, gradient, image and video fills; ImageRef links image fills to GetImageFills.
type Paint struct {
	Type                    string             `json:"type"`
	Visible                 *bool              `json:"visible,omitempty"`
	Opacity                 *float64           `json:"opacity,omitempty"`
	Color                   *Color             `json:"color,omitempty"`
	BlendMode               string             `json:"blendMode,omitempty"`
	GradientHandlePositions []Vector           `json:"gradientHandlePositions,omitempty"`
	GradientStops           []ColorStop        `json:"gradientStops,omitempty"`
	ScaleMode               string             `json:"scaleMode,omitempty"`
	ImageRef                string             `json:"imageRef,omitempty"`
	ImageTransform          [][]float64        `json:"imageTransform,omitempty"`
	ScalingFactor           *float64           `json:"scalingFactor,omitempty"`
	Rotation                float64            `json:"rotation,omitempty"`
	Filters                 map[string]float64 `json:"filters,omitempty"`
	GIFRef                  string             `json:"gifRef,omitempty"`
	// BoundVariables is kept raw: variable bindings are not modelled yet.
	BoundVariables json.RawMessage `json:"boundVariables,omitempty"`
}

func (p Paint) IsVisible() bool { return p.Visible == nil || *p.Visible }

type Effect struct {
	Type                 string          `json:"type"`
	Visible              *bool           `json:"visible,omitempty"`
	Radius               float64         `json:"radius,omitempty"`
	Color                *Color          `json:"color,omitempty"`
	BlendMode            string          `json:"blendMode,omitempty"`
	Offset               *Vector         `json:"offset,omitempty"`
	Spread               float64         `json:"spread,omitempty"`
	ShowShadowBehindNode bool            `json:"showShadowBehindNode,omitempty"`
	BoundVariables       json.RawMessage `json:"boundVariables,omitempty"`
}

func (e Effect) IsVisible() bool { return e.Visible == nil || *e.Visible }

type TypeStyle struct {
	FontFamily                string         `json:"fontFamily,omitempty"`
	FontPostScriptName        string         `json:"fontPostScriptName,omitempty"`
	FontWeight                float64        `json:"fontWeight,omitempty"`
	FontSize                  float64        `json:"fontSize,omitempty"`
	Italic                    bool           `json:"italic,omitempty"`
	TextAlignHorizontal       string         `json:"textAlignHorizontal,omitempty"`
	TextAlignVertical         string         `json:"textAlignVertical,omitempty"`
	LetterSpacing             float64        `json:"letterSpacing,omitempty"`
	LineHeightPx              float64        `json:"lineHeightPx,omitempty"`
	LineHeightPercentFontSize float64        `json:"lineHeightPercentFontSize,omitempty"`
	LineHeightUnit            string         `json:"lineHeightUnit,omitempty"`
	TextCase                  string         `json:"textCase,omitempty"`
	TextDecoration            string         `json:"textDecoration,omitempty"`
	TextAutoResize            string         `json:"textAutoResize,omitempty"`
	ParagraphSpacing          float64        `json:"paragraphSpacing,omitempty"`
	ParagraphIndent           float64        `json:"paragraphIndent,omitempty"`
	Hyperlink                 *Hyperlink     `json:"hyperlink,omitempty"`
	OpentypeFlags             map[string]int `json:"opentypeFlags,omitempty"`
	Fills                     []Paint        `json:"fills,omitempty"`
}

type Path struct {
	Path        string `json:"path"`
	WindingRule string `json:"windingRule,omitempty"`
}

type StrokeWeights struct {
	Top    float64 `json:"top"`
	Right  float64 `json:"right"`
	Bottom float64 `json:"bottom"`
	Left   float64 `json:"left"`
}

type ExportSetting struct {
	Suffix     string `json:"suffix,omitempty"`
	Format     string `json:"format"`
	Constraint *struct {
		Type  string  `json:"type"`
		Value float64 `json:"value"`
	} `json:"constraint,omitempty"`
}

type ComponentProperty struct {
	Type  string `json:"type"`
	Value any    `json:"value,omitempty"`
}

type ComponentPropertyDefinition struct {
	Type           string   `json:"type"`
	DefaultValue   any      `json:"defaultValue,omitempty"`
	VariantOptions []string `json:"variantOptions,omitempty"`
}

// Node is one Figma layer. Optional fields are zero or nil when Figma omits them.
type Node struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Visible   *bool    `json:"visible,omitempty"`
	Locked    bool     `json:"locked,omitempty"`
	Opacity   *float64 `json:"opacity,omitempty"`
	Rotation  float64  `json:"rotation,omitempty"`
	BlendMode string   `json:"blendMode,omitempty"`
	Children  []Node   `json:"children,omitempty"`
	// Extra holds properties this version does not model, exactly as Figma sent them.
	Extra map[string]json.RawMessage `json:"-"`
	// Drift names fields whose value had an unexpected shape; their raw value is in Extra.
	Drift []string `json:"-"`

	AbsoluteBoundingBox  *Rect             `json:"absoluteBoundingBox,omitempty"`
	AbsoluteRenderBounds *Rect             `json:"absoluteRenderBounds,omitempty"`
	Constraints          *LayoutConstraint `json:"constraints,omitempty"`

	LayoutMode              string   `json:"layoutMode,omitempty"`
	LayoutWrap              string   `json:"layoutWrap,omitempty"`
	PrimaryAxisSizingMode   string   `json:"primaryAxisSizingMode,omitempty"`
	CounterAxisSizingMode   string   `json:"counterAxisSizingMode,omitempty"`
	PrimaryAxisAlignItems   string   `json:"primaryAxisAlignItems,omitempty"`
	CounterAxisAlignItems   string   `json:"counterAxisAlignItems,omitempty"`
	CounterAxisAlignContent string   `json:"counterAxisAlignContent,omitempty"`
	PaddingLeft             float64  `json:"paddingLeft,omitempty"`
	PaddingRight            float64  `json:"paddingRight,omitempty"`
	PaddingTop              float64  `json:"paddingTop,omitempty"`
	PaddingBottom           float64  `json:"paddingBottom,omitempty"`
	ItemSpacing             float64  `json:"itemSpacing,omitempty"`
	CounterAxisSpacing      float64  `json:"counterAxisSpacing,omitempty"`
	LayoutSizingHorizontal  string   `json:"layoutSizingHorizontal,omitempty"`
	LayoutSizingVertical    string   `json:"layoutSizingVertical,omitempty"`
	LayoutPositioning       string   `json:"layoutPositioning,omitempty"`
	LayoutAlign             string   `json:"layoutAlign,omitempty"`
	LayoutGrow              float64  `json:"layoutGrow,omitempty"`
	MinWidth                *float64 `json:"minWidth,omitempty"`
	MaxWidth                *float64 `json:"maxWidth,omitempty"`
	MinHeight               *float64 `json:"minHeight,omitempty"`
	MaxHeight               *float64 `json:"maxHeight,omitempty"`
	ClipsContent            bool     `json:"clipsContent,omitempty"`

	Fills                   []Paint         `json:"fills,omitempty"`
	Strokes                 []Paint         `json:"strokes,omitempty"`
	StrokeWeight            float64         `json:"strokeWeight,omitempty"`
	StrokeAlign             string          `json:"strokeAlign,omitempty"`
	StrokeDashes            []float64       `json:"strokeDashes,omitempty"`
	IndividualStrokeWeights *StrokeWeights  `json:"individualStrokeWeights,omitempty"`
	CornerRadius            float64         `json:"cornerRadius,omitempty"`
	RectangleCornerRadii    []float64       `json:"rectangleCornerRadii,omitempty"`
	Effects                 []Effect        `json:"effects,omitempty"`
	IsMask                  bool            `json:"isMask,omitempty"`
	ExportSettings          []ExportSetting `json:"exportSettings,omitempty"`
	BooleanOperation        string          `json:"booleanOperation,omitempty"`
	StrokeCap               string          `json:"strokeCap,omitempty"`
	StrokeJoin              string          `json:"strokeJoin,omitempty"`
	StrokeMiterAngle        float64         `json:"strokeMiterAngle,omitempty"`
	LayoutGrids             []LayoutGrid    `json:"layoutGrids,omitempty"`
	ArcData                 *ArcData        `json:"arcData,omitempty"`
	PointCount              int             `json:"pointCount,omitempty"`
	StarInnerRadius         float64         `json:"starInnerRadius,omitempty"`
	PreserveRatio           bool            `json:"preserveRatio,omitempty"`
	IsFixed                 bool            `json:"isFixed,omitempty"`
	ScrollBehavior          string          `json:"scrollBehavior,omitempty"`
	// Styles links this node to shared styles, keyed by role (fill, stroke, text, effect, grid).
	Styles map[string]string `json:"styles,omitempty"`
	// BoundVariables is kept raw: variable bindings are not modelled yet.
	BoundVariables json.RawMessage `json:"boundVariables,omitempty"`
	FillGeometry   []Path          `json:"fillGeometry,omitempty"`
	StrokeGeometry []Path          `json:"strokeGeometry,omitempty"`

	Characters              string               `json:"characters,omitempty"`
	Style                   *TypeStyle           `json:"style,omitempty"`
	CharacterStyleOverrides []int                `json:"characterStyleOverrides,omitempty"`
	StyleOverrideTable      map[string]TypeStyle `json:"styleOverrideTable,omitempty"`
	LineTypes               []string             `json:"lineTypes,omitempty"`
	LineIndentations        []int                `json:"lineIndentations,omitempty"`

	ComponentID                  string                                 `json:"componentId,omitempty"`
	ComponentProperties          map[string]ComponentProperty           `json:"componentProperties,omitempty"`
	ComponentPropertyDefinitions map[string]ComponentPropertyDefinition `json:"componentPropertyDefinitions,omitempty"`
	ComponentPropertyReferences  map[string]string                      `json:"componentPropertyReferences,omitempty"`
	// Overrides lists the properties an instance changed from its main component.
	Overrides []InstanceOverride `json:"overrides,omitempty"`
}

// Known reports whether Layr models this node type; unknown types are still returned intact.
func (n Node) Known() bool { return knownNodeTypes[n.Type] }

func (n Node) IsVisible() bool { return n.Visible == nil || *n.Visible }

type ComponentMeta struct {
	Key            string `json:"key"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Remote         bool   `json:"remote,omitempty"`
	ComponentSetID string `json:"componentSetId,omitempty"`
}

type ComponentSetMeta struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Remote      bool   `json:"remote,omitempty"`
}

type StyleMeta struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Remote      bool   `json:"remote,omitempty"`
	StyleType   string `json:"styleType"`
}

type FileInfo struct {
	Name         string `json:"name"`
	Role         string `json:"role,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	EditorType   string `json:"editorType,omitempty"`
	ThumbnailURL string `json:"thumbnailUrl,omitempty"`
	Version      string `json:"version,omitempty"`
}

type File struct {
	FileInfo
	SchemaVersion int                         `json:"schemaVersion"`
	Document      Node                        `json:"document"`
	Components    map[string]ComponentMeta    `json:"components,omitempty"`
	ComponentSets map[string]ComponentSetMeta `json:"componentSets,omitempty"`
	Styles        map[string]StyleMeta        `json:"styles,omitempty"`
}

// NodeEntry is the subtree returned for one requested node.
type NodeEntry struct {
	Document      Node                        `json:"document"`
	Components    map[string]ComponentMeta    `json:"components,omitempty"`
	ComponentSets map[string]ComponentSetMeta `json:"componentSets,omitempty"`
	SchemaVersion int                         `json:"schemaVersion"`
	Styles        map[string]StyleMeta        `json:"styles,omitempty"`
}

// FileNodes lists resolved nodes; Missing was null or omitted, Malformed was unreadable.
type FileNodes struct {
	FileInfo
	Nodes     map[string]NodeEntry
	Missing   []string
	Malformed []string
}

// Roots returns the document node of every resolved entry.
func (f *FileNodes) Roots() []Node {
	out := make([]Node, 0, len(f.Nodes))
	for _, e := range f.Nodes {
		out = append(out, e.Document)
	}
	return out
}

// Renders maps node IDs to temporary export URLs; Failed lists nodes Figma could not render.
type Renders struct {
	URLs   map[string]string
	Failed []string
}

type LayoutGrid struct {
	Pattern     string  `json:"pattern"`
	SectionSize float64 `json:"sectionSize,omitempty"`
	Visible     *bool   `json:"visible,omitempty"`
	Color       *Color  `json:"color,omitempty"`
	Alignment   string  `json:"alignment,omitempty"`
	GutterSize  float64 `json:"gutterSize,omitempty"`
	Offset      float64 `json:"offset,omitempty"`
	Count       int     `json:"count,omitempty"`
}

type Hyperlink struct {
	Type   string `json:"type"`
	URL    string `json:"url,omitempty"`
	NodeID string `json:"nodeID,omitempty"`
}

type ArcData struct {
	StartingAngle float64 `json:"startingAngle"`
	EndingAngle   float64 `json:"endingAngle"`
	InnerRadius   float64 `json:"innerRadius"`
}

type InstanceOverride struct {
	ID     string   `json:"id"`
	Fields []string `json:"overriddenFields"`
}
