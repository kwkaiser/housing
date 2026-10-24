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
	a := seededApp(t)
	h := a.Handler()

	res, _ := get(t, h, "/tiles/positron/1/0/0.png")
	if res.StatusCode != http.StatusNotFound {
		t.Errorf("tile without key = %d, want 404", res.StatusCode)
	}
	if _, body := get(t, h, "/map"); !strings.Contains(body, `"positron":false`) {
		t.Error("map offers Positron without a key")
	}

	const key = "carto-secret-key"
	var gotKey string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.URL.Query().Get("key")
		w.Header().Set("Content-Type", "image/png")
		w.Write([]byte("png"))
	}))
	defer upstream.Close()
	a.positron = tiles.NewPositron(t.TempDir(), key)
	a.positron.Upstream = func(context.Context, tiles.Tile) (string, error) { return upstream.URL + "/tile.png", nil }

	res, body := get(t, h, "/tiles/positron/1/0/0@2x.png")
	if res.StatusCode != http.StatusOK || body != "png" || res.Header.Get("Content-Type") != "image/png" || res.Header.Get("Cache-Control") != "public, max-age=86400" {
		t.Errorf("tile = %d %q %v", res.StatusCode, body, res.Header)
	}
	if gotKey != key {
		t.Errorf("upstream key = %q, want %q", gotKey, key)
	}
	if res, _ := get(t, h, "/tiles/positron/1/2/0.png"); res.StatusCode != http.StatusNotFound {
		t.Errorf("out of range tile = %d, want 404", res.StatusCode)
	}

	_, page := get(t, h, "/map")
	if !strings.Contains(page, `"positron":true`) {
		t.Error("map does not offer Positron with a key")
	}
	_, js := get(t, h, a.staticURL("map.js"))
	_, layers := get(t, h, a.staticURL("map-layers.js"))
	for name, s := range map[string]string{"map page": page, "map.js": js, "map-layers.js": layers} {
		if strings.Contains(s, key) || strings.Contains(s, "cartocdn") {
			t.Errorf("%s exposes the CARTO key or upstream", name)
		}
	}
}
