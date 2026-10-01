package dedupe

import (
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func ptr[T any](v T) *T { return &v }

func rent(source listing.Source, id string, dollars int64, beds int, a listing.Address, c *listing.Coordinates) listing.Listing {
	return listing.Listing{
		Source:      source,
		SourceID:    id,
		Offer:       listing.OfferRent,
		Price:       listing.Money{Cents: dollars * 100, Currency: "USD"},
		Beds:        ptr(beds),
		Address:     a,
		Coordinates: c,
	}
}

func keys(g Group) []string {
	var out []string
	for _, l := range g.Members() {
		out = append(out, Key(l))
	}
	return out
}

func TestParse(t *testing.T) {
	cases := map[string]struct {
		in   listing.Address
		want address
	}{
		"unit in street": {
			listing.Address{Street: "296 Highland Ave Unit 2", City: "Somerville", PostalCode: "02144"},
			address{number: "296", street: "296 highland ave", unit: "2", city: "somerville", postal: "02144"},
		},
		"hash unit and long suffix": {
			listing.Address{Street: "9 Kidder Avenue #2", PostalCode: "02144-1234"},
			address{number: "9", street: "9 kidder ave", unit: "2", postal: "02144"},
		},
		"unit field with prefix": {
			listing.Address{Street: "101 North Street", Unit: "Apt 03"},
			address{number: "101", street: "101 n st", unit: "3"},
		},
		"no house number": {
			listing.Address{Street: "Sheridan Ave", City: "Medford"},
			address{city: "medford"},
		},
	}
	for name, c := range cases {
		if got := parse(c.in); got != c.want {
			t.Errorf("%s: got %+v want %+v", name, got, c.want)
		}
	}
}

func TestGroups(t *testing.T) {
	kidder := listing.Address{Street: "9 Kidder Ave", Unit: "2", City: "Somerville", PostalCode: "02144"}
	davis := &listing.Coordinates{Lat: 42.3991, Lng: -71.1185}

	zillow := rent(listing.SourceZillow, "z1", 2800, 2, kidder, davis)
	zillow.Photos = []string{"a"}
	redfin := rent(listing.SourceRedfin, "r1", 2800, 2, listing.Address{Street: "9 Kidder Avenue #2", City: "Somerville", PostalCode: "02144"}, davis)
	redfin.Photos = []string{"a", "b"}
	facebook := rent(listing.SourceFacebook, "f1", 2850, 2, listing.Address{Street: "Kidder Ave", City: "Somerville"}, &listing.Coordinates{Lat: 42.3993, Lng: -71.1186})
	otherUnit := rent(listing.SourceCraigslist, "c1", 2800, 2, listing.Address{Street: "9 Kidder Ave", Unit: "3", PostalCode: "02144"}, davis)
	otherBeds := rent(listing.SourceCraigslist, "c2", 2800, 3, listing.Address{Street: "9 Kidder Ave"}, davis)
	sameSource := rent(listing.SourceZillow, "z2", 2800, 2, kidder, davis)

	groups := Groups([]listing.Listing{redfin, otherUnit, facebook, zillow, otherBeds, sameSource})
	if len(groups) != 4 {
		t.Fatalf("got %d groups: %+v", len(groups), groups)
	}
	if got := keys(groups[0]); len(got) != 3 || got[0] != "zillow/z1" || got[1] != "redfin/r1" || got[2] != "facebook/f1" {
		t.Errorf("zillow should lead the kidder group: %v", got)
	}
	for _, g := range groups[1:] {
		if len(g.Others) != 0 {
			t.Errorf("distinct listing grouped: %v", keys(g))
		}
	}
}

func TestGroupsPrefersAssessed(t *testing.T) {
	a := listing.Address{Street: "23 Everett St", PostalCode: "02138"}
	zillow := rent(listing.SourceZillow, "z", 2800, 2, a, nil)
	facebook := rent(listing.SourceFacebook, "f", 2800, 2, a, nil).WithAssessment("attic", listing.Assessment{Model: "m"})
	if g := Groups([]listing.Listing{zillow, facebook}); len(g) != 1 || g[0].Primary.Source != listing.SourceFacebook {
		t.Errorf("assessed listing should stay primary: %+v", g)
	}
}

func TestGroupsMissingUnit(t *testing.T) {
	street := "2 Beech St"
	unit1 := rent(listing.SourceZillow, "z", 3350, 2, listing.Address{Street: street, Unit: "101", PostalCode: "02140"}, nil)
	unit2 := rent(listing.SourceRedfin, "r", 3350, 2, listing.Address{Street: street, Unit: "201", PostalCode: "02140"}, nil)
	bare := rent(listing.SourceFacebook, "f", 3350, 2, listing.Address{Street: street, PostalCode: "02140"}, nil)
	groups := Groups([]listing.Listing{bare, unit1, unit2})
	if len(groups) != 2 {
		t.Fatalf("a listing without a unit must not bridge two units: %d groups", len(groups))
	}
	pricey := rent(listing.SourceFacebook, "f", 4000, 2, listing.Address{Street: street, PostalCode: "02140"}, nil)
	if len(Groups([]listing.Listing{pricey, unit1})) != 2 {
		t.Error("a missing unit should only match at a similar price")
	}
}

func TestGroupsLocality(t *testing.T) {
	a := rent(listing.SourceZillow, "z", 2000, 1, listing.Address{Street: "1 Main St", PostalCode: "02144"}, nil)
	b := rent(listing.SourceRedfin, "r", 2000, 1, listing.Address{Street: "1 Main Street", PostalCode: "78704"}, nil)
	if len(Groups([]listing.Listing{a, b})) != 2 {
		t.Error("same street in different postal codes must not match")
	}
	b.Offer = listing.OfferSale
	b.Address.PostalCode = "02144"
	if len(Groups([]listing.Listing{a, b})) != 2 {
		t.Error("rent and sale listings must not match")
	}
}
