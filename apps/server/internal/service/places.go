package service

import (
	"context"

	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

func (s *Service) Places(ctx context.Context) ([]store.Place, error) {
	return s.db.Places(ctx)
}

func (s *Service) AddPlace(ctx context.Context, p store.Place) (store.Place, error) {
	p.CreatedAt = s.cfg.Now().UTC()
	return s.db.AddPlace(ctx, p)
}

func (s *Service) RemovePlace(ctx context.Context, id int64) error {
	return s.db.RemovePlace(ctx, id)
}
