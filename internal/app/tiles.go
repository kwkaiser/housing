package app

import (
	"net/http"
	"strconv"

	"git.kwkaiser.io/kwkaiser/housing/internal/tiles"
)

func (a *App) positronTile(w http.ResponseWriter, r *http.Request) {
	t, err := tiles.Parse(r.PathValue("z"), r.PathValue("x"), r.PathValue("file"), ".png")
	if err != nil || !a.positron.Enabled() {
		a.notFound(w, r)
		return
	}
	b, err := a.positron.Get(r.Context(), t)
	if err != nil {
		a.log.Warn("tile", "z", t.Z, "x", t.X, "y", t.Y, "err", err)
		http.Error(w, "tile unavailable", http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Content-Length", strconv.Itoa(len(b)))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(b)
}
