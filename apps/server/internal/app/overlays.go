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
	sources []*tiles.Cache
	minZoom int
	collect func(t tiles.Tile, data [][]byte) ([]overlays.Feature, error)
}

type overlayConfig struct {
	Name    string
	MinZoom int
	MaxZoom int
	Vector  bool
}

type mapConfig struct {
	Positron bool
	Overlays []overlayConfig
}

func newOverlays(transit, openFreeMap, places, townships *tiles.Cache) []overlay {
	return []overlay{
		{name: "towns", sources: []*tiles.Cache{places, townships}, minZoom: 10, collect: towns},
		{name: "bike", sources: []*tiles.Cache{openFreeMap}, minZoom: 13, collect: vector(overlays.BikePaths, overlays.Bike)},
		{name: "rail", sources: []*tiles.Cache{transit}, minZoom: 9, collect: vector(overlays.Rail)},
		{name: "bus", sources: []*tiles.Cache{transit}, minZoom: 12, collect: vector(overlays.Bus)},
	}
}

func vector(kinds ...overlays.Kind) func(tiles.Tile, [][]byte) ([]overlays.Feature, error) {
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = k.Layer
	}
	return func(t tiles.Tile, data [][]byte) ([]overlays.Feature, error) {
		var layers []mvt.Layer
		for _, b := range data {
			ls, err := mvt.Decode(b, t.Z, t.X, t.Y, names...)
			if err != nil {
				return nil, err
			}
			layers = append(layers, ls...)
		}
		var out []overlays.Feature
		for _, k := range kinds {
			out = append(out, k.Collect(layers)...)
		}
		return out, nil
	}
}

func towns(_ tiles.Tile, data [][]byte) ([]overlays.Feature, error) {
	return overlays.Towns(data...)
}

func (o overlay) enabled() bool {
	for _, s := range o.sources {
		if !s.Enabled() {
			return false
		}
	}
	return len(o.sources) > 0
}

func (o overlay) maxZoom() int {
	z := tiles.MaxZoom
	for _, s := range o.sources {
		z = min(z, s.MaxZoom)
	}
	return z
}

func (o overlay) vector() bool {
	return len(o.sources) == 1 && o.sources[0].Ext == ".pbf"
}

func (a *App) overlay(name string) (overlay, bool) {
	for _, ov := range a.overlays {
		if ov.name == name && ov.enabled() {
			return ov, true
		}
	}
	return overlay{}, false
}

func (a *App) mapConfig() mapConfig {
	c := mapConfig{Positron: a.positron.Enabled(), Overlays: []overlayConfig{}}
	for _, ov := range a.overlays {
		if ov.enabled() {
			c.Overlays = append(c.Overlays, overlayConfig{Name: ov.name, MinZoom: ov.minZoom, MaxZoom: ov.maxZoom(), Vector: ov.vector()})
		}
	}
	return c
}

func (a *App) overlayData(ctx context.Context, name, z, x, file string) ([]byte, string, error) {
	ov, ok := a.overlay(name)
	ext := path.Ext(file)
	if ext != ".json" && (ext != ".pbf" || !ov.vector()) {
		return nil, "", errNoTile
	}
	t, err := tiles.Parse(z, x, file, ext)
	if !ok || err != nil || t.Z < ov.minZoom || t.Z > ov.maxZoom() {
		return nil, "", errNoTile
	}
	data := make([][]byte, len(ov.sources))
	for i, s := range ov.sources {
		if data[i], err = s.Get(ctx, t); err != nil {
			a.log.WarnContext(ctx, "overlay tile", "overlay", ov.name, "z", t.Z, "x", t.X, "y", t.Y, "err", err)
			return nil, "", err
		}
	}
	if ext == ".pbf" {
		raw, err := mvt.Inflate(data[0])
		if err != nil {
			a.log.WarnContext(ctx, "overlay inflate", "overlay", ov.name, "z", t.Z, "x", t.X, "y", t.Y, "err", err)
		}
		return raw, mvtContentType, err
	}
	features, err := ov.collect(t, data)
	if err != nil {
		a.log.WarnContext(ctx, "overlay decode", "overlay", ov.name, "z", t.Z, "x", t.X, "y", t.Y, "err", err)
		return nil, "", err
	}
	body, err := json.Marshal(struct {
		Features []overlays.Feature `json:"features"`
	}{features})
	return body, "application/json", err
}

const mvtContentType = "application/vnd.mapbox-vector-tile"
