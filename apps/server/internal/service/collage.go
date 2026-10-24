package service

import (
	"context"
	"log/slog"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
)

type CollageOptions struct {
	Grid         media.Grid
	MaxPhotos    int
	Force        bool
	FetchMissing bool
}

type CollageResult struct {
	Listings        []listing.Listing
	Collages        int
	WithoutCollages int
}

func DefaultCollageOptions() CollageOptions {
	return CollageOptions{Grid: media.NewGrid(), MaxPhotos: media.DefaultMaxPhotos}
}

func (s *Service) Collage(ctx context.Context, o CollageOptions, listings []listing.Listing, log *slog.Logger) (CollageResult, error) {
	var res CollageResult
	log = s.logger(log).With("stage", StageCollage)
	db := s.db
	if listings == nil {
		var err error
		if listings, err = db.Load(ctx); err != nil {
			return res, err
		}
	}

	p := media.NewProcessor(s.photoFetcher(), s.images(), o.Grid)
	if o.MaxPhotos > 0 {
		p.MaxPhotos = o.MaxPhotos
	}
	p.Force = o.Force
	p.Logger = log
	if o.FetchMissing {
		if err := p.FetchPhotos(ctx, listings); err != nil {
			return res, err
		}
	}
	processed, err := p.Collage(ctx, listings)
	if err != nil {
		return res, err
	}
	if err := db.Save(ctx, processed); err != nil {
		return res, err
	}

	distinct := map[string]bool{}
	for _, l := range processed {
		if len(l.Collages) == 0 {
			res.WithoutCollages++
		}
		for _, c := range l.Collages {
			distinct[c] = true
		}
	}
	res.Listings = processed
	res.Collages = len(distinct)
	log.Info("collaged", "listings", len(processed), "collages", res.Collages, "without_collages", res.WithoutCollages)
	return res, nil
}
