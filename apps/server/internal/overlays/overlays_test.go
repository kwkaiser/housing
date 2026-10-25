package overlays

import (
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/mvt"
)

func line(dlng float64) [][]mvt.LatLng {
	return [][]mvt.LatLng{{{42.123456789, -71.1 + dlng}, {42.2, -71.0 + dlng}}}
}

func route(props map[string]any, dlng float64) mvt.Feature {
	return mvt.Feature{Type: mvt.LineString, Props: props, Parts: line(dlng)}
}

func TestTransit(t *testing.T) {
	layers := []mvt.Layer{
		{Name: "routes", Features: []mvt.Feature{
			route(map[string]any{"onestop_id": "r-87", "route_type": 3.0, "route_short_name": "87", "route_long_name": "Arlington Center - Lechmere", "agency_name": "MBTA", "headway_secs": 900.0, "route_color": "FFC72C"}, 0),
			route(map[string]any{"onestop_id": "r-87", "route_type": 3.0, "route_short_name": "87", "route_long_name": "Arlington Center - Lechmere", "agency_name": "MBTA", "headway_secs": 900.0}, 0.01),
			route(map[string]any{"onestop_id": "r-shuttle", "route_type": 3.0, "route_short_name": "Red Line Shuttle", "headway_secs": 100000.0}, 0),
			route(map[string]any{"onestop_id": "r-trolley", "route_type": "11", "route_long_name": "Trackless trolley"}, 0),
			route(map[string]any{"onestop_id": "r-red", "route_type": 1.0, "route_long_name": "Red Line", "route_color": "#DA291C", "headway_secs": 240.0}, 0),
			route(map[string]any{"onestop_id": "r-b", "route_type": 0.0, "route_short_name": "B", "route_long_name": "Green Line B", "route_color": "00843d"}, 0),
			route(map[string]any{"onestop_id": "r-amtrak", "route_type": 2.0, "route_long_name": "Downeaster", "headway_secs": 100000.0, "route_color": "#cae4f1"}, 0),
			route(map[string]any{"onestop_id": "r-ferry", "route_type": 4.0, "route_long_name": "Ferry"}, 0),
			route(map[string]any{"route_type": 3.0, "route_short_name": "no id"}, 0),
			{Type: mvt.Point, Props: map[string]any{"onestop_id": "r-pt", "route_type": 3.0}, Parts: [][]mvt.LatLng{{{42, -71}}}},
		}},
		{Name: "other", Features: []mvt.Feature{route(map[string]any{"onestop_id": "r-x", "route_type": 3.0}, 0)}},
	}

	bus := Bus.Collect(layers)
	if len(bus) != 2 || bus[0].ID != "r-87" || bus[1].ID != "r-trolley" {
		t.Fatalf("bus = %+v", bus)
	}
	b := bus[0]
	if b.Label != "87 · Arlington Center - Lechmere" || b.Detail != "MBTA" || b.Color != "#ffc72c" || b.Headway != 900 || len(b.Lines) != 2 {
		t.Errorf("bus 87 = %+v", b)
	}
	if got := b.Lines[0][0]; got != (mvt.LatLng{42.12346, -71.1}) {
		t.Errorf("rounded point = %v", got)
	}

	rail := Rail.Collect(layers)
	want := map[string]Feature{
		"r-amtrak": {Label: "Downeaster"},
		"r-b":      {Label: "Green Line B", Color: "#00843d"},
		"r-red":    {Label: "Red Line", Color: "#da291c", Headway: 240},
	}
	if len(rail) != len(want) {
		t.Fatalf("rail = %+v", rail)
	}
	for _, f := range rail {
		w := want[f.ID]
		if f.Label != w.Label || f.Color != w.Color || f.Headway != w.Headway {
			t.Errorf("rail %s = %+v, want %+v", f.ID, f, w)
		}
	}
}

func TestBike(t *testing.T) {
	layers := []mvt.Layer{{Name: "transportation_name", Features: []mvt.Feature{
		route(map[string]any{"class": "path", "subclass": "cycleway", "name": "Somerville Community Path"}, 0),
		route(map[string]any{"class": "path", "subclass": "footway", "name": "Somerville Community Path"}, 0.01),
		route(map[string]any{"class": "path", "subclass": "path", "name": "Minuteman Bikeway"}, 0),
		route(map[string]any{"class": "path", "subclass": "footway", "name": "Glover Circle"}, 0),
		route(map[string]any{"class": "path", "subclass": "steps", "name": "Memorial Steps Trail"}, 0),
		route(map[string]any{"class": "path", "subclass": "cycleway"}, 0),
		route(map[string]any{"class": "primary", "name": "Broadway"}, 0),
	}}}
	got := Bike.Collect(layers)
	if len(got) != 2 || got[0].ID != "bike:Minuteman Bikeway" || got[1].ID != "bike:Somerville Community Path" || len(got[1].Lines) != 2 || got[1].Detail != "Bike path" {
		t.Errorf("bike = %+v", got)
	}
}

func TestBikePaths(t *testing.T) {
	layers := []mvt.Layer{{Name: "transportation", Features: []mvt.Feature{
		route(map[string]any{"class": "path", "subclass": "cycleway"}, 0),
		route(map[string]any{"class": "path", "subclass": "footway", "bicycle": "designated"}, 0.01),
		route(map[string]any{"class": "path", "subclass": "footway", "bicycle": "yes"}, 0),
		route(map[string]any{"class": "primary", "bicycle": "designated"}, 0),
	}}}
	got := BikePaths.Collect(layers)
	if len(got) != 1 || got[0].ID != "bikeway" || got[0].Label != "Bike path" || len(got[0].Lines) != 2 {
		t.Errorf("bike paths = %+v", got)
	}
}

func TestRouteLabel(t *testing.T) {
	for _, tc := range []struct{ short, long, want string }{
		{"87", "Arlington Center - Lechmere", "87 · Arlington Center - Lechmere"},
		{"E", "Green Line E", "Green Line E"},
		{"", "Red Line", "Red Line"},
		{"CT2", "", "CT2"},
		{"", "", "Route"},
	} {
		if got := routeLabel(tc.short, tc.long); got != tc.want {
			t.Errorf("routeLabel(%q, %q) = %q, want %q", tc.short, tc.long, got, tc.want)
		}
	}
}
