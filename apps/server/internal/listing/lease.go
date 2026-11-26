package listing

import (
	"regexp"
	"strconv"
	"strings"
)

var leaseMonths = regexp.MustCompile(`(\d+)\s*(month|mo\b|week|day)`)

func ShortLease(months int) bool {
	return months > 0 && months <= ShortTermMaxMonths
}

func ShortLeaseText(term string) bool {
	lower := strings.ToLower(term)
	if AmenitiesFromText(lower)[AmenityShortTerm] {
		return true
	}
	m := leaseMonths.FindStringSubmatch(lower)
	if m == nil {
		return false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return false
	}
	switch m[2] {
	case "week", "day":
		return true
	}
	return ShortLease(n)
}
