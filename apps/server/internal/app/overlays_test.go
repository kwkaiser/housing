package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/mvt"
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

func geojsonServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/geo+json;charset=UTF-8")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func townDoc(code, field, name string) string {
	return fmt.Sprintf(`{"type":"FeatureCollection","features":[{"type":"Feature","properties":{"STATE":"25","%s":"%s","NAME":"%s","AREALAND":10671516},`+
		`"geometry":{"type":"Polygon","coordinates":[[[-71.09,42.37],[-71.05,42.37],[-71.05,42.39],[-71.09,42.37]]]}}]}`, field, code, name)
}

func TestOverlayTiles(t *testing.T) {
	a, h, key := apiApp(t)
	names := func() []string {
		t.Helper()
		var out []string
		for _, o := range apiGet[APIMapConfig](t, h, key, apiV1+"/map/config").Overlays {
			out = append(out, fmt.Sprintf("%s:%d-%d", o.Name, o.MinZoom, o.MaxZoom))
		}
		return out
	}

	if res, _ := apiDo(t, h, http.MethodGet, apiV1+"/overlays/bus/14/4958/6060.json", key); res.StatusCode != http.StatusNotFound {
		t.Errorf("bus without key = %d, want 404", res.StatusCode)
	}
	if got := strings.Join(names(), ","); got != "towns:10-12,bike:13-14" {
		t.Errorf("map config without keys = %s, want only towns and bike paths", got)
	}

	line := [][][2]int{{{100, 100}, {2000, 2000}}}
	transit := mvttest.Tile(mvttest.Layer{Name: "routes", Features: []mvttest.Feature{
		{ID: 1, Type: 2, Lines: line, Props: map[string]any{"onestop_id": "r-87", "route_type": 3, "route_short_name": "87", "headway_secs": 900}},
		{ID: 2, Type: 2, Lines: line, Props: map[string]any{"onestop_id": "r-red", "route_type": 1, "route_long_name": "Red Line"}},
	}})
	bike := mvttest.Tile(
		mvttest.Layer{Name: "transportation_name", Features: []mvttest.Feature{
			{ID: 1, Type: 2, Lines: line, Props: map[string]any{"class": "path", "subclass": "cycleway", "name": "Somerville Community Path"}},
		}},
		mvttest.Layer{Name: "transportation", Features: []mvttest.Feature{
			{ID: 2, Type: 2, Lines: line, Props: map[string]any{"class": "path", "subclass": "cycleway"}},
		}},
	)

	var gotKey string
	transitSrv := vectorServer(t, transit, &gotKey)
	bikeSrv := vectorServer(t, bike, nil)
	tl := tiles.NewTransitland(t.TempDir(), "tl-secret")
	tl.Upstream = func(context.Context, tiles.Tile) (string, error) { return transitSrv.URL + "/routes.pbf", nil }
	ofm := tiles.NewOpenFreeMap(t.TempDir())
	ofm.Upstream = func(context.Context, tiles.Tile) (string, error) { return bikeSrv.URL + "/bike.pbf", nil }
	placesSrv := geojsonServer(t, townDoc("62535", "PLACE", "Somerville city"))
	townshipsSrv := geojsonServer(t, townDoc("62535", "COUSUB", "Somerville city"))
	places := tiles.NewPlaces(t.TempDir())
	places.Upstream = func(context.Context, tiles.Tile) (string, error) { return placesSrv.URL + "/places", nil }
	townships := tiles.NewTownships(t.TempDir())
	townships.Upstream = func(context.Context, tiles.Tile) (string, error) { return townshipsSrv.URL + "/townships", nil }
	a.overlays = newOverlays(tl, ofm, places, townships)

	ids := func(target string) []string {
		t.Helper()
		res, body := apiDo(t, h, http.MethodGet, target, key)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("GET %s = %d %q\n%s", target, res.StatusCode, res.Header.Get("Content-Type"), body)
		}
		var doc struct{ Features []overlays.Feature }
		if err := json.Unmarshal(body, &doc); err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, f := range doc.Features {
			p := firstPoint(f)
			if p[0] < 42.3 || p[0] > 42.4 || p[1] < -71.1 || p[1] > -71.0 {
				t.Errorf("GET %s: %s point = %v, want near Boston", target, f.ID, p)
			}
			out = append(out, f.ID)
		}
		return out
	}
	for target, want := range map[string]string{
		apiV1 + "/overlays/bus/14/4958/6060.json":   "r-87",
		apiV1 + "/overlays/rail/14/4958/6060.json":  "r-red",
		apiV1 + "/overlays/bike/14/4958/6060.json":  "bikeway,bike:Somerville Community Path",
		apiV1 + "/overlays/towns/12/1239/1515.json": "town:2562535",
	} {
		if got := ids(target); strings.Join(got, ",") != want {
			t.Errorf("GET %s = %v, want %s", target, got, want)
		}
	}
	if gotKey != "tl-secret" {
		t.Errorf("upstream apikey = %q", gotKey)
	}

	for _, target := range []string{"/overlays/bus/11/619/757.json", "/overlays/bus/15/9916/12120.json", "/overlays/bike/12/1239/1515.json", "/overlays/nope/14/4958/6060.json", "/overlays/bus/14/4958/6060.png", "/overlays/towns/13/2478/3030.json", "/overlays/towns/9/154/189.json", "/overlays/towns/12/1239/1515.pbf"} {
		if res, _ := apiDo(t, h, http.MethodGet, apiV1+target, key); res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("GET %s = %d, want 404 or 422", target, res.StatusCode)
		}
	}

	if got := strings.Join(names(), ","); got != "towns:10-12,bike:13-14,rail:9-14,bus:12-14" {
		t.Errorf("map config with a Transitland key = %s, want towns, bike, rail and bus", got)
	}
	for _, o := range apiGet[APIMapConfig](t, h, key, apiV1+"/map/config").Overlays {
		if vector := o.VectorURL != ""; vector != (o.Name != "towns") {
			t.Errorf("overlay %s vector_url = %q", o.Name, o.VectorURL)
		}
	}
	_, cfg := apiDo(t, h, http.MethodGet, apiV1+"/map/config", key)
	if strings.Contains(string(cfg), "tl-secret") || strings.Contains(string(cfg), "transit.land/api") {
		t.Error("map config exposes the Transitland key or upstream")
	}
}

func firstPoint(f overlays.Feature) mvt.LatLng {
	if len(f.Polygons) > 0 {
		return f.Polygons[0][0][0]
	}
	return f.Lines[0][0]
}
