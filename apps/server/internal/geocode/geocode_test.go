package geocode

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
)

const sample = `{"type":"FeatureCollection","features":[
{"properties":{"type":"house","name":"77 Massachusetts Ave","street":"Massachusetts Avenue","city":"Cambridge","state":"Massachusetts","country":"United States","postcode":"02139"},"geometry":{"type":"Point","coordinates":[-71.09375,42.35941]}},
{"properties":{"type":"house","housenumber":"770","name":"Cambridge Post Office","street":"Massachusetts Avenue","city":"Cambridge","state":"Massachusetts","country":"United States","postcode":"02139"},"geometry":{"type":"Point","coordinates":[-71.10604,42.36634]}},
{"properties":{"type":"house","housenumber":"770","name":"Cambridge Post Office","street":"Massachusetts Avenue","city":"Cambridge","state":"Massachusetts","country":"United States","postcode":"02139"},"geometry":{"type":"Point","coordinates":[-71.10604,42.36634]}},
{"properties":{"type":"city","name":"Toronto","state":"Ontario","country":"Canada"},"geometry":{"type":"Point","coordinates":[-79.38,43.65]}},
{"properties":{"name":"Broken"},"geometry":{"type":"Point","coordinates":[]}}
]}`

func TestSearch(t *testing.T) {
	var got *http.Request
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r
		w.Write([]byte(sample))
	}))
	defer srv.Close()
	p := &Photon{URL: srv.URL, Client: srv.Client()}

	rs, err := p.Search(t.Context(), " 77 mass ave ", &Near{Lat: 42.36, Lng: -71.09}, 50)
	if err != nil {
		t.Fatal(err)
	}
	q := got.URL.Query()
	if got.URL.Path != "/api/" || q.Get("q") != "77 mass ave" || q.Get("limit") != "8" || q.Get("lat") != "42.36000" || q.Get("lon") != "-71.09000" || got.UserAgent() != userAgent {
		t.Errorf("request = %s %s", got.URL, got.UserAgent())
	}
	want := []Result{
		{Address: "77 Massachusetts Ave, Cambridge, Massachusetts 02139", Lat: 42.35941, Lng: -71.09375},
		{Name: "Cambridge Post Office", Address: "770 Massachusetts Avenue, Cambridge, Massachusetts 02139", Lat: 42.36634, Lng: -71.10604},
		{Address: "Toronto, Ontario, Canada", Lat: 43.65, Lng: -79.38},
	}
	if !reflect.DeepEqual(rs, want) {
		t.Errorf("results =\n%+v\nwant\n%+v", rs, want)
	}

	if rs, err := p.Search(t.Context(), "  ", nil, 5); err != nil || len(rs) != 0 {
		t.Errorf("blank query = %+v %v", rs, err)
	}
}

func TestSearchUpstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "slow down", http.StatusTooManyRequests)
	}))
	defer srv.Close()
	p := &Photon{URL: srv.URL, Client: srv.Client()}
	if _, err := p.Search(t.Context(), "x", nil, 5); err == nil {
		t.Error("want error")
	}
}
