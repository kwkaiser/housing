package cli

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

type searchFlags struct {
	location  string
	radius    float64
	minPrice  int
	maxPrice  int
	minBeds   int
	maxBeds   int
	maxAge    string
	amenities []string
	limit     int
}

func (s *searchFlags) register(f *pflag.FlagSet) {
	f.StringVar(&s.location, "location", "", "city, ZIP, neighborhood, county or state")
	f.Float64Var(&s.radius, "radius", 0, "search radius in miles around the location")
	f.IntVar(&s.minPrice, "min-price", 0, "minimum price in dollars (monthly rent in rent mode)")
	f.IntVar(&s.maxPrice, "max-price", 0, "maximum price in dollars (monthly rent in rent mode)")
	f.IntVar(&s.minBeds, "min-beds", 0, "minimum bedrooms")
	f.IntVar(&s.maxBeds, "max-beds", 0, "maximum bedrooms")
	f.StringVar(&s.maxAge, "max-age", "", "maximum listing age, e.g. 7d or 36h")
	f.StringSliceVar(&s.amenities, "amenity", nil, "required amenity (repeatable): "+amenityNames())
	f.IntVar(&s.limit, "limit", 0, "maximum search results")
}

func (s *searchFlags) apply(cmd *cobra.Command, base profile.Search) (profile.Search, error) {
	out := base
	changed := cmd.Flags().Changed
	if changed("location") {
		out.Location = s.location
	}
	if changed("radius") {
		out.RadiusMiles = s.radius
	}
	if changed("min-price") {
		out.MinPrice = intPtr(s.minPrice)
	}
	if changed("max-price") {
		out.MaxPrice = intPtr(s.maxPrice)
	}
	if changed("min-beds") {
		out.MinBeds = intPtr(s.minBeds)
	}
	if changed("max-beds") {
		out.MaxBeds = intPtr(s.maxBeds)
	}
	if changed("max-age") {
		d, err := parseAge(s.maxAge)
		if err != nil {
			return profile.Search{}, err
		}
		out.MaxAgeDays = int(math.Ceil(d.Hours() / 24))
	}
	if changed("amenity") {
		out.Amenities = nil
		for _, a := range s.amenities {
			out.Amenities = append(out.Amenities, listing.Amenity(a))
		}
	}
	if changed("limit") {
		out.Limit = s.limit
	}
	return out, out.Validate()
}

var searchFlagNames = []string{"location", "radius", "min-price", "max-price", "min-beds", "max-beds", "max-age", "amenity", "limit"}

func (s *searchFlags) anyChanged(cmd *cobra.Command) bool {
	for _, name := range searchFlagNames {
		if cmd.Flags().Changed(name) {
			return true
		}
	}
	return false
}

func intPtr(v int) *int {
	return &v
}

func amenityNames() string {
	names := make([]string, len(listing.Amenities))
	for i, a := range listing.Amenities {
		names[i] = string(a)
	}
	return strings.Join(names, ", ")
}

func parseAge(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, fmt.Errorf("invalid --max-age %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid --max-age %q", s)
	}
	return d, nil
}
