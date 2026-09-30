package designapi

import "context"

// ScreenIDs lists every screen of the project's current design, in design order; the caller must own the project.
func (s *Service) ScreenIDs(ctx context.Context, userID, projectID string) ([]string, error) {
	_, ix, err := s.ready(ctx, userID, projectID)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ix.ir.Screens))
	for i := range ix.ir.Screens {
		out[i] = ix.ir.Screens[i].ID
	}
	return out, nil
}
