package app

import (
	"context"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

type hiddenInput struct {
	Source   listing.Source `path:"source"`
	SourceID string         `path:"source_id"`
}

func (a *App) apiSetHidden(on bool) func(context.Context, *hiddenInput) (*struct{}, error) {
	return func(ctx context.Context, in *hiddenInput) (*struct{}, error) {
		if err := a.svc.SetHidden(ctx, in.Source, in.SourceID, on); err != nil {
			return nil, a.apiError(ctx, err)
		}
		return nil, nil
	}
}
