package service

import (
	"context"
	"errors"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/geocode"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

type fakeGeocoder map[string][]geocode.Result

func (f fakeGeocoder) Search(_ context.Context, query string, _ *geocode.Near, _ int) ([]geocode.Result, error) {
	if query == "5 Boom St, Somerville, MA 02144" {
		return nil, errors.New("boom")
	}
	return f[query], nil
}

func TestGeocodeQuery(t *testing.T) {
	for _, tc := range []struct {
		a             listing.Address
		query, number string
	}{
		{listing.Address{Street: "7 Windom St", Unit: "2", City: "Somerville", State: "MA", PostalCode: "02144", Formatted: "7 Windom St #2, Somerville, MA 02144"}, "7 Windom St, Somerville, MA 02144", "7"},
		{listing.Address{Street: "12A Elm St", PostalCode: "02144"}, "12A Elm St, 02144", "12A"},
		{listing.Address{Formatted: "31 Fairmount Ave, Somerville, MA 02144"}, "31 Fairmount Ave, Somerville, MA 02144", "31"},
		{listing.Address{Formatted: "Somerville, MA 02144", PostalCode: "02144"}, "", ""},
		{listing.Address{Street: "Elm St", City: "Somerville"}, "", ""},
		{listing.Address{}, "", ""},
	} {
		q, n := geocodeQuery(tc.a)
		if q != tc.query || n != tc.number {
			t.Errorf("geocodeQuery(%+v) = %q, %q; want %q, %q", tc.a, q, n, tc.query, tc.number)
		}
	}
}

func TestLocate(t *testing.T) {
	g := fakeGeocoder{
		"7 Windom St, Somerville, MA 02144": {
			{Address: "Windom Street, Somerville, Massachusetts 02144", Lat: 42.39, Lng: -71.11},
			{Address: "7 Windom Street, Somerville, Massachusetts 02144", Lat: 42.3911, Lng: -71.1105},
		},
		"9 Far Rd, Somerville, MA 02144": {{Address: "9 Far Road, Springfield, Massachusetts 01103", Lat: 42.1, Lng: -72.59}},
	}
	exact := listing.Coordinates{Lat: 42.4, Lng: -71.12}
	addr := func(street string) listing.Address {
		return listing.Address{Street: street, City: "Somerville", State: "MA", PostalCode: "02144"}
	}
	ls := []listing.Listing{
		{SourceID: "a", Address: addr("7 Windom St")},
		{SourceID: "b", Address: addr("9 Far Rd")},
		{SourceID: "c", Address: addr("1 Missing Way")},
		{SourceID: "d", Address: addr("7 Windom St"), Coordinates: &exact},
		{SourceID: "e", Address: addr("5 Boom St")},
	}
	s := &Service{cfg: Config{Clients: Clients{Geocoder: g}}}
	s.locate(t.Context(), ls, s.logger(nil))
	if c := ls[0].Coordinates; c == nil || *c != (listing.Coordinates{Lat: 42.3911, Lng: -71.1105}) {
		t.Errorf("house match = %+v", c)
	}
	if ls[1].Coordinates != nil {
		t.Errorf("a match far from the zip code should be rejected: %+v", ls[1].Coordinates)
	}
	if ls[2].Coordinates != nil || ls[4].Coordinates != nil {
		t.Errorf("unmatched listings should stay unplaced: %+v %+v", ls[2].Coordinates, ls[4].Coordinates)
	}
	if *ls[3].Coordinates != exact {
		t.Errorf("existing coordinates changed: %+v", ls[3].Coordinates)
	}
}

func TestCarryOverCoordinates(t *testing.T) {
	c := listing.Coordinates{Lat: 42.39, Lng: -71.11}
	a := listing.Address{Street: "7 Windom St", PostalCode: "02144"}
	stored := []listing.Listing{{Source: listing.SourceZillow, SourceID: "z1", Address: a, Coordinates: &c}}
	moved := listing.Address{Street: "8 Windom St", PostalCode: "02144"}
	out := CarryOver(stored, []listing.Listing{
		{Source: listing.SourceZillow, SourceID: "z1", Address: a},
	})
	if out[0].Coordinates == nil || *out[0].Coordinates != c {
		t.Errorf("same address should keep its coordinates: %+v", out[0].Coordinates)
	}
	if out := CarryOver(stored, []listing.Listing{{Source: listing.SourceZillow, SourceID: "z1", Address: moved}}); out[0].Coordinates != nil {
		t.Errorf("a changed address should be geocoded again: %+v", out[0].Coordinates)
	}
}
