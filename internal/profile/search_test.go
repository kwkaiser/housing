package profile

import (
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func ptrInt(v int) *int { return &v }

func TestSearchQuery(t *testing.T) {
	s := Search{
		Location:   "Somerville, MA",
		MinPrice:   ptrInt(500000),
		MaxPrice:   ptrInt(800000),
		MinBeds:    ptrInt(2),
		MaxAgeDays: 14,
		Amenities:  []listing.Amenity{listing.AmenityDishwasher},
		Limit:      5,
	}
	q := s.Query(ModeBuy)
	if q.Offer != listing.OfferSale || q.Area.Location != "Somerville, MA" || q.MaxAge != 14*24*time.Hour || q.Limit != 5 {
		t.Errorf("query = %+v", q)
	}
	if q.MinPrice.Cents != 50000000 || q.MaxPrice.Cents != 80000000 || *q.MinBeds != 2 {
		t.Errorf("prices/beds = %+v %+v %v", q.MinPrice, q.MaxPrice, *q.MinBeds)
	}
	if ModeRent.Offer() != listing.OfferRent {
		t.Error("rent mode should map to rent offers")
	}
}

func TestSearchValidate(t *testing.T) {
	bad := []Search{
		{Amenities: []listing.Amenity{"pets_allowed"}},
		{MinPrice: ptrInt(10), MaxPrice: ptrInt(5)},
		{MinBeds: ptrInt(3), MaxBeds: ptrInt(1)},
	}
	for _, s := range bad {
		if s.Validate() == nil {
			t.Errorf("%+v should be invalid", s)
		}
	}
	p := Profile{ID: "x", Searches: map[Mode]Search{"lease": {}}}
	if p.Validate() == nil {
		t.Error("unknown mode should be invalid")
	}
}

func TestSearchesDoNotChangeProfileHash(t *testing.T) {
	p := testProfile
	before := p.Hash()
	p.Searches = map[Mode]Search{ModeRent: {Location: "Somerville, MA"}}
	if p.Hash() != before {
		t.Error("editing saved searches should not invalidate assessments")
	}
}
