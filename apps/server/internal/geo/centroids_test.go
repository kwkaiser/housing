package geo

import (
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func TestZipCentroid(t *testing.T) {
	for code, want := range map[string]listing.Coordinates{
		"02144": {Lat: 42.4001, Lng: -71.1231},
		"11211": {Lat: 40.7141, Lng: -73.9452},
	} {
		if got, ok := ZipCentroid(code); !ok || got != want {
			t.Errorf("ZipCentroid(%s) = %+v, %v; want %+v", code, got, ok, want)
		}
	}
	if _, ok := ZipCentroid("00000"); ok {
		t.Error("ZipCentroid(00000) should miss")
	}
}

func TestPostalCode(t *testing.T) {
	for _, tc := range []struct {
		a    listing.Address
		want string
	}{
		{listing.Address{PostalCode: "02144"}, "02144"},
		{listing.Address{PostalCode: "02144-1234"}, "02144"},
		{listing.Address{Formatted: "7 Windom St, Somerville, MA 02144"}, "02144"},
		{listing.Address{Formatted: "7 Windom St, Somerville, MA 02144-1234"}, "02144"},
		{listing.Address{Formatted: "12345 Main St"}, ""},
		{listing.Address{}, ""},
	} {
		if got := PostalCode(tc.a); got != tc.want {
			t.Errorf("PostalCode(%+v) = %q, want %q", tc.a, got, tc.want)
		}
	}
}

func TestPlaced(t *testing.T) {
	for _, tc := range []struct {
		l    listing.Listing
		want bool
	}{
		{listing.Listing{Coordinates: &listing.Coordinates{Lat: 42.39, Lng: -71.1}}, true},
		{listing.Listing{Coordinates: &listing.Coordinates{}, Address: listing.Address{PostalCode: "02144"}}, false},
		{listing.Listing{Address: listing.Address{PostalCode: "02144"}}, false},
	} {
		if got := Placed(tc.l); got != tc.want {
			t.Errorf("Placed(%+v) = %v, want %v", tc.l, got, tc.want)
		}
	}
}
