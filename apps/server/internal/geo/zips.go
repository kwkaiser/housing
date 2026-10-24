package geo

import (
	"cmp"
	_ "embed"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

//go:embed zips.tsv
var zipsTSV string

type Zip struct {
	Code        string
	Center      listing.Coordinates
	RadiusMiles float64
}

var loadZips = sync.OnceValue(func() []Zip {
	zips, err := parseZips(zipsTSV)
	if err != nil {
		panic(err)
	}
	return zips
})

func parseZips(tsv string) ([]Zip, error) {
	var out []Zip
	for i, line := range strings.Split(strings.TrimSpace(tsv), "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 4 {
			return nil, fmt.Errorf("zips.tsv line %d: want 4 fields, got %d", i+1, len(f))
		}
		var nums [3]float64
		for j, s := range f[1:] {
			v, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return nil, fmt.Errorf("zips.tsv line %d: %w", i+1, err)
			}
			nums[j] = v
		}
		out = append(out, Zip{Code: f[0], Center: listing.Coordinates{Lat: nums[0], Lng: nums[1]}, RadiusMiles: nums[2]})
	}
	return out, nil
}

func LookupZip(code string) (Zip, bool) {
	zips := loadZips()
	i := slices.IndexFunc(zips, func(z Zip) bool { return z.Code == code })
	if i < 0 {
		return Zip{}, false
	}
	return zips[i], true
}

func ZipsWithin(c listing.Coordinates, radiusMiles float64) []Zip {
	type hit struct {
		zip  Zip
		dist float64
	}
	var hits []hit
	for _, z := range loadZips() {
		if d := DistanceMiles(c, z.Center); d-z.RadiusMiles <= radiusMiles {
			hits = append(hits, hit{z, d})
		}
	}
	slices.SortFunc(hits, func(a, b hit) int { return cmp.Compare(a.dist, b.dist) })
	out := make([]Zip, len(hits))
	for i, h := range hits {
		out[i] = h.zip
	}
	return out
}
