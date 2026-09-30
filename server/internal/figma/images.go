package figma

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// Render formats Layr may request from Figma.
const (
	FormatPNG = "png"
	FormatJPG = "jpg"
	FormatSVG = "svg"
)

// RenderOptions are the export settings Layr foresees using.
type RenderOptions struct {
	// Format is png, jpg or svg; empty means png.
	Format string
	// Scale is 0.01 to 4; zero means Figma's default of 1.
	Scale   float64
	Version string
	// UseAbsoluteBounds renders the full node box including clipped content.
	UseAbsoluteBounds bool
	// SVG-only switches; nil leaves Figma's default (outline text and simplify strokes are on).
	SVGOutlineText    *bool
	SVGSimplifyStroke *bool
	SVGIncludeID      bool
}

// GetImageFills maps imageRef to temporary URLs (GET /v1/files/:key/images); never store them.
func (a *API) GetImageFills(ctx context.Context, userID, fileKey string) (map[string]string, error) {
	if err := validateKey(fileKey); err != nil {
		return nil, err
	}

	var body struct {
		Images map[string]json.RawMessage `json:"images"`
		Meta   struct {
			Images map[string]json.RawMessage `json:"images"`
		} `json:"meta"`
	}
	err := a.get(ctx, call{op: "get_image_fills", userID: userID, fileKey: fileKey, path: "/v1/files/" + url.PathEscape(fileKey) + "/images"}, &body)
	if err != nil {
		return nil, err
	}

	// Figma documents the map at the top level but has also returned it under meta.
	fills := body.Images
	if len(fills) == 0 {
		fills = body.Meta.Images
	}
	out := make(map[string]string, len(fills))
	for ref, raw := range fills {
		var u string
		if json.Unmarshal(raw, &u) == nil && u != "" {
			out[ref] = u
		}
	}
	return out, nil
}

// RenderNodes asks Figma to export nodes as PNG, JPG or SVG (GET /v1/images/:key).
func (a *API) RenderNodes(ctx context.Context, userID, fileKey string, nodeIDs []string, opts RenderOptions) (*Renders, error) {
	if err := validateKey(fileKey); err != nil {
		return nil, err
	}
	ids, err := validateNodeIDs(nodeIDs)
	if err != nil {
		return nil, err
	}
	q, err := opts.values()
	if err != nil {
		return nil, err
	}
	q.Set("ids", strings.Join(ids, ","))

	var body struct {
		Err    any                        `json:"err"`
		Images map[string]json.RawMessage `json:"images"`
	}
	err = a.get(ctx, call{op: "render_nodes", userID: userID, fileKey: fileKey, nodeCount: len(ids), path: "/v1/images/" + url.PathEscape(fileKey), query: q}, &body)
	if err != nil {
		return nil, err
	}
	if body.Err != nil {
		return nil, &Error{Kind: KindBadResponse, Operation: "render_nodes", Status: 200, Err: fmt.Errorf("provider reported a render error")}
	}

	out := &Renders{URLs: make(map[string]string, len(ids))}
	for _, id := range ids {
		var u string
		if err := json.Unmarshal(body.Images[id], &u); err == nil && u != "" {
			out.URLs[id] = u
		} else {
			out.Failed = append(out.Failed, id)
		}
	}
	return out, nil
}

func (o RenderOptions) values() (url.Values, error) {
	q := url.Values{}
	format := o.Format
	if format == "" {
		format = FormatPNG
	}
	if format != FormatPNG && format != FormatJPG && format != FormatSVG {
		return nil, fmt.Errorf("%w: format must be png, jpg or svg", ErrInvalidInput)
	}
	q.Set("format", format)

	if o.Scale != 0 {
		if o.Scale < 0.01 || o.Scale > 4 {
			return nil, fmt.Errorf("%w: scale must be between 0.01 and 4", ErrInvalidInput)
		}
		q.Set("scale", strconv.FormatFloat(o.Scale, 'f', -1, 64))
	}
	if o.Version != "" {
		if err := validateToken("version", o.Version); err != nil {
			return nil, err
		}
		q.Set("version", o.Version)
	}
	if o.UseAbsoluteBounds {
		q.Set("use_absolute_bounds", "true")
	}
	if format == FormatSVG {
		if o.SVGOutlineText != nil {
			q.Set("svg_outline_text", strconv.FormatBool(*o.SVGOutlineText))
		}
		if o.SVGSimplifyStroke != nil {
			q.Set("svg_simplify_stroke", strconv.FormatBool(*o.SVGSimplifyStroke))
		}
		if o.SVGIncludeID {
			q.Set("svg_include_id", "true")
		}
	}
	return q, nil
}
