package facebook

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

type image struct {
	URI string `json:"uri"`
}

type item struct {
	ID           string `json:"id"`
	Title        string `json:"marketplace_listing_title"`
	CustomTitle  string `json:"custom_title"`
	ListingURL   string `json:"listingUrl"`
	CreationTime int64  `json:"creation_time"`
	Price        struct {
		Amount   string `json:"amount"`
		Currency string `json:"currency"`
	} `json:"listing_price"`
	Location struct {
		Latitude  *float64 `json:"latitude"`
		Longitude *float64 `json:"longitude"`
		Geocode   struct {
			City       string `json:"city"`
			State      string `json:"state"`
			PostalCode string `json:"postal_code"`
		} `json:"reverse_geocode_detailed"`
	} `json:"location"`
	HomeAddress struct {
		Street *string `json:"street"`
	} `json:"home_address"`
	UnitRoomInfo string          `json:"unit_room_info"`
	UnitAreaInfo json.RawMessage `json:"unit_area_info"`
	Description  struct {
		Text string `json:"text"`
	} `json:"redacted_description"`
	ListingPhotos []struct {
		Image image `json:"image"`
	} `json:"listing_photos"`
	PrimaryPhotoURL string `json:"primary_listing_photo_url"`
	PrimaryPhoto    struct {
		Image image `json:"image"`
	} `json:"primary_listing_photo"`
}

var (
	bedsPattern  = regexp.MustCompile(`(?i)(\d+)\s*(?:beds?|br|bedrooms?)\b`)
	bathsPattern = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(?:baths?|ba|bathrooms?)\b`)
	studio       = regexp.MustCompile(`(?i)\bstudio\b`)
	room         = regexp.MustCompile(`(?i)\b(?:private\s+)?room\b`)
	sqftPattern  = regexp.MustCompile(`(?i)(\d[\d,]*)\s*(?:sq\.?\s*ft|square\s+feet|ft²|sqft)`)
)

func toListing(raw json.RawMessage, now time.Time) (listing.Listing, bool, error) {
	var it item
	if err := json.Unmarshal(raw, &it); err != nil {
		return listing.Listing{}, false, fmt.Errorf("decode facebook item: %w", err)
	}
	if it.ID == "" {
		return listing.Listing{}, false, nil
	}
	amount, err := strconv.ParseFloat(it.Price.Amount, 64)
	if err != nil || amount <= 0 {
		return listing.Listing{}, false, nil
	}
	rooms := strings.Join([]string{it.UnitRoomInfo, it.CustomTitle, it.Title}, " · ")
	beds := parseBeds(rooms)
	if beds == nil {
		return listing.Listing{}, false, nil
	}

	currency := it.Price.Currency
	if currency == "" {
		currency = "USD"
	}
	l := listing.Listing{
		Source:      listing.SourceFacebook,
		SourceID:    it.ID,
		URL:         cmpOr(it.ListingURL, "https://www.facebook.com/marketplace/item/"+it.ID+"/"),
		Offer:       listing.OfferRent,
		Price:       listing.Money{Cents: int64(math.Round(amount * 100)), Currency: currency},
		Address:     address(it),
		Beds:        beds,
		Baths:       parseBaths(rooms),
		SqFt:        parseSqFt(string(it.UnitAreaInfo) + " " + it.Description.Text),
		Description: strings.TrimSpace(it.Description.Text),
		Photos:      photos(it),
		ObservedAt:  now,
		Raw:         raw,
	}
	if it.Location.Latitude != nil && it.Location.Longitude != nil {
		l.Coordinates = &listing.Coordinates{Lat: *it.Location.Latitude, Lng: *it.Location.Longitude}
	}
	if it.CreationTime > 0 {
		t := time.Unix(it.CreationTime, 0).UTC()
		l.ListedAt = &t
	}
	if found := listing.AmenitiesFromText(it.Title + ". " + it.Description.Text); len(found) > 0 {
		l.Amenities = found
	}
	return l, true, nil
}

func parseBeds(s string) *int {
	if m := bedsPattern.FindStringSubmatch(s); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil {
			return &n
		}
	}
	if studio.MatchString(s) {
		n := 0
		return &n
	}
	if room.MatchString(s) {
		n := 1
		return &n
	}
	return nil
}

func parseBaths(s string) *float64 {
	m := bathsPattern.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	f, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return nil
	}
	return &f
}

func parseSqFt(s string) *int {
	m := sqftPattern.FindStringSubmatch(s)
	if m == nil {
		return nil
	}
	n, err := strconv.Atoi(strings.ReplaceAll(m[1], ",", ""))
	if err != nil || n < 100 {
		return nil
	}
	return &n
}

func address(it item) listing.Address {
	g := it.Location.Geocode
	zip := g.PostalCode
	if i := strings.Index(zip, "-"); i > 0 {
		zip = zip[:i]
	}
	var street string
	if it.HomeAddress.Street != nil {
		street = strings.TrimSpace(*it.HomeAddress.Street)
	}
	var parts []string
	for _, part := range []string{street, g.City, strings.TrimSpace(g.State + " " + zip)} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	formatted := strings.Join(parts, ", ")
	if formatted == "" {
		formatted = it.Title
	}
	return listing.Address{Formatted: formatted, Street: street, City: g.City, State: g.State, PostalCode: zip}
}

func photos(it item) []string {
	out := []string{}
	for _, p := range it.ListingPhotos {
		if p.Image.URI != "" {
			out = append(out, p.Image.URI)
		}
	}
	if len(out) == 0 {
		if u := cmpOr(it.PrimaryPhoto.Image.URI, it.PrimaryPhotoURL); u != "" {
			out = append(out, u)
		}
	}
	return out
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
