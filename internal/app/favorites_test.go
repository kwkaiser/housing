package app

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
)

func TestFavorites(t *testing.T) {
	a := seededApp(t)
	h := a.Handler()

	res, body := get(t, h, "/favorites")
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "No favorites yet") || !strings.Contains(body, `<a href="/favorites" aria-current="page">Favorites</a>`) {
		t.Fatalf("empty favorites = %d\n%s", res.StatusCode, body)
	}
	_, body = get(t, h, "/listing/zillow/z1?collection=somerville")
	if !strings.Contains(body, `action="/listing/zillow/z1/favorite"`) || !strings.Contains(body, `name="on" value="1"`) || !strings.Contains(body, `value="/listing/zillow/z1?collection=somerville"`) {
		t.Errorf("listing page missing an unstarred star form")
	}

	if res, _ := post(t, h, "/listing/zillow/z1/favorite", url.Values{"on": {"1"}}, map[string]string{"Sec-Fetch-Site": "cross-site"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site favorite = %d", res.StatusCode)
	}
	if res, _ := post(t, h, "/listing/zillow/nope/favorite", url.Values{"on": {"1"}}, sameOriginHeader); res.StatusCode != http.StatusNotFound {
		t.Errorf("favorite missing listing = %d", res.StatusCode)
	}
	res, _ = post(t, h, "/listing/zillow/z1/favorite", url.Values{"on": {"1"}, "next": {"/c/somerville?sort=price&dir=asc"}}, sameOriginHeader)
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/c/somerville?sort=price&dir=asc" {
		t.Errorf("favorite = %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	for _, next := range []string{"https://evil.example/", "//evil.example/", "/\\evil.example", "c/somerville", ""} {
		res, _ := post(t, h, "/listing/zillow/z2/favorite", url.Values{"on": {"1"}, "next": {next}}, sameOriginHeader)
		if res.Header.Get("Location") != "/listing/zillow/z2" {
			t.Errorf("next %q redirected to %q", next, res.Header.Get("Location"))
		}
	}

	_, body = get(t, h, "/favorites")
	for _, s := range []string{"7 Windom St", "31 Fairmount Ave", `<a href="/listing/zillow/z1">`, `<td class="num score">88%</td>`, `name="on" value="0"`, `value="/favorites"`, "2026-09-30"} {
		if !strings.Contains(body, s) {
			t.Errorf("favorites missing %q", s)
		}
	}
	if strings.Contains(body, "5 New St") {
		t.Errorf("favorites should only list starred listings")
	}

	_, body = get(t, h, "/c/somerville")
	if strings.Count(body, `aria-pressed="true"`) != 2 || !strings.Contains(body, `value="/c/somerville"`) {
		t.Errorf("collection page should mark the two starred rows")
	}
	_, body = get(t, h, "/listing/zillow/z1")
	if !strings.Contains(body, `aria-pressed="true"`) || !strings.Contains(body, `name="on" value="0"`) {
		t.Errorf("listing page should show the listing as starred")
	}

	if res, _ := post(t, h, "/listing/zillow/z1/favorite", url.Values{"on": {"0"}, "next": {"/favorites"}}, sameOriginHeader); res.Header.Get("Location") != "/favorites" {
		t.Errorf("unfavorite redirect = %q", res.Header.Get("Location"))
	}
	_, body = get(t, h, "/favorites")
	if strings.Contains(body, "7 Windom St") || !strings.Contains(body, "31 Fairmount Ave") {
		t.Errorf("unfavorited listing should leave the list")
	}
}

func TestListingMap(t *testing.T) {
	a := seededApp(t)
	h := a.Handler()

	_, body := get(t, h, "/listing/zillow/z1")
	for _, s := range []string{
		`<details class="section listing-map-section">`, `<div id="listing-map" class="listing-map"></div>`,
		"approximate, placed at the zip code's center", `id="map-config">{"positron":false,"overlays":[{"name":"bike"`,
		`id="listing-pin">{"lat":42.4001,"lng":-71.1231,"approx":true,"title":"7 Windom St, Somerville, MA 02144"}`, `/static/leaflet/leaflet.css?v=`, `/static/map-layers.js?v=`, `/static/listing-map.js?v=`,
	} {
		if !strings.Contains(body, s) {
			t.Errorf("listing map missing %q", s)
		}
	}
	if strings.Contains(body, `<details class="section listing-map-section" open`) {
		t.Errorf("map should be collapsed by default")
	}
}
