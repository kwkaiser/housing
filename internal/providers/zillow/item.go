package zillow

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const noResults = "No results found."

type item struct {
	zListing
	CardType         string `json:"cardType"`
	LivingAreaUnit   string `json:"livingAreaUnit"`
	DaysOnZillow     *int   `json:"daysOnZillow"`
	MainImage        string `json:"mainImage"`
	FactsAndFeatures struct {
		HasAirConditioning *bool `json:"hasAirConditioning"`
	} `json:"factsAndFeatures"`
	Units []struct {
		Price string `json:"price"`
		Beds  string `json:"beds"`
	} `json:"units"`
}

func toListings(raw json.RawMessage, q listing.Query) ([]listing.Listing, error) {
	var it item
	if err := json.Unmarshal(raw, &it); err != nil {
		return nil, fmt.Errorf("decode zillow item: %w", err)
	}
	if it.Error != "" {
		if it.Error == noResults {
			return nil, nil
		}
		return nil, fmt.Errorf("zillow scraper: %s", it.Error)
	}
	if it.ZPID == "" || it.PropertyURL == "" {
		return nil, nil
	}

	base := it.base(q.Offer, it.PropertyURL, raw)
	if len(base.Photos) == 0 && it.MainImage != "" {
		base.Photos = []string{it.MainImage}
	}
	base.Amenities = amenities(it, q)

	if len(it.Units) > 0 {
		return unitListings(base, it), nil
	}

	if it.ListingPrice == nil {
		return nil, nil
	}
	l := base
	l.Price = money(it.ListingPrice.Amount, it.ListingPrice.Currency)
	if it.LivingAreaUnit != "" && it.LivingAreaUnit != "sqft" {
		l.SqFt = nil
	}
	if it.DaysOnZillow != nil {
		t := l.ObservedAt.Add(-time.Duration(*it.DaysOnZillow) * day)
		l.ListedAt = &t
	}
	return []listing.Listing{l}, nil
}

func unitListings(base listing.Listing, it item) []listing.Listing {
	base.Address.Unit = ""
	var out []listing.Listing
	for i, u := range it.Units {
		cents, ok := parseDollars(u.Price)
		if !ok {
			continue
		}
		l := base
		l.SourceID = fmt.Sprintf("%s#%d", it.ZPID, i)
		l.Price = listing.Money{Cents: cents, Currency: "USD"}
		if beds, err := strconv.Atoi(u.Beds); err == nil {
			l.Beds = &beds
		}
		out = append(out, l)
	}
	return out
}

func amenities(it item, q listing.Query) map[listing.Amenity]bool {
	out := map[listing.Amenity]bool{}
	for _, a := range q.Amenities {
		out[a] = true
	}
	if ac := it.FactsAndFeatures.HasAirConditioning; ac != nil && *ac {
		out[listing.AmenityAirConditioning] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
