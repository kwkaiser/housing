package app

import (
	"encoding/json"
	"net/http"

	"git.kwkaiser.io/kwkaiser/housing/internal/mvt"
	"git.kwkaiser.io/kwkaiser/housing/internal/overlays"
	"git.kwkaiser.io/kwkaiser/housing/internal/tiles"
)

type overlay struct {
	name    string
	source  *tiles.Cache
	minZoom int
	kind    overlays.Kind
}

type overlayConfig struct {
	Name    string `json:"name"`
	MinZoom int    `json:"minZoom"`
	MaxZoom int    `json:"maxZoom"`
}

type mapConfig struct {
	Positron bool            `json:"positron"`
	Overlays []overlayConfig `json:"overlays"`
}

func newOverlays(transit, openFreeMap *tiles.Cache) []overlay {
	return []overlay{
		{name: "bike", source: openFreeMap, minZoom: 13, kind: overlays.Bike},
		{name: "rail", source: transit, minZoom: 9, kind: overlays.Rail},
		{name: "bus", source: transit, minZoom: 12, kind: overlays.Bus},
	}
}

func (a *App) overlay(name string) (overlay, bool) {
	for _, ov := range a.overlays {
		if ov.name == name && ov.source.Enabled() {
			return ov, true
		}
	}
	return overlay{}, false
}

func (a *App) mapConfig() mapConfig {
	c := mapConfig{Positron: a.positron.Enabled(), Overlays: []overlayConfig{}}
	for _, ov := range a.overlays {
		if ov.source.Enabled() {
			c.Overlays = append(c.Overlays, overlayConfig{Name: ov.name, MinZoom: ov.minZoom, MaxZoom: ov.source.MaxZoom})
		}
	}
	return c
}

func (a *App) overlayTile(w http.ResponseWriter, r *http.Request) {
	ov, ok := a.overlay(r.PathValue("name"))
	t, err := tiles.Parse(r.PathValue("z"), r.PathValue("x"), r.PathValue("file"), ".json")
	if !ok || err != nil || t.Z < ov.minZoom || t.Z > ov.source.MaxZoom {
		a.notFound(w, r)
		return
	}
	b, err := ov.source.Get(r.Context(), t)
	if err != nil {
		a.log.Warn("overlay tile", "overlay", ov.name, "z", t.Z, "x", t.X, "y", t.Y, "err", err)
		http.Error(w, "overlay unavailable", http.StatusBadGateway)
		return
	}
	layers, err := mvt.Decode(b, t.Z, t.X, t.Y, ov.kind.Layer)
	if err != nil {
		a.log.Warn("overlay decode", "overlay", ov.name, "z", t.Z, "x", t.X, "y", t.Y, "err", err)
		http.Error(w, "overlay unavailable", http.StatusBadGateway)
		return
	}
	body, err := json.Marshal(struct {
		Features []overlays.Feature `json:"features"`
	}{ov.kind.Collect(layers)})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(body)
}
