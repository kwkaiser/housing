package craigslist

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const postedLayout = "2006-01-02T15:04:05-0700"

var (
	digits     = regexp.MustCompile(`[\d.]+`)
	photoSize  = regexp.MustCompile(`_\d+x\d+\.jpg$`)
	parkingYes = []string{"off-street parking", "attached garage", "detached garage", "carport", "valet parking"}
)

type item struct {
	ID        string   `json:"id"`
	URL       string   `json:"url"`
	Title     string   `json:"title"`
	Datetime  string   `json:"datetime"`
	Location  string   `json:"location"`
	Price     string   `json:"price"`
	Latitude  string   `json:"latitude"`
	Longitude string   `json:"longitude"`
	Post      string   `json:"post"`
	Pics      []string `json:"pics"`
	Amenities []string `json:"amenities"`
	Bedrooms  string   `json:"bedrooms"`
	Bathrooms string   `json:"bathrooms"`
	Space     string   `json:"space"`
	Address   struct {
		PostalCode string `json:"postalCode"`
		Street     string `json:"street"`
		City       string `json:"city"`
	} `json:"address"`
	ScrapedAt time.Time `json:"scrapedAt"`
}

func toListing(raw json.RawMessage, now time.Time) (listing.Listing, bool, error) {
	var it item
	if err := json.Unmarshal(raw, &it); err != nil {
		return listing.Listing{}, false, fmt.Errorf("decode craigslist item: %w", err)
	}
	cents, ok := parseDollars(it.Price)
	if it.ID == "" || it.URL == "" || !ok || cents == 0 {
		return listing.Listing{}, false, nil
	}

	observed := now
	if !it.ScrapedAt.IsZero() {
		observed = it.ScrapedAt.UTC()
	}
	l := listing.Listing{
		Source:      listing.SourceCraigslist,
		SourceID:    it.ID,
		URL:         it.URL,
		Offer:       listing.OfferRent,
		Price:       listing.Money{Cents: cents, Currency: "USD"},
		Address:     address(it),
		Beds:        parseInt(it.Bedrooms),
		Baths:       parseFloat(it.Bathrooms),
		SqFt:        parseInt(it.Space),
		Description: strings.TrimSpace(it.Title + "\n\n" + it.Post),
		Photos:      photos(it.Pics),
		ObservedAt:  observed,
		Raw:         raw,
	}
	if lat, lng := parseFloat(it.Latitude), parseFloat(it.Longitude); lat != nil && lng != nil {
		l.Coordinates = &listing.Coordinates{Lat: *lat, Lng: *lng}
	}
	if t, err := time.Parse(postedLayout, it.Datetime); err == nil {
		t = t.UTC()
		l.ListedAt = &t
	}
	l.Amenities = amenities(it)
	return l, true, nil
}

func address(it item) listing.Address {
	var areas []string
	for _, part := range strings.Split(it.Location, `\`) {
		part = strings.TrimSpace(part)
		if part != "" && !containsFold(areas, part) {
			areas = append(areas, part)
		}
	}
	street := strings.TrimSpace(it.Address.Street)
	cross := strings.TrimSpace(it.Address.City)
	var parts []string
	for _, s := range []string{street, cross, strings.Join(areas, " / ")} {
		if s != "" && !containsFold(parts, s) {
			parts = append(parts, s)
		}
	}
	a := listing.Address{Formatted: strings.Join(parts, ", "), Street: street, PostalCode: it.Address.PostalCode}
	if a.Formatted == "" {
		a.Formatted = it.Title
	}
	return a
}

func photos(pics []string) []string {
	out := make([]string, 0, len(pics))
	for _, p := range pics {
		if p == "" {
			continue
		}
		out = append(out, photoSize.ReplaceAllString(p, "_1200x900.jpg"))
	}
	return out
}

func amenities(it item) map[listing.Amenity]bool {
	found := listing.AmenitiesFromText(it.Title + ". " + it.Post)
	for _, a := range it.Amenities {
		a = strings.ToLower(strings.TrimSpace(a))
		switch {
		case a == "w/d in unit":
			found[listing.AmenityInUnitLaundry] = true
		case a == "air conditioning":
			found[listing.AmenityAirConditioning] = true
		case containsFold(parkingYes, a):
			found[listing.AmenityParking] = true
		}
	}
	if len(found) == 0 {
		return nil
	}
	return found
}

func parseDollars(s string) (int64, bool) {
	f := parseFloat(strings.ReplaceAll(s, ",", ""))
	if f == nil {
		return 0, false
	}
	return int64(math.Round(*f * 100)), true
}

func parseFloat(s string) *float64 {
	m := digits.FindString(s)
	if m == "" {
		return nil
	}
	f, err := strconv.ParseFloat(m, 64)
	if err != nil {
		return nil
	}
	return &f
}

func parseInt(s string) *int {
	f := parseFloat(s)
	if f == nil {
		return nil
	}
	v := int(*f)
	return &v
}

func containsFold(ss []string, s string) bool {
	for _, x := range ss {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}
