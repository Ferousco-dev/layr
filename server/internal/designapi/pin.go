package designapi

import (
	"context"
	"errors"

	"github.com/ferousco-dev/layr/server/internal/designir"
)

// Pinned is a design fixed to one exact version. Design is shared between requests and must never be modified.
type Pinned struct {
	Design   *designir.DesignIR
	Version  string
	ImportID string
}

// Pin proves ownership and returns the project's design only if it is exactly the requested version.
// A design that was replaced or whose temporary files expired is ErrVersionUnavailable, never a different design.
func (s *Service) Pin(ctx context.Context, userID, projectID, version string) (*Pinned, error) {
	st, ix, err := s.ready(ctx, userID, projectID)
	switch {
	case errors.Is(err, ErrExpired):
		return nil, ErrVersionUnavailable
	case err != nil:
		return nil, err
	}
	if ix.version != version {
		return nil, ErrVersionUnavailable
	}
	return &Pinned{Design: ix.ir, Version: ix.version, ImportID: st.current.ID}, nil
}
