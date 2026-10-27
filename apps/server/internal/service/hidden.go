package service

import (
	"context"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func (s *Service) SetHidden(ctx context.Context, source listing.Source, sourceID string, on bool) error {
	db, err := s.catalog(ctx)
	if err != nil {
		return err
	}
	return db.SetHidden(ctx, source, sourceID, on)
}
