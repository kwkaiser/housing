package craigslist

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const viewURL = "https://www.craigslist.org/view/d/medford-medford-tufts-2bed-in-unit/j2QRLoSwSfTzCLCkKJx9Q9"

func assumedLookup(t *testing.T) []json.RawMessage {
	t.Helper()
	b, err := os.ReadFile("testdata/lookup_assumed.json")
	if err != nil {
		t.Fatal(err)
	}
	var items []json.RawMessage
	if err := json.Unmarshal(b, &items); err != nil {
		t.Fatal(err)
	}
	return items
}

func TestListingURL(t *testing.T) {
	for in, want := range map[string]string{
		viewURL + "?lang=en#map": viewURL,
		"https://boston.craigslist.org/gbs/apa/d/somerville-sunny-2br/7881234567.html":  "https://boston.craigslist.org/gbs/apa/d/somerville-sunny-2br/7881234567.html",
		"http://Boston.Craigslist.org/apa/d/somerville-sunny-2br/7881234567.html?utm=1": "https://boston.craigslist.org/apa/d/somerville-sunny-2br/7881234567.html",
		"https://phoenix.craigslist.org/cph/apa/d/tempe-listing/7929480219.html":        "https://phoenix.craigslist.org/cph/apa/d/tempe-listing/7929480219.html",
	} {
		if got, err := ListingURL(in); err != nil || got != want {
			t.Errorf("ListingURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{
		"https://boston.craigslist.org/search/apa?query=somerville",
		"https://boston.craigslist.org/",
		"https://boston.craigslist.org/gbs/apa/d/somerville-sunny-2br/",
		"https://craigslist.example.com/apa/d/x/1.html",
	} {
		if _, err := ListingURL(in); err == nil {
			t.Errorf("ListingURL(%q) should fail", in)
		}
	}
}

func TestLookup(t *testing.T) {
	searchP, _ := newProvider(fixture(t), zip02144)
	found, err := searchP.Search(t.Context(), listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "02144"}})
	if err != nil {
		t.Fatal(err)
	}
	fromSearch := found[0]

	p, r := newProvider(assumedLookup(t), zip02144)
	got, err := p.Lookup(t.Context(), viewURL+"?lang=en")
	if err != nil {
		t.Fatal(err)
	}
	want := actorInput{StartURLs: []actorURL{{URL: viewURL}}, IncludeDetails: true, MaxItems: 1}
	if !reflect.DeepEqual(r.input, want) {
		t.Errorf("input = %+v", r.input)
	}
	if got.Source != listing.SourceCraigslist || got.SourceID != fromSearch.SourceID || got.URL != fromSearch.URL {
		t.Errorf("identity %s %s, search has %s %s", got.SourceID, got.URL, fromSearch.SourceID, fromSearch.URL)
	}
	if got.Offer != listing.OfferRent || got.Price.Cents != 350000 || *got.Beds != 2 || *got.Baths != 1 || got.Coordinates == nil ||
		got.Address != fromSearch.Address || got.ListedAt == nil || got.Description == "" || len(got.Raw) == 0 {
		t.Errorf("lookup = %+v", got)
	}
	if len(got.Photos) != 3 || !strings.HasSuffix(got.Photos[0], "_1200x900.jpg") {
		t.Errorf("photos = %v", got.Photos)
	}
	if !got.Amenities[listing.AmenityInUnitLaundry] || !got.Amenities[listing.AmenityParking] {
		t.Errorf("amenities = %v", got.Amenities)
	}
}

func TestLookupWithoutActorID(t *testing.T) {
	var item map[string]any
	if err := json.Unmarshal(assumedLookup(t)[0], &item); err != nil {
		t.Fatal(err)
	}
	item["id"] = ""
	raw, _ := json.Marshal(item)
	p, _ := newProvider([]json.RawMessage{raw}, zip02144)
	got, err := p.Lookup(t.Context(), viewURL)
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceID != "j2QRLoSwSfTzCLCkKJx9Q9" {
		t.Errorf("source id = %q, want the code from the post link", got.SourceID)
	}
	if postID("https://boston.craigslist.org/apa/d/sunny-2br/7881234567.html") != "7881234567" {
		t.Error("classic links should use the numeric post id")
	}
}

func TestLookupErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		url   string
		items []json.RawMessage
		want  string
	}{
		"search page":  {"https://boston.craigslist.org/search/apa", nil, "not a craigslist posting"},
		"empty output": {viewURL, nil, "returned no priced rental"},
		"no results":   {viewURL, []json.RawMessage{json.RawMessage(`{"label":"NO_RESULTS","causes":["gone"]}`)}, "returned no priced rental"},
		"unpriced":     {viewURL, []json.RawMessage{json.RawMessage(`{"id":"x","url":"` + viewURL + `","price":""}`)}, "returned no priced rental"},
		"not json":     {viewURL, []json.RawMessage{json.RawMessage(`"oops"`)}, "decode craigslist item"},
	} {
		p, _ := newProvider(tc.items, zip02144)
		if _, err := p.Lookup(t.Context(), tc.url); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}
