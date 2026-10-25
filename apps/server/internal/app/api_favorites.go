package app

import (
	"context"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

type APIFavorite struct {
	Listing  APIListing `json:"listing"`
	AddedAt  time.Time  `json:"added_at"`
	LastSeen string     `json:"last_seen,omitempty" doc:"The last day any collection saw the listing"`
	Best     *APIGrade  `json:"best,omitempty" doc:"The best grade across the profiles that graded the listing"`
}

func (a *App) apiFavorites(ctx context.Context, _ *struct{}) (*struct{ Body []APIFavorite }, error) {
	fs, err := a.svc.Favorites(ctx)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := make([]APIFavorite, len(fs))
	for i, f := range fs {
		out[i] = APIFavorite{Listing: a.listing(ctx, f.Listing), AddedAt: f.CreatedAt, LastSeen: f.LastSeen}
		if f.Graded {
			g := apiGrade(f.Grade)
			out[i].Best = &g
		}
	}
	return &struct{ Body []APIFavorite }{out}, nil
}

type favoriteInput struct {
	Source   listing.Source `path:"source"`
	SourceID string         `path:"source_id"`
}

func (a *App) apiSetFavorite(on bool) func(context.Context, *favoriteInput) (*struct{}, error) {
	return func(ctx context.Context, in *favoriteInput) (*struct{}, error) {
		if err := a.svc.SetFavorite(ctx, in.Source, in.SourceID, on); err != nil {
			return nil, a.apiError(ctx, err)
		}
		return nil, nil
	}
}
