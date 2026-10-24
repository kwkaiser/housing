package service

import (
	"context"
	"slices"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

type FavoriteView struct {
	store.Favorite
	report.Grade
	Graded bool
}

func (s *Service) SetFavorite(ctx context.Context, source listing.Source, sourceID string, on bool) error {
	db, err := s.catalog(ctx)
	if err != nil {
		return err
	}
	if !on {
		return db.RemoveFavorite(ctx, source, sourceID)
	}
	return db.AddFavorite(ctx, source, sourceID, s.cfg.Now())
}

func (s *Service) Favorites(ctx context.Context) ([]FavoriteView, error) {
	db, err := s.catalog(ctx)
	if err != nil {
		return nil, err
	}
	fs, err := db.Favorites(ctx)
	if err != nil {
		return nil, err
	}
	ps, err := db.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, p := range ps {
		if !p.IsAvoid() && slices.ContainsFunc(fs, func(f store.Favorite) bool { _, ok := f.Listing.Assessments[p.ID]; return ok }) {
			ids = append(ids, p.ID)
		}
	}
	var templates []template
	if len(ids) > 0 {
		if templates, err = s.templates(ctx, db, ids); err != nil {
			return nil, err
		}
	}
	out := make([]FavoriteView, len(fs))
	for i, f := range fs {
		out[i] = FavoriteView{Favorite: f}
		var ts []report.Template
		for _, t := range templates {
			if _, ok := f.Listing.Assessments[t.Profile.ID]; ok {
				ts = append(ts, t.Template)
			}
		}
		if len(ts) == 0 {
			continue
		}
		r := report.Build(ts, []listing.Listing{f.Listing}, report.Options{IncludeStale: true, Dealbreakers: true})
		if len(r.Rows) > 0 {
			out[i].Grade, out[i].Graded = r.Rows[0].Grade, true
		}
	}
	return out, nil
}
