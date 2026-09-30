package figma

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const (
	maxNodeIDs   = 100
	maxKeyLength = 128
)

// FileOptions selects the smallest useful payload; zero values send nothing.
type FileOptions struct {
	// Version pins a file version ID.
	Version string
	// Depth limits how deep the node tree goes; 0 means Figma's default (everything).
	Depth int
	// Geometry adds vector path data (geometry=paths).
	Geometry bool
	// Snapshot receives the untouched JSON of a successful response; discard it on error.
	Snapshot io.Writer
}

type NodesOptions = FileOptions

// GetFile returns a file's metadata and node tree (GET /v1/files/:key).
func (a *API) GetFile(ctx context.Context, userID, fileKey string, opts FileOptions) (*File, error) {
	if err := validateKey(fileKey); err != nil {
		return nil, err
	}
	q, err := opts.values()
	if err != nil {
		return nil, err
	}

	var out File
	err = a.get(ctx, call{op: "get_file", userID: userID, fileKey: fileKey, path: "/v1/files/" + url.PathEscape(fileKey), query: q, snapshot: opts.Snapshot}, &out)
	if err != nil {
		return nil, err
	}
	a.reportDrift(ctx, "get_file", userID, fileKey, out.Document)
	return &out, nil
}

// GetFileNodes returns the subtrees for the given canonical node IDs (GET /v1/files/:key/nodes).
func (a *API) GetFileNodes(ctx context.Context, userID, fileKey string, nodeIDs []string, opts NodesOptions) (*FileNodes, error) {
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

	var body nodesEnvelope
	err = a.get(ctx, call{op: "get_file_nodes", userID: userID, fileKey: fileKey, nodeCount: len(ids), path: "/v1/files/" + url.PathEscape(fileKey) + "/nodes", query: q, snapshot: opts.Snapshot}, &body)
	if err != nil {
		return nil, err
	}

	out := &FileNodes{FileInfo: body.FileInfo, Nodes: make(map[string]NodeEntry, len(ids))}
	for _, id := range ids {
		raw := bytes.TrimSpace(body.Nodes[id])
		if len(raw) == 0 || string(raw) == "null" {
			out.Missing = append(out.Missing, id)
			continue
		}
		var entry NodeEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			out.Malformed = append(out.Malformed, id)
			continue
		}
		out.Nodes[id] = entry
	}
	a.reportDrift(ctx, "get_file_nodes", userID, fileKey, out.Roots()...)
	return out, nil
}

func (o FileOptions) values() (url.Values, error) {
	q := url.Values{}
	if o.Depth < 0 {
		return nil, fmt.Errorf("%w: depth must not be negative", ErrInvalidInput)
	}
	if o.Depth > 0 {
		q.Set("depth", strconv.Itoa(o.Depth))
	}
	if o.Version != "" {
		if err := validateToken("version", o.Version); err != nil {
			return nil, err
		}
		q.Set("version", o.Version)
	}
	if o.Geometry {
		q.Set("geometry", "paths")
	}
	return q, nil
}

// validateKey rejects only what would break the URL path; real keys are opaque.
func validateKey(key string) error {
	if key == "" || len(key) > maxKeyLength {
		return fmt.Errorf("%w: file key must be 1 to %d characters", ErrInvalidInput, maxKeyLength)
	}
	return validateToken("file key", key)
}

func validateToken(what, v string) error {
	for _, r := range v {
		if r <= ' ' || r == 0x7f || strings.ContainsRune("/?#\\,", r) {
			return fmt.Errorf("%w: %s contains an unsupported character", ErrInvalidInput, what)
		}
	}
	return nil
}

// validateNodeIDs trims duplicates and enforces a bounded, comma-safe list.
func validateNodeIDs(ids []string) ([]string, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("%w: at least one node ID is required", ErrInvalidInput)
	}
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id == "" || len(id) > 64 {
			return nil, fmt.Errorf("%w: node ID must be 1 to 64 characters", ErrInvalidInput)
		}
		if err := validateToken("node ID", id); err != nil {
			return nil, err
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) > maxNodeIDs {
		return nil, fmt.Errorf("%w: at most %d node IDs per call", ErrInvalidInput, maxNodeIDs)
	}
	sort.Strings(out)
	return out, nil
}

// nodesEnvelope is the /nodes response; its metadata is decoded leniently and only nodes is required.
type nodesEnvelope struct {
	FileInfo
	Nodes map[string]json.RawMessage `json:"nodes"`
}

func (e *nodesEnvelope) UnmarshalJSON(data []byte) error {
	*e = nodesEnvelope{}
	_, _, err := decodeObject(data, e, objectHooks{required: map[string]bool{"nodes": true}})
	return err
}
