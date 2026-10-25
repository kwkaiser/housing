package app

import (
	"context"
	"errors"

	"git.kwkaiser.io/kwkaiser/housing/internal/tiles"
)

var errNoTile = errors.New("no such tile")

func (a *App) positronPNG(ctx context.Context, z, x, file string) ([]byte, error) {
	t, err := tiles.Parse(z, x, file, ".png")
	if err != nil || !a.positron.Enabled() {
		return nil, errNoTile
	}
	b, err := a.positron.Get(ctx, t)
	if err != nil {
		a.log.Warn("tile", "z", t.Z, "x", t.X, "y", t.Y, "err", err)
		return nil, err
	}
	return b, nil
}

func (a *App) terrainWebP(ctx context.Context, z, x, file string) ([]byte, error) {
	t, err := tiles.Parse(z, x, file, ".webp")
	if err != nil || t.Z > a.terrain.MaxZoom {
		return nil, errNoTile
	}
	b, err := a.terrain.Get(ctx, t)
	if err != nil {
		a.log.Warn("terrain tile", "z", t.Z, "x", t.X, "y", t.Y, "err", err)
		return nil, err
	}
	return b, nil
}
