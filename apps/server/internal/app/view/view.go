package view

import (
	"fmt"
	"strconv"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func Money(cents int64, offer listing.OfferType) string {
	if cents == 0 {
		return "-"
	}
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	s := sign + "$" + Thousands((cents+50)/100)
	if offer == listing.OfferRent {
		s += "/mo"
	}
	return s
}

func Thousands(n int64) string {
	if n < 0 {
		return "-" + Thousands(-n)
	}
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func Beds(beds *int) string {
	switch {
	case beds == nil:
		return "-"
	case *beds == 0:
		return "studio"
	}
	return strconv.Itoa(*beds)
}

func Match(match float64, calibrated, stale bool) string {
	s := fmt.Sprintf("%.0f%%", match)
	if !calibrated {
		s = fmt.Sprintf("%.1f", match)
	}
	if stale {
		s += "*"
	}
	return s
}

func Baths(baths *float64) string {
	if baths == nil {
		return "-"
	}
	return strconv.FormatFloat(*baths, 'f', -1, 64)
}
