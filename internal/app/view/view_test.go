package view

import (
	"testing"
	"time"

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

func TestSmall(t *testing.T) {
	half, six := 1.5, 1200
	for _, tc := range []struct{ got, want string }{
		{Fixed1(61.04), "61.0"},
		{Percent(66.6), "67%"},
		{ShortModel("google/gemini-3-flash"), "gemini-3-flash"},
		{ShortModel("plain"), "plain"},
		{Ints([]int{1, 3}), "1, 3"},
		{Ints(nil), ""},
		{Baths(&half), "1.5"},
		{Baths(nil), "-"},
		{SqFt(&six), "1,200"},
		{SqFt(nil), "-"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}

func TestAgo(t *testing.T) {
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		at   time.Time
		want string
	}{
		{time.Time{}, "-"},
		{now.Add(time.Minute), "just now"},
		{now.Add(-30 * time.Second), "just now"},
		{now.Add(-5*time.Minute - 30*time.Second), "5m ago"},
		{now.Add(-2*time.Hour - 59*time.Minute), "2h ago"},
		{now.Add(-47 * time.Hour), "47h ago"},
		{now.Add(-72 * time.Hour), "3d ago"},
	} {
		if got := Ago(tc.at, now); got != tc.want {
			t.Errorf("Ago(%v) = %q, want %q", now.Sub(tc.at), got, tc.want)
		}
	}
}

func TestElapsed(t *testing.T) {
	for d, want := range map[time.Duration]string{
		-time.Second:                               "0s",
		400 * time.Millisecond:                     "0s",
		12*time.Second + 600*time.Millisecond:      "13s",
		3*time.Minute + 5*time.Second:              "3m 05s",
		time.Hour + 2*time.Minute + 40*time.Second: "1h 02m",
	} {
		if got := Elapsed(d); got != want {
			t.Errorf("Elapsed(%v) = %q, want %q", d, got, want)
		}
	}
}

func TestJobLabels(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{Cost(0), "-"},
		{Cost(0.0423), "$0.042"},
		{Cost(1.234), "$1.23"},
		{Truncate("short", 10), "short"},
		{Truncate("a  long\nmessage here", 6), "a long…"},
		{Truncate("héllo wörld", 5), "héllo…"},
		{StatusClass("failed"), "status failed"},
		{StatusClass("<x>"), "status"},
		{KindLabel("run_collection"), "Collection run"},
		{KindLabel("draft_profile"), "Draft profile"},
		{KindLabel("new_thing"), "new thing"},
	} {
		if tc.got != tc.want {
			t.Errorf("got %q, want %q", tc.got, tc.want)
		}
	}
}
