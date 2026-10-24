package view

import (
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func TestMoney(t *testing.T) {
	for _, tc := range []struct {
		cents int64
		offer listing.OfferType
		want  string
	}{
		{360000, listing.OfferRent, "$3,600/mo"},
		{125000000, listing.OfferSale, "$1,250,000"},
		{99950, listing.OfferSale, "$1,000"},
		{-12500, "", "-$125"},
		{0, listing.OfferRent, "-"},
	} {
		if got := Money(tc.cents, tc.offer); got != tc.want {
			t.Errorf("Money(%d, %q) = %q, want %q", tc.cents, tc.offer, got, tc.want)
		}
	}
}

func TestBeds(t *testing.T) {
	zero, two := 0, 2
	for _, tc := range []struct {
		beds *int
		want string
	}{
		{nil, "-"},
		{&zero, "studio"},
		{&two, "2"},
	} {
		if got := Beds(tc.beds); got != tc.want {
			t.Errorf("Beds = %q, want %q", got, tc.want)
		}
	}
}

func TestMatch(t *testing.T) {
	for _, tc := range []struct {
		match             float64
		calibrated, stale bool
		want              string
	}{
		{73.4, true, false, "73%"},
		{73.4, false, false, "73.4"},
		{73.4, true, true, "73%*"},
		{73.4, false, true, "73.4*"},
	} {
		if got := Match(tc.match, tc.calibrated, tc.stale); got != tc.want {
			t.Errorf("Match(%v, %v, %v) = %q, want %q", tc.match, tc.calibrated, tc.stale, got, tc.want)
		}
	}
}

func TestThousands(t *testing.T) {
	for n, want := range map[int64]string{0: "0", 999: "999", 1000: "1,000", 1234567: "1,234,567", -4500: "-4,500"} {
		if got := Thousands(n); got != want {
			t.Errorf("Thousands(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestBaths(t *testing.T) {
	half := 1.5
	if got := Baths(&half); got != "1.5" {
		t.Errorf("Baths(1.5) = %q", got)
	}
	if got := Baths(nil); got != "-" {
		t.Errorf("Baths(nil) = %q", got)
	}
}
