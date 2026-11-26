package listing

import (
	"context"
	"slices"
	"testing"
)

type capProvider struct{ caps Capabilities }

func (p capProvider) Source() Source                                   { return SourceZillow }
func (p capProvider) Capabilities(OfferType) Capabilities              { return p.caps }
func (p capProvider) Search(context.Context, Query) ([]Listing, error) { return nil, nil }

type capEnricher struct {
	capProvider
	enriches []Amenity
}

func (e capEnricher) Enriches(OfferType) []Amenity                         { return e.enriches }
func (e capEnricher) Enrich(context.Context, []Listing) ([]Listing, error) { return nil, nil }

func TestCapabilitiesOf(t *testing.T) {
	p := capEnricher{
		capProvider: capProvider{NewCapabilities([]Amenity{AmenityParking}, []Amenity{AmenityAirConditioning, AmenityParking})},
		enriches:    []Amenity{AmenityParking, AmenityDishwasher, AmenityFurnished},
	}
	got := CapabilitiesOf(p, OfferRent)
	want := Capabilities{AmenityParking: SupportSearch, AmenityAirConditioning: SupportResults, AmenityDishwasher: SupportEnrich, AmenityFurnished: SupportEnrich}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for a, s := range want {
		if got[a] != s {
			t.Errorf("%s: got %q, want %q", a, got[a], s)
		}
	}
	if s := CapabilitiesOf(p, OfferSale)[AmenityFurnished]; s != "" {
		t.Errorf("sale furnished support = %q, want none", s)
	}
	if got := got.Searchable(); !slices.Equal(got, []Amenity{AmenityParking}) {
		t.Errorf("searchable = %v", got)
	}
	if got := CapabilitiesOf(capProvider{}, OfferRent); got == nil || len(got) != 0 {
		t.Errorf("nil capabilities = %v", got)
	}
}

func TestOfferAmenities(t *testing.T) {
	if slices.ContainsFunc(OfferAmenities(OfferSale), Amenity.RentOnly) {
		t.Error("sale amenities include rental terms")
	}
	if !slices.Equal(OfferAmenities(OfferRent), Amenities) {
		t.Error("rent amenities differ from all amenities")
	}
}
