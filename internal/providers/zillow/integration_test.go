//go:build integration

package zillow

import (
	"context"
	"os"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
)

func TestSearchLive(t *testing.T) {
	token, err := config.Load().Apify()
	if err != nil {
		t.Skip(err)
	}
	client := apify.NewClient(token)
	client.MaxTotalChargeUSD = 0.05
	p := New(client, NewAutocomplete())

	minBeds := 1
	type tc struct {
		query  listing.Query
		postal string
	}
	queries := map[string]tc{
		"rent": {query: listing.Query{
			Offer:     listing.OfferRent,
			Area:      listing.Area{Location: "Austin, TX", RadiusMiles: 5},
			MaxPrice:  &listing.Money{Cents: 300000, Currency: "USD"},
			MinBeds:   &minBeds,
			MaxAge:    7 * day,
			Amenities: []listing.Amenity{listing.AmenityInUnitLaundry},
			Limit:     3,
		}},
		"sale": {postal: "78704", query: listing.Query{
			Offer:    listing.OfferSale,
			Area:     listing.Area{Location: "78704"},
			MaxPrice: &listing.Money{Cents: 80000000, Currency: "USD"},
			MaxAge:   14 * day,
			Limit:    3,
		}},
	}

	for name, c := range queries {
		q := c.query
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
			defer cancel()

			got, err := p.Search(ctx, q)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) == 0 || len(got) > q.Limit {
				t.Fatalf("got %d listings", len(got))
			}
			for _, l := range got {
				if l.SourceID == "" || l.URL == "" || l.Offer != q.Offer || l.Price.Cents == 0 ||
					l.Address.Formatted == "" || len(l.Photos) == 0 || l.ObservedAt.IsZero() {
					t.Errorf("incomplete listing: %+v", l)
				}
				if c.postal != "" && l.Address.PostalCode != c.postal {
					t.Errorf("listing outside %s: %s", c.postal, l.Address.Formatted)
				}
			}
			persist(t, got)
		})
	}
}

func TestEnrichLive(t *testing.T) {
	token, err := config.Load().Apify()
	if err != nil {
		t.Skip(err)
	}
	client := apify.NewClient(token)
	client.MaxTotalChargeUSD = 0.05
	p := New(client, NewAutocomplete())

	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()

	q := listing.Query{
		Offer:    listing.OfferRent,
		Area:     listing.Area{Location: "78704"},
		MaxPrice: &listing.Money{Cents: 250000, Currency: "USD"},
		Limit:    2,
	}
	found, err := p.Search(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	enriched, err := p.Enrich(ctx, found)
	if err != nil {
		t.Fatal(err)
	}

	var got []listing.Listing
	for _, l := range enriched {
		if q.Matches(l) {
			got = append(got, l)
		}
	}
	if len(got) == 0 {
		t.Fatalf("no listings after enrichment (searched %d, enriched %d)", len(found), len(enriched))
	}
	for _, l := range got {
		if l.Beds == nil || l.Baths == nil || l.SqFt == nil {
			t.Errorf("enriched listing missing beds/baths/sqft: %s %s", l.SourceID, l.URL)
		}
	}
	t.Logf("searched %d, enriched %d, matching %d", len(found), len(enriched), len(got))
	persist(t, got)
}

func persist(t *testing.T, ls []listing.Listing) {
	t.Helper()
	dir := os.Getenv("HOUSING_DATA_DIR")
	if dir == "" {
		return
	}
	if err := (jsonfile.Persister{}).Persist(t.Context(), dir, ls); err != nil {
		t.Fatal(err)
	}
}
