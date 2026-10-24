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

func TestLocate(t *testing.T) {
	somerville := listing.Coordinates{Lat: 42.4001, Lng: -71.1231}
	exact := listing.Coordinates{Lat: 42.39, Lng: -71.1}
	for _, tc := range []struct {
		name   string
		l      listing.Listing
		want   listing.Coordinates
		approx bool
		ok     bool
	}{
		{"coordinates", listing.Listing{Coordinates: &exact, Address: listing.Address{PostalCode: "11211"}}, exact, false, true},
		{"zero coordinates", listing.Listing{Coordinates: &listing.Coordinates{}, Address: listing.Address{PostalCode: "02144"}}, somerville, true, true},
		{"postal code", listing.Listing{Address: listing.Address{PostalCode: "02144"}}, somerville, true, true},
		{"zip+4", listing.Listing{Address: listing.Address{PostalCode: "02144-1234"}}, somerville, true, true},
		{"formatted", listing.Listing{Address: listing.Address{Formatted: "7 Windom St, Somerville, MA 02144"}}, somerville, true, true},
		{"formatted zip+4", listing.Listing{Address: listing.Address{Formatted: "7 Windom St, Somerville, MA 02144-1234"}}, somerville, true, true},
		{"unknown zip", listing.Listing{Address: listing.Address{PostalCode: "00000"}}, listing.Coordinates{}, false, false},
		{"street number only", listing.Listing{Address: listing.Address{Formatted: "12345 Main St"}}, listing.Coordinates{}, false, false},
		{"nothing", listing.Listing{}, listing.Coordinates{}, false, false},
	} {
		got, approx, ok := Locate(tc.l)
		if got != tc.want || approx != tc.approx || ok != tc.ok {
			t.Errorf("%s: Locate = %+v, %v, %v; want %+v, %v, %v", tc.name, got, approx, ok, tc.want, tc.approx, tc.ok)
		}
	}
}
