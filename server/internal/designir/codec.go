package designir

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// Marshal validates and serialises the design deterministically (struct order, sorted, compact).
func Marshal(ir *DesignIR, lim Limits) ([]byte, error) {
	if err := Validate(ir, lim); err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(ir); err != nil {
		return nil, &Error{Code: CodeSerialize, Problems: []string{"the design could not be serialised"}}
	}
	return buf.Bytes(), nil
}

// Unmarshal reads a design strictly: unknown fields, trailing data and invalid designs are rejected.
func Unmarshal(data []byte, lim Limits) (*DesignIR, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var ir DesignIR
	if err := dec.Decode(&ir); err != nil {
		return nil, &Error{Code: CodeInvalidInput, Problems: []string{"the design file is not valid Design IR JSON"}}
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, &Error{Code: CodeInvalidInput, Problems: []string{"unexpected data after the design"}}
	}
	if err := Validate(&ir, lim); err != nil {
		return nil, err
	}
	return &ir, nil
}

// Select returns an independent design of only the given screens, recomputed, with no Figma data needed.
func (ir *DesignIR) Select(screenIDs []string, lim Limits) (*DesignIR, error) {
	data, err := Marshal(ir, lim)
	if err != nil {
		return nil, err
	}
	clone, err := Unmarshal(data, lim)
	if err != nil {
		return nil, err
	}

	want := map[string]bool{}
	for _, id := range screenIDs {
		want[id] = true
	}
	kept := clone.Screens[:0:0]
	nodeIDs := []string{}
	for _, s := range clone.Screens {
		if want[s.ID] {
			kept = append(kept, s)
			nodeIDs = append(nodeIDs, s.SourceNodeID)
			delete(want, s.ID)
		}
	}
	if len(want) > 0 || len(kept) == 0 {
		return nil, &Error{Code: CodeInvalidInput, Problems: []string{"a requested screen does not exist"}}
	}
	clone.Screens, clone.Source.NodeIDs = kept, nodeIDs
	clone.Finalize()
	return clone, Validate(clone, lim)
}
