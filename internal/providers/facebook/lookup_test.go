package facebook

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const itemLink = "https://www.facebook.com/marketplace/item/1071136835630251/"

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
	for _, in := range []string{
		"https://www.facebook.com/marketplace/item/1071136835630251/?ref=search&referral_code=null&tracking=browse_serp%3Aabc",
		"https://facebook.com/marketplace/item/1071136835630251",
		"http://m.facebook.com/marketplace/item/1071136835630251/#photos",
	} {
		if got, err := ListingURL(in); err != nil || got != itemLink {
			t.Errorf("ListingURL(%q) = %q, %v", in, got, err)
		}
	}
	for _, in := range []string{
		"https://www.facebook.com/marketplace/boston/propertyrentals",
		"https://www.facebook.com/marketplace/item/",
		"https://www.facebook.com/marketplace/item/abc/",
		"https://www.facebook.com/groups/123",
		"https://facebook.example.com/marketplace/item/1/",
	} {
		if _, err := ListingURL(in); err == nil {
			t.Errorf("ListingURL(%q) should fail", in)
		}
	}
}

func TestLookup(t *testing.T) {
	searchP, _ := newProvider(fixture(t), somerville)
	found, err := searchP.Search(context.Background(), listing.Query{Offer: listing.OfferRent, Area: listing.Area{Location: "Somerville, MA", RadiusMiles: 50}})
	if err != nil {
		t.Fatal(err)
	}
	var fromSearch listing.Listing
	for _, l := range found {
		if l.SourceID == "1071136835630251" {
			fromSearch = l
		}
	}
	if fromSearch.SourceID == "" {
		t.Fatal("fixture listing missing from search")
	}

	p, r := newProvider(assumedLookup(t), somerville)
	got, err := p.Lookup(context.Background(), itemLink+"?ref=search")
	if err != nil {
		t.Fatal(err)
	}
	in, ok := r.input.(actorInput)
	if r.actor != ActorID || !ok || len(in.URLs) != 1 || in.URLs[0] != itemLink || !in.GetListingDetails || !in.GetAllListingPhotos || in.Proxy.ApifyProxyCountry != "US" {
		t.Errorf("run %s %+v", r.actor, r.input)
	}
	if got.Source != listing.SourceFacebook || got.SourceID != fromSearch.SourceID || got.URL != fromSearch.URL {
		t.Errorf("identity %s %s, search has %s %s", got.SourceID, got.URL, fromSearch.SourceID, fromSearch.URL)
	}
	if got.Offer != listing.OfferRent || got.Price.Cents != 235000 || *got.Beds != 2 || *got.Baths != 1 || got.Coordinates == nil ||
		got.Address != fromSearch.Address || got.ListedAt == nil || got.Description == "" || len(got.Photos) != 4 || len(got.Raw) == 0 {
		t.Errorf("lookup = %+v", got)
	}
}

func TestLookupKeepsListingsWithoutRooms(t *testing.T) {
	raw := json.RawMessage(`{"id":"42","marketplace_listing_title":"Sunny sublet","listing_price":{"amount":"1800.00","currency":"USD"}}`)
	p, _ := newProvider([]json.RawMessage{raw}, somerville)
	got, err := p.Lookup(context.Background(), "https://www.facebook.com/marketplace/item/42/")
	if err != nil {
		t.Fatal(err)
	}
	if got.SourceID != "42" || got.URL != "https://www.facebook.com/marketplace/item/42" || got.Beds != nil || got.Price.Cents != 180000 {
		t.Errorf("lookup = %+v", got)
	}
}

func TestLookupErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		url   string
		items []json.RawMessage
		want  string
	}{
		"search page":  {"https://www.facebook.com/marketplace/boston/search?query=apartment", nil, "not a facebook marketplace item"},
		"empty output": {itemLink, nil, "returned nothing"},
		"other item":   {itemLink, []json.RawMessage{json.RawMessage(`{"id":"9","listing_price":{"amount":"1"}}`)}, "returned nothing"},
		"unpriced":     {itemLink, []json.RawMessage{json.RawMessage(`{"id":"1071136835630251","listing_price":{"amount":"0"}}`)}, "has no price"},
		"not json":     {itemLink, []json.RawMessage{json.RawMessage(`"oops"`)}, "decode facebook item"},
	} {
		p, _ := newProvider(tc.items, somerville)
		if _, err := p.Lookup(context.Background(), tc.url); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want %q", name, err, tc.want)
		}
	}
}
