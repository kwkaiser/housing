package geo

import (
	_ "embed"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

//go:embed centroids.tsv
var centroidsTSV string

var loadCentroids = sync.OnceValue(func() map[string]listing.Coordinates {
	m, err := parseCentroids(centroidsTSV)
	if err != nil {
		panic(err)
	}
	return m
})

func parseCentroids(tsv string) (map[string]listing.Coordinates, error) {
	lines := strings.Split(strings.TrimSpace(tsv), "\n")
	out := make(map[string]listing.Coordinates, len(lines))
	for i, line := range lines {
		f := strings.Split(line, "\t")
		if len(f) != 3 {
			return nil, fmt.Errorf("centroids.tsv line %d: want 3 fields, got %d", i+1, len(f))
		}
		lat, err := strconv.ParseFloat(f[1], 64)
		if err != nil {
			return nil, fmt.Errorf("centroids.tsv line %d: %w", i+1, err)
		}
		lng, err := strconv.ParseFloat(f[2], 64)
		if err != nil {
			return nil, fmt.Errorf("centroids.tsv line %d: %w", i+1, err)
		}
		out[f[0]] = listing.Coordinates{Lat: lat, Lng: lng}
	}
	return out, nil
}

func ZipCentroid(code string) (listing.Coordinates, bool) {
	c, ok := loadCentroids()[code]
	return c, ok
}

var trailingZip = regexp.MustCompile(`\b(\d{5})(?:-\d{4})?\s*$`)

func postalCode(a listing.Address) string {
	if z := strings.TrimSpace(a.PostalCode); len(z) >= 5 {
		return z[:5]
	}
	if m := trailingZip.FindStringSubmatch(a.Formatted); m != nil {
		return m[1]
	}
	return ""
}

func Locate(l listing.Listing) (c listing.Coordinates, approx, ok bool) {
	if l.Coordinates != nil && (l.Coordinates.Lat != 0 || l.Coordinates.Lng != 0) {
		return *l.Coordinates, false, true
	}
	if z := postalCode(l.Address); z != "" {
		if c, ok := ZipCentroid(z); ok {
			return c, true, true
		}
	}
	return listing.Coordinates{}, false, false
}
