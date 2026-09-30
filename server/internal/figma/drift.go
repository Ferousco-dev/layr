package figma

import (
	"context"
	"log/slog"
	"sort"
)

// DriftReport lists what Figma sent that this version of Layr does not understand.
type DriftReport struct {
	// UnknownNodeTypes are node types Layr does not model.
	UnknownNodeTypes []string
	// ExtraFields are node properties that are not modelled; their values remain in Node.Extra.
	ExtraFields []string
	// MismatchedFields are modelled properties that arrived in an unexpected shape.
	MismatchedFields []string
}

func (d DriftReport) Empty() bool {
	return len(d.UnknownNodeTypes) == 0 && len(d.ExtraFields) == 0 && len(d.MismatchedFields) == 0
}

// Analyze walks node trees and reports drift. It is cheap and safe to call on any result.
func Analyze(roots ...Node) DriftReport {
	types, extra, mismatched := map[string]bool{}, map[string]bool{}, map[string]bool{}
	var walk func(n Node)
	walk = func(n Node) {
		if !n.Known() {
			types[n.Type] = true
		}
		for k := range n.Extra {
			extra[k] = true
		}
		for _, k := range n.Drift {
			mismatched[k] = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, r := range roots {
		walk(r)
	}
	return DriftReport{UnknownNodeTypes: keys(types), ExtraFields: keys(extra), MismatchedFields: keys(mismatched)}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// reportDrift logs one warning per call when Figma sent something new, so gaps get noticed.
func (a *API) reportDrift(ctx context.Context, op, userID, fileKey string, roots ...Node) {
	report := Analyze(roots...)
	if report.Empty() {
		return
	}
	a.log.WarnContext(ctx, "figma.schema_drift",
		slog.String("figma_operation", op),
		slog.String("user_id", userID),
		slog.String("file_key", fileKey),
		slog.Any("unknown_node_types", first(report.UnknownNodeTypes, 10)),
		slog.Any("extra_fields", first(report.ExtraFields, 20)),
		slog.Any("mismatched_fields", first(report.MismatchedFields, 20)))
}

func first(items []string, n int) []string {
	if len(items) > n {
		return items[:n]
	}
	return items
}
