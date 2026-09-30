// Package figmaurl extracts a file key and optional node ID from a pasted Figma URL, and nothing else.
package figmaurl

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

const maxLength = 2048

// ErrInvalid is returned for anything that is not a supported Figma design URL.
var ErrInvalid = errors.New("figma url invalid")

// Parsed holds the identifiers Layr needs; NodeID is in API form ("120:450") or empty.
type Parsed struct {
	FileKey string
	NodeID  string
}

// hosts is an exact allow-list; substring or suffix matching would accept lookalike domains.
var hosts = map[string]bool{"figma.com": true, "www.figma.com": true}

// kinds are the URL path prefixes that open a design file; board (FigJam) and make are not importable.
var kinds = map[string]bool{"design": true, "file": true, "proto": true}

var (
	fileKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{6,128}$`)
	// urlNode is the query form: digits separated by hyphens, optionally an instance path joined by ";".
	urlNode = regexp.MustCompile(`^I?\d+-\d+(;\d+-\d+)*$`)
	apiNode = regexp.MustCompile(`^I?\d+:\d+(;\d+:\d+)*$`)
)

// Parse validates raw and returns the file key and normalized node ID.
func Parse(raw string) (Parsed, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxLength {
		return Parsed{}, ErrInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Opaque != "" {
		return Parsed{}, ErrInvalid
	}
	if !hosts[strings.ToLower(u.Hostname())] || (u.Port() != "" && u.Port() != "443") {
		return Parsed{}, ErrInvalid
	}

	key, err := fileKey(u.Path)
	if err != nil {
		return Parsed{}, err
	}
	node, err := nodeID(u.Query()["node-id"])
	if err != nil {
		return Parsed{}, err
	}
	return Parsed{FileKey: key, NodeID: node}, nil
}

// fileKey reads /design/:key, /file/:key or /proto/:key; a branch URL uses the branch key.
func fileKey(path string) (string, error) {
	segments := strings.Split(strings.Trim(path, "/"), "/")
	if len(segments) < 2 || !kinds[segments[0]] {
		return "", ErrInvalid
	}
	key := segments[1]
	if len(segments) >= 4 && segments[2] == "branch" {
		key = segments[3]
	}
	if !fileKeyPattern.MatchString(key) {
		return "", ErrInvalid
	}
	return key, nil
}

// nodeID accepts none, or exactly one node-id value in URL or API form.
func nodeID(values []string) (string, error) {
	switch len(values) {
	case 0:
		return "", nil
	case 1:
	default:
		return "", ErrInvalid
	}
	v := values[0]
	switch {
	case v == "":
		return "", nil
	case urlNode.MatchString(v):
		return strings.ReplaceAll(v, "-", ":"), nil
	case apiNode.MatchString(v):
		return v, nil
	}
	return "", ErrInvalid
}
