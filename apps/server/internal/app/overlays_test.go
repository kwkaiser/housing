package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/mvt/mvttest"
	"git.kwkaiser.io/kwkaiser/housing/internal/overlays"
	"git.kwkaiser.io/kwkaiser/housing/internal/tiles"
)

func vectorServer(t *testing.T, body []byte, gotKey *string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if gotKey != nil {
			*gotKey = r.Header.Get("apikey")
		}
		w.Header().Set("Content-Type", "application/vnd.mapbox-vector-tile")
		w.Write(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestOverlayTiles(t *testing.T) {
	a := seededApp(t)
	h := a.Handler()

	if res, _ := get(t, h, "/overlays/bus/14/4958/6060.json"); res.StatusCode != http.StatusNotFound {
		t.Errorf("bus without key = %d, want 404", res.StatusCode)
	}
	if _, body := get(t, h, "/map"); !strings.Contains(body, `id="map-config">{"positron":false,"overlays":[{"name":"bike","minZoom":13,"maxZoom":14}]}</script>`) {
		t.Error("map config without keys should only offer bike paths")
	}

	line := [][][2]int{{{100, 100}, {2000, 2000}}}
	transit := mvttest.Tile(mvttest.Layer{Name: "routes", Features: []mvttest.Feature{
		{ID: 1, Type: 2, Lines: line, Props: map[string]any{"onestop_id": "r-87", "route_type": 3, "route_short_name": "87", "headway_secs": 900}},
		{ID: 2, Type: 2, Lines: line, Props: map[string]any{"onestop_id": "r-red", "route_type": 1, "route_long_name": "Red Line"}},
	}})
	bike := mvttest.Tile(mvttest.Layer{Name: "transportation_name", Features: []mvttest.Feature{
		{ID: 1, Type: 2, Lines: line, Props: map[string]any{"class": "path", "subclass": "cycleway", "name": "Somerville Community Path"}},
	}})

	var gotKey string
	transitSrv := vectorServer(t, transit, &gotKey)
	bikeSrv := vectorServer(t, bike, nil)
	tl := tiles.NewTransitland(t.TempDir(), "tl-secret")
	tl.Upstream = func(context.Context, tiles.Tile) (string, error) { return transitSrv.URL + "/routes.pbf", nil }
	ofm := tiles.NewOpenFreeMap(t.TempDir())
	ofm.Upstream = func(context.Context, tiles.Tile) (string, error) { return bikeSrv.URL + "/bike.pbf", nil }
	a.overlays = newOverlays(tl, ofm)

	ids := func(target string) []string {
		t.Helper()
		res, body := get(t, h, target)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("GET %s = %d %q\n%s", target, res.StatusCode, res.Header.Get("Content-Type"), body)
		}
		var doc struct{ Features []overlays.Feature }
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, f := range doc.Features {
			if p := f.Lines[0][0]; p[0] < 42.3 || p[0] > 42.4 || p[1] < -71.1 || p[1] > -71.0 {
				t.Errorf("GET %s: %s point = %v, want near Boston", target, f.ID, p)
			}
			out = append(out, f.ID)
		}
		return out
	}
	for target, want := range map[string]string{
		"/overlays/bus/14/4958/6060.json":  "r-87",
		"/overlays/rail/14/4958/6060.json": "r-red",
		"/overlays/bike/14/4958/6060.json": "bike:Somerville Community Path",
	} {
		if got := ids(target); strings.Join(got, ",") != want {
			t.Errorf("GET %s = %v, want %s", target, got, want)
		}
	}
	if gotKey != "tl-secret" {
		t.Errorf("upstream apikey = %q", gotKey)
	}

	for _, target := range []string{"/overlays/bus/11/619/757.json", "/overlays/bus/15/9916/12120.json", "/overlays/bike/12/1239/1515.json", "/overlays/nope/14/4958/6060.json", "/overlays/bus/14/4958/6060.png"} {
		if res, _ := get(t, h, target); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, res.StatusCode)
		}
	}

	_, page := get(t, h, "/map")
	if !strings.Contains(page, `"overlays":[{"name":"bike","minZoom":13,"maxZoom":14},{"name":"rail","minZoom":9,"maxZoom":14},{"name":"bus","minZoom":12,"maxZoom":14}]`) {
		t.Error("map config with a Transitland key should offer bike, rail and bus")
	}
	for _, path := range []string{"map.js", "map-layers.js", "vector-overlay.js"} {
		_, js := get(t, h, a.staticURL(path))
		for name, s := range map[string]string{"map page": page, path: js} {
			if strings.Contains(s, "tl-secret") || strings.Contains(s, "transit.land/api") {
				t.Errorf("%s exposes the Transitland key or upstream", name)
			}
		}
	}
}
