package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/tiles"
)

func TestPositronTiles(t *testing.T) {
	a, h, key := apiApp(t)

	if res, _ := apiDo(t, h, http.MethodGet, apiV1+"/tiles/positron/1/0/0.png", key); res.StatusCode != http.StatusNotFound {
		t.Errorf("tile without key = %d, want 404", res.StatusCode)
	}
	if c := apiGet[APIMapConfig](t, h, key, apiV1+"/map/config"); c.Positron != nil {
		t.Errorf("map config offers Positron without a key: %+v", c.Positron)
	}

	const cartoKey = "carto-secret-key"
	var gotKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.URL.Query().Get("key")
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("png"))
	}))
	defer upstream.Close()
	a.positron = tiles.NewPositron(t.TempDir(), cartoKey)
	a.positron.Upstream = func(context.Context, tiles.Tile) (string, error) { return upstream.URL + "/tile.png", nil }

	res, body := apiDo(t, h, http.MethodGet, apiV1+"/tiles/positron/1/0/0@2x.png", key)
	if res.StatusCode != http.StatusOK || string(body) != "png" || res.Header.Get("Content-Type") != "image/png" || res.Header.Get("Cache-Control") != "private, max-age=86400" {
		t.Errorf("tile = %d %q %v", res.StatusCode, body, res.Header)
	}
	if gotKey != cartoKey {
		t.Errorf("upstream key = %q, want %q", gotKey, cartoKey)
	}
	if res, _ := apiDo(t, h, http.MethodGet, apiV1+"/tiles/positron/1/2/0.png", key); res.StatusCode != http.StatusNotFound {
		t.Errorf("out of range tile = %d, want 404", res.StatusCode)
	}

	_, cfg := apiDo(t, h, http.MethodGet, apiV1+"/map/config", key)
	if !strings.Contains(string(cfg), `"positron":{"url":"/api/v1/tiles/positron/{z}/{x}/{y}{r}.png"`) {
		t.Errorf("map config does not offer Positron with a key:\n%s", cfg)
	}
	if strings.Contains(string(cfg), cartoKey) || strings.Contains(string(cfg), "cartocdn") {
		t.Error("map config exposes the CARTO key or upstream")
	}
}

func TestAPITerrainTiles(t *testing.T) {
	a, h, key := apiApp(t)
	var hits int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "image/webp")
		w.Write([]byte("webp"))
	}))
	defer upstream.Close()
	a.terrain = tiles.NewTerrain(t.TempDir())
	a.terrain.Upstream = func(context.Context, tiles.Tile) (string, error) { return upstream.URL + "/tile.webp", nil }

	c := apiGet[APIMapConfig](t, h, key, "/api/v1/map/config")
	if c.Terrain == nil || c.Terrain.URL != "/api/v1/tiles/terrain/{z}/{x}/{y}.webp" || c.Terrain.MaxZoom != tiles.TerrainMaxZoom || c.Terrain.Encoding != "terrarium" {
		t.Errorf("terrain config = %+v", c.Terrain)
	}

	for range 2 {
		res, body := apiDo(t, h, http.MethodGet, "/api/v1/tiles/terrain/3/1/2.webp", key)
		if res.StatusCode != http.StatusOK || string(body) != "webp" || res.Header.Get("Content-Type") != "image/webp" {
			t.Errorf("tile = %d %q %v", res.StatusCode, body, res.Header)
		}
	}
	if hits != 1 {
		t.Errorf("upstream hits = %d, want 1 after a cached read", hits)
	}

	for _, target := range []string{"/api/v1/tiles/terrain/13/0/0.webp", "/api/v1/tiles/terrain/1/2/0.webp", "/api/v1/tiles/terrain/1/0/0.png"} {
		if res, _ := apiDo(t, h, http.MethodGet, target, key); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, res.StatusCode)
		}
	}
}

func TestAPINoiseTiles(t *testing.T) {
	a, h, key := apiApp(t)
	var hits int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path == "/empty" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("png"))
	}))
	defer upstream.Close()
	a.noise = tiles.NewNoise(t.TempDir())
	a.noise.Upstream = func(_ context.Context, tl tiles.Tile) (string, error) {
		if tl.X == 0 {
			return upstream.URL + "/empty", nil
		}
		return upstream.URL + "/tile", nil
	}

	c := apiGet[APIMapConfig](t, h, key, "/api/v1/map/config")
	if c.CyclOSM == nil || c.CyclOSM.URL != "/api/v1/tiles/cyclosm/{z}/{x}/{y}.png" || c.CyclOSM.MaxZoom != tiles.MaxZoom {
		t.Errorf("cyclosm config = %+v", c.CyclOSM)
	}
	if c.Noise == nil || c.Noise.URL != "/api/v1/tiles/noise/{z}/{x}/{y}.png" || c.Noise.MaxZoom != tiles.NoiseMaxZoom {
		t.Errorf("noise config = %+v", c.Noise)
	}

	for range 2 {
		res, body := apiDo(t, h, http.MethodGet, "/api/v1/tiles/noise/3/1/2.png", key)
		if res.StatusCode != http.StatusOK || string(body) != "png" || res.Header.Get("Content-Type") != "image/png" {
			t.Errorf("tile = %d %q %v", res.StatusCode, body, res.Header)
		}
		if res, _ := apiDo(t, h, http.MethodGet, "/api/v1/tiles/noise/3/0/2.png", key); res.StatusCode != http.StatusNotFound {
			t.Errorf("tile without data = %d, want 404", res.StatusCode)
		}
	}
	if hits != 2 {
		t.Errorf("upstream hits = %d, want 2 after cached reads of a tile and a missing tile", hits)
	}

	for _, target := range []string{"/api/v1/tiles/noise/13/1/0.png", "/api/v1/tiles/noise/3/1/2@2x.png", "/api/v1/tiles/noise/3/1/2.webp"} {
		if res, _ := apiDo(t, h, http.MethodGet, target, key); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, res.StatusCode)
		}
	}
}
