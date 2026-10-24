//go:build integration

package streeteasy

import (
	"context"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

func liveProvider(t *testing.T) *Provider {
	t.Helper()
	token, err := config.Load().Apify()
	if err != nil {
		t.Skip(err)
	}
	client := apify.NewClient(token)
	client.MaxTotalChargeUSD = 0.05
	return New(apify.Limit(client, apify.DefaultConcurrency), zillow.NewAutocomplete())
}

func TestSearchLive(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	q := listing.Query{
		Offer:    listing.OfferRent,
		Area:     listing.Area{Location: "11211", RadiusMiles: 0.5},
		MaxPrice: &listing.Money{Cents: 500000, Currency: "USD"},
		MinBeds:  ptr(1),
		Limit:    3,
	}
	got, err := p.Search(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) == 0 || len(got) > q.Limit {
		t.Fatalf("got %d listings", len(got))
	}
	center, _ := geo.LookupZip("11211")
	for _, l := range got {
		if l.SourceID == "" || l.URL == "" || l.Offer != q.Offer || l.Price.Cents == 0 || l.Address.Formatted == "" ||
			len(l.Photos) == 0 || l.Coordinates == nil || l.ListedAt == nil || !q.Matches(l) {
			t.Errorf("incomplete listing: %+v", l)
			continue
		}
		t.Logf("%s %s $%d %.2fmi %s", l.SourceID, l.ListedAt.Format(time.DateOnly), l.Price.Cents/100, geo.DistanceMiles(center.Center, *l.Coordinates), l.Address.Formatted)
	}
}

func TestLookupLive(t *testing.T) {
	p := liveProvider(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()

	l, err := p.Lookup(ctx, "https://streeteasy.com/building/240-meeker-avenue-brooklyn/9")
	if err != nil {
		t.Fatal(err)
	}
	if l.SourceID != "5165844" || l.Offer != listing.OfferRent || l.Price.Cents == 0 || l.Coordinates == nil || len(l.Photos) == 0 {
		t.Errorf("listing = %+v", l)
	}
	t.Logf("%s %s $%d %+v", l.SourceID, l.Address.Formatted, l.Price.Cents/100, *l.Coordinates)
}
