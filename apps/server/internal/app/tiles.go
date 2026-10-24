package app

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"git.kwkaiser.io/kwkaiser/housing/internal/tiles"
)

var errNoTile = errors.New("no such tile")

func (a *App) positronTile(w http.ResponseWriter, r *http.Request) {
	b, err := a.positronPNG(r.Context(), r.PathValue("z"), r.PathValue("x"), r.PathValue("file"))
	if errors.Is(err, errNoTile) {
		a.notFound(w, r)
		return
	}
	if err != nil {
		http.Error(w, "tile unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(b)
}

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
