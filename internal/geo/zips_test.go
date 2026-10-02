package geo

import (
	"slices"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func codes(zips []Zip) []string {
	out := make([]string, len(zips))
	for i, z := range zips {
		out[i] = z.Code
	}
	return out
}

func TestZipsWithin(t *testing.T) {
	williamsburg := listing.Coordinates{Lat: 40.7145, Lng: -73.9425}
	got := codes(ZipsWithin(williamsburg, 1.5))
	if len(got) == 0 || got[0] != "11211" {
		t.Fatalf("ZipsWithin = %v, want 11211 first", got)
	}
	for _, want := range []string{"11206", "11222", "11249"} {
		if !slices.Contains(got, want) {
			t.Errorf("ZipsWithin = %v, missing %s", got, want)
		}
	}
	for _, far := range []string{"10001", "11215", "07302"} {
		if slices.Contains(got, far) {
			t.Errorf("ZipsWithin = %v, should not include %s", got, far)
		}
	}
	if got := ZipsWithin(listing.Coordinates{Lat: 42.3876, Lng: -71.0995}, 5); len(got) != 0 {
		t.Errorf("ZipsWithin(somerville) = %v, want none", codes(got))
	}
}

func TestLookupZip(t *testing.T) {
	z, ok := LookupZip("11211")
	if !ok || DistanceMiles(z.Center, listing.Coordinates{Lat: 40.714, Lng: -73.945}) > 0.5 || z.RadiusMiles <= 0 {
		t.Errorf("LookupZip(11211) = %+v, %v", z, ok)
	}
	if _, ok := LookupZip("02144"); ok {
		t.Error("LookupZip(02144) should miss outside the NYC area")
	}
}

func TestParseZips(t *testing.T) {
	got, err := parseZips("11211\t40.71408\t-73.94517\t0.68\n07302\t40.71881\t-74.04463\t0.68\n")
	if err != nil {
		t.Fatal(err)
	}
	want := []Zip{
		{Code: "11211", Center: listing.Coordinates{Lat: 40.71408, Lng: -73.94517}, RadiusMiles: 0.68},
		{Code: "07302", Center: listing.Coordinates{Lat: 40.71881, Lng: -74.04463}, RadiusMiles: 0.68},
	}
	if !slices.Equal(got, want) {
		t.Errorf("parseZips = %+v", got)
	}
	for _, bad := range []string{"11211\t40.7\t-73.9", "11211\tx\t-73.9\t0.5"} {
		if _, err := parseZips(bad); err == nil {
			t.Errorf("parseZips(%q) should fail", bad)
		}
	}
}
