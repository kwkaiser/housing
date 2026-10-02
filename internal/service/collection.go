package service

import (
	"context"
	"errors"
	"fmt"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func (s *Service) Collection(ctx context.Context, id string) (collection.Collection, error) {
	db, err := s.catalog(ctx)
	if err != nil {
		return collection.Collection{}, err
	}
	return db.Collection(ctx, id)
}

func (s *Service) Collections(ctx context.Context) ([]collection.Collection, error) {
	db, err := s.catalog(ctx)
	if err != nil {
		return nil, err
	}
	return db.Collections(ctx)
}

func (s *Service) SaveCollection(ctx context.Context, c collection.Collection) error {
	return s.saveCollection(ctx, c, false)
}

func (s *Service) CreateCollection(ctx context.Context, c collection.Collection) error {
	return s.saveCollection(ctx, c, true)
}

func (s *Service) saveCollection(ctx context.Context, c collection.Collection, create bool) error {
	db, err := s.catalog(ctx)
	if err != nil {
		return err
	}
	var errs []error
	if err := c.Validate(); err != nil {
		errs = append(errs, err)
	}
	if create && profile.ValidID(c.ID) == nil {
		switch _, err := db.Collection(ctx, c.ID); {
		case err == nil:
			errs = append(errs, profile.FieldError{Field: "id", Err: fmt.Errorf("%w: %s", collection.ErrExists, c.ID)})
		case !errors.Is(err, collection.ErrNotFound):
			return err
		}
	}
	if err := ValidateSources(c.Sources); err != nil {
		errs = append(errs, profile.FieldError{Field: "sources", Err: err})
	}
	for _, id := range c.Profiles {
		p, err := db.Profile(ctx, id)
		switch {
		case errors.Is(err, profile.ErrNotFound):
			errs = append(errs, profile.FieldError{Field: "profiles", Err: err})
		case err != nil:
			return err
		case p.IsAvoid():
			errs = append(errs, profile.FieldError{Field: "profiles", Err: fmt.Errorf("profile %q is an avoid profile; avoid profiles apply to every collection automatically", id)})
		default:
			if _, err := db.EffectiveProfile(ctx, id); err != nil {
				return err
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	return db.SaveCollection(ctx, c)
}
