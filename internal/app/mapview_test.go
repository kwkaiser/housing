package app

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func TestMapPage(t *testing.T) {
	a := seededApp(t)
	ctx := t.Context()
	db := a.svc.Store()
	attic, err := db.EffectiveProfile(ctx, "attic")
	if err != nil {
		t.Fatal(err)
	}
	shared := assessed(rental(listing.SourceZillow, "z4", "5 New St", 250000), attic, 60)
	located := rental(listing.SourceZillow, "z9", "1 Exact Way", 280000)
	located.Coordinates = &listing.Coordinates{Lat: 42.39, Lng: -71.1}
	nowhere := rental(listing.SourceZillow, "z8", "Somewhere", 270000)
	nowhere.Address = listing.Address{Formatted: "Somewhere"}
	if err := db.Observe(ctx, day2, "zempty", []listing.Listing{shared, assessed(located, attic, 50), assessed(nowhere, attic, 40)}); err != nil {
		t.Fatal(err)
	}
	a.now = func() time.Time { return time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC) }
	h := a.Handler()

	for _, tc := range []struct {
		target   string
		contains []string
		missing  []string
	}{
		{"/map", []string{
			"<title>Map · housing</title>",
			`<a href="/map" aria-current="page">Map</a>`,
			`/static/leaflet/leaflet.css?v=`, `/static/leaflet/leaflet.js?v=`, `/static/map.js?v=`,
			"4 listings placed · 3 approximate from zip code · 1 without a location",
			`<details class="picker collections" hidden>`,
			`<input type="checkbox" data-collection="somerville" data-color="#2563eb" checked>`,
			`<span class="swatch" style="background: #2563eb"></span> somerville <span class="muted">3 · 2026-09-30</span>`,
			`<input type="checkbox" data-collection="zempty" data-color="#dc2626" checked>`,
			`zempty <span class="muted">2 · 2026-09-30</span>`,
			`<input type="date" name="from" value="2026-09-15" min="2026-09-15" max="2026-09-30"`,
			`<select class="quick" aria-label="Quick">`, `<option value="" disabled>Quick</option>`,
			`<option value="/map?from=2026-10-14">Last day</option>`, `<option value="/map?from=2026-10-08">Last week</option>`, `<option value="/map?from=2026-10-01">Last 2 weeks</option>`,
			`<option value="/map" selected>Last month</option>`, `<option value="/map?from=">All dates</option>`,
			`<select class="fading" aria-label="Fading" hidden>`, `<label class="toggle contrast" hidden>Contrast <input type="range" min="0" max="100" value="50" aria-label="Fading contrast"></label>`, `<option value="age" selected>Fade by age</option>`, `<option value="match">Fade by match</option>`,
			`"title":"5 New St, Somerville, MA 02144","href":"/listing/zillow/z4?collection=somerville\u0026day=2026-09-30","price":"$2,500/mo","beds":"2","matches":{"somerville":"75%","zempty":"`, `"scores":{"somerville":75,"zempty":`,
			`"firstSeen":"2026-09-30","collections":["somerville","zempty"]`,
			`"title":"7 Windom St, Somerville, MA 02144"`, `"firstSeen":"2026-09-29","preview":"/media/` + collageKey + `"`,
			`{"lat":42.39,"lng":-71.1,"title":"1 Exact Way`,
		}, []string{"9 Carpet Ct", `selected>All dates`, "outside the date range"}},
		{"/map?from=2026-09-30", []string{
			"2 listings placed · 1 approximate from zip code · 2 outside the date range · 1 without a location",
			`<input type="date" name="from" value="2026-09-30"`, `<option value="" disabled selected>Quick</option>`, `<option value="/map">Last month</option>`,
			`somerville <span class="muted">1 · 2026-09-30</span>`, `zempty <span class="muted">2 · 2026-09-30</span>`,
		}, []string{"7 Windom St", "31 Fairmount Ave"}},
		{"/map?from=2026-09-30&to=2026-09-29", []string{
			"4 listings placed",
			`name="from" value="2026-09-29"`, `name="to" value="2026-09-30"`,
		}, []string{"outside the date range"}},
		{"/map?to=2026-09-29", []string{"2 listings placed · 2 approximate from zip code · 2 outside the date range"}, []string{"5 New St", "1 Exact Way"}},
		{"/map?from=bogus", []string{"4 listings placed", `name="from" value=""`, `<option value="/map?from=" selected>All dates</option>`}, nil},
		{"/map?from=", []string{"4 listings placed", `name="from" value="" min="2026-09-29" max="2026-09-30"`, `<option value="/map?from=" selected>All dates</option>`}, []string{`selected>Last month`, "outside the date range"}},
		{"/map?from=2026-10-08", []string{`<option value="/map?from=2026-10-08" selected>Last week</option>`, "0 listings placed"}, []string{`selected>Last month`, `disabled selected>Quick`}},
		{"/map?from=2026-10-05", []string{
			"0 listings placed · 4 outside the date range", `/static/map.js?v=`, `/static/leaflet/leaflet.js?v=`, `<option value="" disabled selected>Quick</option>`,
			`<div id="map" class="map"></div>`, `<script type="application/json" id="map-pins">[]</script>`,
			`<script type="application/json" id="map-extent">{"somerville":[[42.4001,-71.1231],[42.4001,-71.1231]],"zempty":[[42.39,-71.1231],[42.4001,-71.1]]}</script>`,
		}, []string{"No listings were first seen"}},
	} {
		res, body := get(t, h, tc.target)
		if res.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d\n%s", tc.target, res.StatusCode, body)
			continue
		}
		for _, s := range tc.contains {
			if !strings.Contains(body, s) {
				t.Errorf("GET %s: missing %q", tc.target, s)
			}
		}
		for _, s := range tc.missing {
			if strings.Contains(body, s) {
				t.Errorf("GET %s: unexpected %q", tc.target, s)
			}
		}
	}

	a.now = func() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }
	_, body := get(t, h, "/map")
	for _, s := range []string{
		"2 listings placed · 1 approximate from zip code · 2 outside the date range · 1 without a location",
		`name="from" value="2026-09-30" min="2026-09-29" max="2026-09-30"`,
	} {
		if !strings.Contains(body, s) {
			t.Errorf("GET /map on 2026-10-30: missing %q", s)
		}
	}

	empty, _ := newApp(t)
	_, body = get(t, empty.Handler(), "/map")
	if !strings.Contains(body, "There are no collections yet.") || strings.Contains(body, "leaflet") {
		t.Errorf("GET /map without collections:\n%s", body)
	}
}
