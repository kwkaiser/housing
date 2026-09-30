package pipeline

import (
	"context"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
)

type CollageOptions struct {
	Grid         media.Grid
	MaxPhotos    int
	Force        bool
	FetchMissing bool
}

func (e *Env) Collage(ctx context.Context, o CollageOptions, listings []listing.Listing) ([]listing.Listing, error) {
	if listings == nil {
		var err error
		if listings, err = e.persister().Load(ctx, e.DataDir); err != nil {
			return nil, err
		}
	}

	p := media.NewProcessor(media.NewHTTPFetcher(), e.images(), o.Grid)
	if o.MaxPhotos > 0 {
		p.MaxPhotos = o.MaxPhotos
	}
	p.Force = o.Force
	if o.FetchMissing {
		if err := p.FetchPhotos(ctx, listings); err != nil {
			return nil, err
		}
	}
	processed, err := p.Collage(ctx, listings)
	if err != nil {
		return nil, err
	}
	if err := e.persister().Persist(ctx, e.DataDir, processed); err != nil {
		return nil, err
	}

	distinct := map[string]bool{}
	without := 0
	for _, l := range processed {
		if len(l.Collages) == 0 {
			without++
		}
		for _, c := range l.Collages {
			distinct[c] = true
		}
	}
	e.printf("collage: %d listings, %d collages, %d listings without collages\n", len(processed), len(distinct), without)
	return processed, nil
}
