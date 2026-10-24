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
