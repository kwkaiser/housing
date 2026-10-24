package streeteasy

import (
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

func TestListingURL(t *testing.T) {
	for raw, want := range map[string]string{
		"https://streeteasy.com/building/240-meeker-avenue-brooklyn/9":                       "https://streeteasy.com/building/240-meeker-avenue-brooklyn/9",
		"https://www.streeteasy.com/building/one-fifty/rental/4642882?utm_campaign=x&lstt=y": "https://streeteasy.com/building/one-fifty/rental/4642882",
		"https://streeteasy.com/building/the-riverie/th107/?featured=1":                      "https://streeteasy.com/building/the-riverie/th107",
		"https://streeteasy.com/rental/5170579":                                              "https://streeteasy.com/rental/5170579",
		"https://streeteasy.com/sale/1845446?utm_source=web":                                 "https://streeteasy.com/sale/1845446",
	} {
		got, err := ListingURL(raw)
		if err != nil || got != want {
			t.Errorf("%s: got %q err %v, want %q", raw, got, err, want)
		}
	}
	for raw, want := range map[string]string{
		"https://streeteasy.com/for-rent/brooklyn":   "not a streeteasy listing page",
		"https://streeteasy.com/building/one-fifty":  "not a streeteasy listing page",
		"https://streeteasy.com/building/a/b/c":      "not a streeteasy listing page",
		"https://notstreeteasy.com/rental/5170579":   "not a streeteasy url",
		"https://www.zillow.com/homedetails/1_zpid/": "not a streeteasy url",
	} {
		if _, err := ListingURL(raw); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", raw, err, want)
		}
	}
}

func TestLookupRental(t *testing.T) {
	geocoded := zillow.Region{Name: "240 Meeker Ave #9 Brooklyn, NY 11211", Center: listing.Coordinates{Lat: 40.715878, Lng: -73.950447}}
	p, r, regions := newProvider(fixture(t, "lookup_rent.json")[:1], geocoded)
	l, err := p.Lookup(t.Context(), "https://www.streeteasy.com/building/240-meeker-avenue-brooklyn/9?utm_source=share")
	if err != nil {
		t.Fatal(err)
	}
	in := r.inputs[0].Input.(actorInput)
	if len(in.StartURLs) != 1 || in.StartURLs[0].URL != "https://streeteasy.com/building/240-meeker-avenue-brooklyn/9" || in.MaxItems != 1 {
		t.Errorf("input = %+v", in)
	}
	if l.Offer != listing.OfferRent || l.SourceID != "5165844" || l.Price.Cents != 399900 || *l.Beds != 1 || len(l.Photos) != 9 {
		t.Errorf("listing = %+v", l)
	}
	if l.Address.Formatted != "240 Meeker Avenue #9, Brooklyn, NY 11211" || regions.query != l.Address.Formatted {
		t.Errorf("address %q geocoded with %q", l.Address.Formatted, regions.query)
	}
	if l.Coordinates == nil || *l.Coordinates != geocoded.Center {
		t.Errorf("coordinates = %+v", l.Coordinates)
	}
}

func TestLookupSale(t *testing.T) {
	p, _, regions := newProvider(fixture(t, "lookup_sale.json"), zillow.Region{})
	regions.err = zillow.ErrRegionNotFound
	l, err := p.Lookup(t.Context(), "https://streeteasy.com/building/420-6-avenue-brooklyn/12")
	if err != nil {
		t.Fatal(err)
	}
	if l.Offer != listing.OfferSale || l.Price.Cents != 97500000 || l.Coordinates != nil {
		t.Errorf("listing = %+v", l)
	}
}

func TestLookupErrors(t *testing.T) {
	p, r, _ := newProvider(nil, zillow.Region{})
	if _, err := p.Lookup(t.Context(), "https://streeteasy.com/for-rent/brooklyn"); err == nil || len(r.inputs) != 0 {
		t.Errorf("search page: err = %v, runs = %d", err, len(r.inputs))
	}
	if _, err := p.Lookup(t.Context(), "https://streeteasy.com/rental/1"); err == nil || !strings.Contains(err.Error(), "no priced listing") {
		t.Errorf("empty result: err = %v", err)
	}
}
