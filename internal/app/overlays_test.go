package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/mvt/mvttest"
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
	if _, body := get(t, h, "/map"); strings.Contains(body, "data-transit") {
		t.Error("map offers transit without a key")
	}

	line := func(dx int) [][2]int { return [][2]int{{100 + dx, 100}, {2000 + dx, 2000}} }
	transit := mvttest.Tile(mvttest.Layer{Name: "routes", Features: []mvttest.Feature{
		{ID: 1, Type: 2, Lines: [][][2]int{line(0)}, Props: map[string]any{"onestop_id": "r-87", "route_type": 3, "route_short_name": "87", "route_long_name": "Arlington Center - Lechmere", "agency_name": "MBTA", "headway_secs": 900, "route_color": "FFC72C"}},
		{ID: 2, Type: 2, Lines: [][][2]int{line(50)}, Props: map[string]any{"onestop_id": "r-87", "route_type": 3, "route_short_name": "87", "route_long_name": "Arlington Center - Lechmere", "agency_name": "MBTA", "headway_secs": 900, "route_color": "FFC72C"}},
		{ID: 3, Type: 2, Lines: [][][2]int{line(10)}, Props: map[string]any{"onestop_id": "r-red", "route_type": 1, "route_long_name": "Red Line", "agency_name": "MBTA", "route_color": "#DA291C"}},
		{ID: 4, Type: 2, Lines: [][][2]int{line(20)}, Props: map[string]any{"onestop_id": "r-ferry", "route_type": 4, "route_long_name": "Ferry"}},
		{ID: 5, Type: 1, Lines: [][][2]int{{{5, 5}}}, Props: map[string]any{"onestop_id": "r-pt", "route_type": 3}},
		{ID: 6, Type: 2, Lines: [][][2]int{line(30)}, Props: map[string]any{"onestop_id": "r-shuttle", "route_type": 3, "route_short_name": "Red Line Shuttle", "headway_secs": 100000}},
		{ID: 7, Type: 2, Lines: [][][2]int{line(40)}, Props: map[string]any{"onestop_id": "r-amtrak", "route_type": 2, "route_long_name": "Downeaster", "agency_name": "Amtrak", "headway_secs": 100000, "route_color": "#cae4f1"}},
	}})
	bike := mvttest.Tile(mvttest.Layer{Name: "transportation_name", Features: []mvttest.Feature{
		{ID: 1, Type: 2, Lines: [][][2]int{line(0)}, Props: map[string]any{"class": "path", "subclass": "cycleway", "name": "Somerville Community Path"}},
		{ID: 2, Type: 2, Lines: [][][2]int{line(30)}, Props: map[string]any{"class": "path", "subclass": "footway", "name": "Somerville Community Path"}},
		{ID: 3, Type: 2, Lines: [][][2]int{line(60)}, Props: map[string]any{"class": "path", "subclass": "footway", "name": "Glover Circle"}},
		{ID: 4, Type: 2, Lines: [][][2]int{line(90)}, Props: map[string]any{"class": "primary", "name": "Broadway"}},
	}})

	var gotKey string
	transitSrv := vectorServer(t, transit, &gotKey)
	bikeSrv := vectorServer(t, bike, nil)
	tl := tiles.NewTransitland(t.TempDir(), "tl-secret")
	tl.Upstream = func(context.Context, tiles.Tile) (string, error) { return transitSrv.URL + "/routes.pbf", nil }
	ofm := tiles.NewOpenFreeMap(t.TempDir())
	ofm.Upstream = func(context.Context, tiles.Tile) (string, error) { return bikeSrv.URL + "/bike.pbf", nil }
	a.overlays = newOverlays(tl, ofm)

	decode := func(target string) []overlayFeature {
		t.Helper()
		res, body := get(t, h, target)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("GET %s = %d %q\n%s", target, res.StatusCode, res.Header.Get("Content-Type"), body)
		}
		var doc struct{ Features []overlayFeature }
		if err := json.Unmarshal([]byte(body), &doc); err != nil {
			t.Fatal(err)
		}
		return doc.Features
	}

	bus := decode("/overlays/bus/14/4958/6060.json")
	if len(bus) != 1 {
		t.Fatalf("bus features = %+v", bus)
	}
	b := bus[0]
	if b.ID != "r-87" || b.Label != "87 · Arlington Center - Lechmere" || b.Detail != "MBTA" || b.Color != "#ffc72c" || b.Headway != 900 || len(b.Lines) != 2 {
		t.Errorf("bus feature = %+v", b)
	}
	if p := b.Lines[0][0]; p[0] < 42.3 || p[0] > 42.4 || p[1] < -71.1 || p[1] > -71.0 {
		t.Errorf("bus point = %v, want near Boston", p)
	}
	if gotKey != "tl-secret" {
		t.Errorf("upstream apikey = %q", gotKey)
	}

	rail := decode("/overlays/rail/14/4958/6060.json")
	if len(rail) != 2 || rail[0].ID != "r-amtrak" || rail[0].Color != "" || rail[0].Headway != 0 || rail[1].ID != "r-red" || rail[1].Label != "Red Line" || rail[1].Color != "#da291c" {
		t.Errorf("rail features = %+v", rail)
	}

	bikes := decode("/overlays/bike/14/4958/6060.json")
	if len(bikes) != 1 || bikes[0].ID != "bike:Somerville Community Path" || bikes[0].Label != "Somerville Community Path" || len(bikes[0].Lines) != 2 {
		t.Errorf("bike features = %+v", bikes)
	}

	for _, target := range []string{"/overlays/bus/11/619/757.json", "/overlays/bus/15/9916/12120.json", "/overlays/bike/12/1239/1515.json", "/overlays/nope/14/4958/6060.json", "/overlays/bus/14/4958/6060.png"} {
		if res, _ := get(t, h, target); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, res.StatusCode)
		}
	}

	_, page := get(t, h, "/map")
	if !strings.Contains(page, "data-transit") {
		t.Error("map does not offer transit with a key")
	}
	_, js := get(t, h, a.staticURL("map.js"))
	for name, s := range map[string]string{"map page": page, "map.js": js} {
		if strings.Contains(s, "tl-secret") || strings.Contains(s, "transit.land/api") {
			t.Errorf("%s exposes the Transitland key or upstream", name)
		}
	}
}
