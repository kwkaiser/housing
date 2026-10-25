package app

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestAPIPlaces(t *testing.T) {
	a, h, key := apiApp(t)
	var query string
	photon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		if r.URL.Query().Get("q") == "down" {
			http.Error(w, "nope", http.StatusServiceUnavailable)
			return
		}
		w.Write([]byte(`{"features":[{"properties":{"housenumber":"2","street":"Elm Street","city":"Somerville","state":"Massachusetts","postcode":"02144","country":"United States"},"geometry":{"coordinates":[-71.12,42.39]}}]}`))
	}))
	defer photon.Close()
	a.geocoder.URL, a.geocoder.Client = photon.URL, photon.Client()

	ms := apiGet[[]APIPlaceMatch](t, h, key, "/api/v1/places/search?q=2+elm&lat=42.4&lng=-71.1")
	if len(ms) != 1 || ms[0] != (APIPlaceMatch{Address: "2 Elm Street, Somerville, Massachusetts 02144", Lat: 42.39, Lng: -71.12}) {
		t.Errorf("search = %+v", ms)
	}
	if query == "" || !strings.Contains(query, "lat=42.40000") {
		t.Errorf("upstream query = %q", query)
	}
	if res, b := apiDo(t, h, http.MethodGet, "/api/v1/places/search?q=down", key); res.StatusCode != http.StatusBadGateway {
		t.Errorf("upstream down = %d\n%s", res.StatusCode, b)
	}

	res, b := apiSend(t, h, http.MethodPost, "/api/v1/places", key, map[string]any{"name": " Sam ", "address": ms[0].Address, "lat": ms[0].Lat, "lng": ms[0].Lng})
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("add = %d\n%s", res.StatusCode, b)
	}
	p := decode[APIPlace](t, b)
	if p.ID == 0 || p.Name != "Sam" || p.Lat != 42.39 || p.AddedAt.IsZero() {
		t.Errorf("added = %+v", p)
	}
	for _, bad := range []map[string]any{
		{"name": "  ", "address": "x", "lat": 1, "lng": 1},
		{"name": "x", "address": "x", "lat": 91, "lng": 1},
	} {
		if res, b := apiSend(t, h, http.MethodPost, "/api/v1/places", key, bad); res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("add %v = %d\n%s", bad, res.StatusCode, b)
		}
	}

	if ps := apiGet[[]APIPlace](t, h, key, "/api/v1/places"); len(ps) != 1 || ps[0].ID != p.ID {
		t.Errorf("places = %+v", ps)
	}
	target := "/api/v1/places/" + strconv.FormatInt(p.ID, 10)
	if res, b := apiDo(t, h, http.MethodDelete, target, key); res.StatusCode != http.StatusNoContent {
		t.Errorf("remove = %d\n%s", res.StatusCode, b)
	}
	if res, _ := apiDo(t, h, http.MethodDelete, target, key); res.StatusCode != http.StatusNotFound {
		t.Errorf("remove again = %d", res.StatusCode)
	}
	if ps := apiGet[[]APIPlace](t, h, key, "/api/v1/places"); len(ps) != 0 {
		t.Errorf("places after remove = %+v", ps)
	}
}
