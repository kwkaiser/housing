package app

import (
	"context"
	"encoding/json"
	"path"

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
	Name    string
	MinZoom int
	MaxZoom int
}

type mapConfig struct {
	Positron bool
	Overlays []overlayConfig
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

func (a *App) overlayData(ctx context.Context, name, z, x, file string) ([]byte, string, error) {
	ov, ok := a.overlay(name)
	ext := path.Ext(file)
	if ext != ".json" && ext != ".pbf" {
		return nil, "", errNoTile
	}
	t, err := tiles.Parse(z, x, file, ext)
	if !ok || err != nil || t.Z < ov.minZoom || t.Z > ov.source.MaxZoom {
		return nil, "", errNoTile
	}
	b, err := ov.source.Get(ctx, t)
	if err != nil {
		a.log.Warn("overlay tile", "overlay", ov.name, "z", t.Z, "x", t.X, "y", t.Y, "err", err)
		return nil, "", err
	}
	if ext == ".pbf" {
		raw, err := mvt.Inflate(b)
		if err != nil {
			a.log.Warn("overlay inflate", "overlay", ov.name, "z", t.Z, "x", t.X, "y", t.Y, "err", err)
		}
		return raw, mvtContentType, err
	}
	layers, err := mvt.Decode(b, t.Z, t.X, t.Y, ov.kind.Layer)
	if err != nil {
		a.log.Warn("overlay decode", "overlay", ov.name, "z", t.Z, "x", t.X, "y", t.Y, "err", err)
		return nil, "", err
	}
	body, err := json.Marshal(struct {
		Features []overlays.Feature `json:"features"`
	}{ov.kind.Collect(layers)})
	return body, "application/json", err
}

const mvtContentType = "application/vnd.mapbox-vector-tile"
