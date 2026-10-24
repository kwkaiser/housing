package zillow

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const DetailActorID = "maxcopell/zillow-detail-scraper"

type detailInput struct {
	StartURLs            []actorURL `json:"startUrls"`
	PropertyStatus       string     `json:"propertyStatus"`
	ExtractBuildingUnits string     `json:"extractBuildingUnits"`
}

type detailItem struct {
	zListing
	InputURL       string     `json:"addressOrUrlFromInput"`
	Description    string     `json:"description"`
	OnMarketDate   *time.Time `json:"onMarketDate"`
	AtAGlanceFacts []struct {
		Label string `json:"factLabel"`
		Value string `json:"factValue"`
	} `json:"atAGlanceFacts"`
	Parking *struct {
		TotalSpaces int  `json:"totalSpaces"`
		HasGarage   bool `json:"hasGarage"`
	} `json:"parking"`
	PropertyFeatures struct {
		Appliances []string `json:"appliances"`
		Cooling    []string `json:"cooling"`
	} `json:"propertyFeatures"`
	BuildingAttributes *struct {
		Appliances              []string          `json:"appliances"`
		AirConditioning         string            `json:"airConditioning"`
		ParkingTypes            []string          `json:"parkingTypes"`
		DetailedParkingPolicies []json.RawMessage `json:"detailedParkingPolicies"`
		CustomUnitAmenities     []string          `json:"customUnitAmenities"`
		CustomPropertyAmenities []string          `json:"customPropertyAmenities"`
	} `json:"buildingAttributes"`
	FloorPlans []struct {
		Name       string   `json:"name"`
		Bedrooms   *float64 `json:"bedrooms"`
		Bathrooms  *float64 `json:"bathrooms"`
		LivingArea *float64 `json:"livingArea"`
		MinPrice   *float64 `json:"minPrice"`
		Units      []struct {
			UnitNumber string   `json:"unitNumber"`
			ZPID       flexID   `json:"zpid"`
			Price      *float64 `json:"price"`
			LivingArea *float64 `json:"livingArea"`
		} `json:"units"`
	} `json:"listingFloorPlans"`
}

func (p *Provider) Enrich(ctx context.Context, listings []listing.Listing) ([]listing.Listing, error) {
	urlsByOffer := map[listing.OfferType][]string{}
	seen := map[string]bool{}
	for _, l := range listings {
		if l.Source != listing.SourceZillow || seen[l.URL] {
			continue
		}
		seen[l.URL] = true
		urlsByOffer[l.Offer] = append(urlsByOffer[l.Offer], l.URL)
	}

	details := map[string][]detailResult{}
	for offer, urls := range urlsByOffer {
		status, units := "FOR_SALE", "for_sale"
		if offer == listing.OfferRent {
			status, units = "FOR_RENT", "for_rent"
		}
		input := detailInput{PropertyStatus: status, ExtractBuildingUnits: units}
		for _, u := range urls {
			input.StartURLs = append(input.StartURLs, actorURL{URL: u})
		}
		items, err := p.Runner.Run(ctx, DetailActorID, input)
		if err != nil {
			return nil, err
		}
		for _, raw := range items {
			var d detailItem
			if err := json.Unmarshal(raw, &d); err != nil {
				return nil, fmt.Errorf("decode zillow detail: %w", err)
			}
			if d.Error != "" {
				continue
			}
			key, unit, isUnit := strings.Cut(cmp.Or(d.InputURL, d.PropertyURL), buildingUnitMarker)
			key = strings.TrimSpace(key)
			details[key] = append(details[key], detailResult{item: d, raw: raw, buildingUnit: isUnit && unit != ""})
		}
	}

	var out []listing.Listing
	expanded := map[string]bool{}
	for _, l := range listings {
		ds, ok := details[l.URL]
		if l.Source != listing.SourceZillow || !ok {
			out = append(out, l)
			continue
		}
		if len(ds) == 1 && !ds[0].buildingUnit && len(ds[0].item.FloorPlans) == 0 {
			out = append(out, enrichHome(l, ds[0]))
			continue
		}
		if expanded[l.URL] {
			continue
		}
		expanded[l.URL] = true
		for _, d := range ds {
			if len(d.item.FloorPlans) > 0 {
				out = append(out, expandBuilding(l, d)...)
				continue
			}
			unit := d.item.base(l.Offer, l.URL, d.raw)
			unit.Amenities = l.Amenities
			if d.item.ListingPrice == nil || d.item.ListingPrice.Amount == 0 {
				continue
			}
			unit.Price = money(d.item.ListingPrice.Amount, d.item.ListingPrice.Currency)
			out = append(out, enrichHome(unit, d))
		}
	}
	return out, nil
}

const buildingUnitMarker = "--building->"

type detailResult struct {
	item         detailItem
	raw          json.RawMessage
	buildingUnit bool
}

func enrichHome(l listing.Listing, d detailResult) listing.Listing {
	it := d.item
	if l.Beds == nil {
		l.Beds = intPtr(it.Bedrooms)
	}
	if l.Baths == nil {
		l.Baths = it.Bathrooms
	}
	if l.SqFt == nil {
		l.SqFt = intPtr(it.LivingArea)
	}
	if it.OnMarketDate != nil {
		t := *it.OnMarketDate
		l.ListedAt = &t
	}
	if it.Description != "" {
		l.Description = it.Description
	}
	if ps := it.photoURLs(); len(ps) > 0 {
		l.Photos = ps
	}
	l.Amenities = mergeAmenities(l.Amenities, homeAmenities(it), listing.AmenitiesFromText(it.Description))
	l.Raw = d.raw
	return l
}

func expandBuilding(base listing.Listing, d detailResult) []listing.Listing {
	it := d.item
	base.Address.Unit = ""
	base.Baths, base.SqFt, base.ListedAt = nil, nil, nil
	if ps := it.photoURLs(); len(ps) > 0 {
		base.Photos = ps
	}
	base.Amenities = mergeAmenities(base.Amenities, buildingAmenities(it), listing.AmenitiesFromText(it.Description))
	if it.Description != "" {
		base.Description = it.Description
	}
	rawPlans := rawFloorPlans(d.raw)

	var out []listing.Listing
	for i, fp := range it.FloorPlans {
		var rp rawFloorPlan
		if i < len(rawPlans) {
			rp = rawPlans[i]
		}
		plan := base
		plan.Beds = intPtr(fp.Bedrooms)
		plan.Baths = fp.Bathrooms
		plan.SqFt = intPtr(fp.LivingArea)

		if len(fp.Units) == 0 {
			if fp.MinPrice == nil {
				continue
			}
			l := plan
			l.SourceID = fmt.Sprintf("%s#%s", it.ZPID, fp.Name)
			l.Price = money(*fp.MinPrice, "USD")
			l.Raw = unitRaw(it.ZPID, rp.plan, nil)
			out = append(out, l)
			continue
		}

		for j, u := range fp.Units {
			price := u.Price
			if price == nil {
				price = fp.MinPrice
			}
			if price == nil || u.ZPID == "" {
				continue
			}
			l := plan
			l.SourceID = string(u.ZPID)
			l.Price = money(*price, "USD")
			l.Address.Unit = strings.TrimSpace(strings.TrimPrefix(u.UnitNumber, "Unit"))
			if u.LivingArea != nil {
				l.SqFt = intPtr(u.LivingArea)
			}
			var ru json.RawMessage
			if j < len(rp.units) {
				ru = rp.units[j]
			}
			l.Raw = unitRaw(it.ZPID, rp.plan, ru)
			out = append(out, l)
		}
	}
	return out
}

func homeAmenities(it detailItem) map[listing.Amenity]bool {
	out := map[listing.Amenity]bool{}
	appliances := it.PropertyFeatures.Appliances
	if hasFold(appliances, "dishwasher") {
		out[listing.AmenityDishwasher] = true
	}
	if hasFold(appliances, "washer") && hasFold(appliances, "dryer") {
		out[listing.AmenityInUnitLaundry] = true
	}
	for _, f := range it.AtAGlanceFacts {
		if f.Label == "Laundry" && strings.Contains(strings.ToLower(f.Value), "in unit") {
			out[listing.AmenityInUnitLaundry] = true
		}
	}
	for _, c := range it.PropertyFeatures.Cooling {
		if !strings.EqualFold(c, "none") {
			out[listing.AmenityAirConditioning] = true
		}
	}
	if it.Parking != nil && (it.Parking.TotalSpaces > 0 || it.Parking.HasGarage) {
		out[listing.AmenityParking] = true
	}
	return out
}

func buildingAmenities(it detailItem) map[listing.Amenity]bool {
	out := map[listing.Amenity]bool{}
	ba := it.BuildingAttributes
	if ba == nil {
		return out
	}
	custom := append(append([]string{}, ba.CustomUnitAmenities...), ba.CustomPropertyAmenities...)
	if hasFold(ba.Appliances, "dishwasher") || containsFold(custom, "dishwasher") {
		out[listing.AmenityDishwasher] = true
	}
	if (hasFold(ba.Appliances, "washer") && hasFold(ba.Appliances, "dryer")) ||
		containsFold(custom, "washer-dryer", "washer/dryer", "washer and dryer", "in-unit laundry", "in unit laundry") {
		out[listing.AmenityInUnitLaundry] = true
	}
	switch strings.ToLower(ba.AirConditioning) {
	case "", "unknown", "none":
	default:
		out[listing.AmenityAirConditioning] = true
	}
	if len(ba.ParkingTypes) > 0 || len(ba.DetailedParkingPolicies) > 0 {
		out[listing.AmenityParking] = true
	}
	return out
}

type rawFloorPlan struct {
	plan  map[string]json.RawMessage
	units []json.RawMessage
}

func rawFloorPlans(raw json.RawMessage) []rawFloorPlan {
	var doc struct {
		FloorPlans []map[string]json.RawMessage `json:"listingFloorPlans"`
	}
	if json.Unmarshal(raw, &doc) != nil {
		return nil
	}
	out := make([]rawFloorPlan, len(doc.FloorPlans))
	for i, fp := range doc.FloorPlans {
		json.Unmarshal(fp["units"], &out[i].units)
		delete(fp, "units")
		out[i].plan = fp
	}
	return out
}

func unitRaw(buildingZPID flexID, plan map[string]json.RawMessage, unit json.RawMessage) json.RawMessage {
	b, err := json.Marshal(struct {
		BuildingZPID string                     `json:"buildingZpid"`
		FloorPlan    map[string]json.RawMessage `json:"floorPlan"`
		Unit         json.RawMessage            `json:"unit,omitempty"`
	}{string(buildingZPID), plan, unit})
	if err != nil {
		return nil
	}
	return b
}

func (p *Provider) Lookup(ctx context.Context, rawURL string) (listing.Listing, error) {
	u, err := url.Parse(rawURL)
	if err != nil || !strings.HasSuffix(u.Hostname(), "zillow.com") {
		return listing.Listing{}, fmt.Errorf("not a zillow url: %q", rawURL)
	}
	u.RawQuery, u.Fragment = "", ""

	items, err := p.Runner.Run(ctx, DetailActorID, detailInput{
		StartURLs:            []actorURL{{URL: u.String()}},
		PropertyStatus:       "FOR_RENT",
		ExtractBuildingUnits: "disabled",
	})
	if err != nil {
		return listing.Listing{}, err
	}

	for _, raw := range items {
		var d detailItem
		if err := json.Unmarshal(raw, &d); err != nil {
			return listing.Listing{}, fmt.Errorf("decode zillow detail: %w", err)
		}
		if d.Error != "" || d.ZPID == "" {
			continue
		}
		base := d.base("", u.String(), raw)
		if d.ListingPrice != nil && d.ListingPrice.Amount > 0 {
			base.Price = money(d.ListingPrice.Amount, d.ListingPrice.Currency)
		}
		return enrichHome(base, detailResult{item: d, raw: raw}), nil
	}
	return listing.Listing{}, fmt.Errorf("zillow detail scraper returned nothing for %s", u)
}
