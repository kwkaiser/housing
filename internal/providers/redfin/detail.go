package redfin

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"maps"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const DetailActorID = "tri_angle/redfin-detail"

var _ listing.Enricher = (*Provider)(nil)

type detailInput struct {
	DetailURLs []actorURL `json:"detailUrls"`
}

type detailItem struct {
	Input         string `json:"scraperInput"`
	MainHouseInfo struct {
		MarketingRemarks []struct {
			Remark string `json:"marketingRemark"`
		} `json:"marketingRemarks"`
		SelectedAmenities []struct {
			Header  string `json:"header"`
			Content string `json:"content"`
		} `json:"selectedAmenities"`
	} `json:"mainHouseInfo"`
	MediaBrowserInfo struct {
		Photos []struct {
			PhotoURLs struct {
				FullScreen string `json:"fullScreenPhotoUrl"`
			} `json:"photoUrls"`
		} `json:"photos"`
	} `json:"mediaBrowserInfo"`
}

func (p *Provider) Enrich(ctx context.Context, listings []listing.Listing) ([]listing.Listing, error) {
	var input detailInput
	seen := map[string]bool{}
	for _, l := range listings {
		if l.Source != listing.SourceRedfin || l.Offer != listing.OfferSale || seen[l.URL] {
			continue
		}
		seen[l.URL] = true
		input.DetailURLs = append(input.DetailURLs, actorURL{URL: l.URL})
	}
	if len(input.DetailURLs) == 0 {
		return listings, nil
	}

	items, err := p.Runner.Run(ctx, DetailActorID, input)
	if err != nil {
		return nil, err
	}
	details := make(map[string]detailItem, len(items))
	for _, raw := range items {
		var d detailItem
		if err := json.Unmarshal(raw, &d); err != nil {
			return nil, fmt.Errorf("decode redfin detail: %w", err)
		}
		if d.Input != "" {
			details[d.Input] = d
		}
	}

	out := make([]listing.Listing, len(listings))
	for i, l := range listings {
		if d, ok := details[l.URL]; ok && l.Source == listing.SourceRedfin {
			l = enrichSale(l, d)
		}
		out[i] = l
	}
	return out, nil
}

func enrichSale(l listing.Listing, d detailItem) listing.Listing {
	var remarks []string
	for _, r := range d.MainHouseInfo.MarketingRemarks {
		if text := strings.TrimSpace(html.UnescapeString(r.Remark)); text != "" {
			remarks = append(remarks, text)
		}
	}
	if full := strings.Join(remarks, "\n\n"); len(full) > len(l.Description) {
		l.Description = full
	}

	var photos []string
	for _, p := range d.MediaBrowserInfo.Photos {
		if u := p.PhotoURLs.FullScreen; u != "" {
			photos = append(photos, u)
		}
	}
	if len(photos) > 0 {
		l.Photos = photos
	}

	found := maps.Clone(l.Amenities)
	if found == nil {
		found = map[listing.Amenity]bool{}
	}
	var facts []string
	for _, a := range d.MainHouseInfo.SelectedAmenities {
		facts = append(facts, a.Header+": "+a.Content)
		maps.Copy(found, amenityFromFact(a.Header, a.Content))
	}
	maps.Copy(found, listing.AmenitiesFromText(l.Description+". "+strings.Join(facts, ". ")))
	if len(found) > 0 {
		l.Amenities = found
	}
	return l
}

func amenityFromFact(header, content string) map[listing.Amenity]bool {
	h, c := strings.ToLower(header), strings.ToLower(content)
	none := c == "" || strings.HasPrefix(c, "no") || strings.HasPrefix(c, "none") || strings.HasPrefix(c, "0 ")
	out := map[listing.Amenity]bool{}
	switch {
	case none:
	case h == "laundry" && (strings.Contains(c, "in-unit") || strings.Contains(c, "in unit")):
		out[listing.AmenityInUnitLaundry] = true
	case h == "parking" && !strings.Contains(c, "street"):
		out[listing.AmenityParking] = true
	case h == "cooling" || h == "air conditioning":
		out[listing.AmenityAirConditioning] = true
	case strings.Contains(c, "dishwasher"):
		out[listing.AmenityDishwasher] = true
	}
	return out
}
