package zillow

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const DefaultAutocompleteURL = "https://www.zillowstatic.com/autocomplete/v3/suggestions"

var ErrRegionNotFound = errors.New("zillow region not found")

type Region struct {
	Name   string
	ID     int
	Type   string
	Center listing.Coordinates
}

type RegionResolver interface {
	Resolve(ctx context.Context, query string) (Region, error)
}

var regionTypeCodes = map[string]int{
	"state":        2,
	"county":       4,
	"city":         6,
	"zipcode":      7,
	"neighborhood": 8,
}

var regionSpanMiles = map[string]float64{
	"state":        600,
	"county":       80,
	"city":         40,
	"zipcode":      15,
	"neighborhood": 10,
}

type Autocomplete struct {
	URL       string
	UserAgent string
	HTTP      *http.Client
}

func NewAutocomplete() *Autocomplete {
	return &Autocomplete{URL: DefaultAutocompleteURL, UserAgent: "Mozilla/5.0", HTTP: http.DefaultClient}
}

func (a *Autocomplete) Resolve(ctx context.Context, query string) (Region, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL+"?q="+url.QueryEscape(query), nil)
	if err != nil {
		return Region{}, err
	}
	req.Header.Set("User-Agent", a.UserAgent)

	resp, err := a.HTTP.Do(req)
	if err != nil {
		return Region{}, fmt.Errorf("resolve %q: %w", query, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Region{}, fmt.Errorf("resolve %q: %s", query, resp.Status)
	}

	var body struct {
		Results []struct {
			Display  string `json:"display"`
			MetaData struct {
				RegionID   int     `json:"regionId"`
				RegionType string  `json:"regionType"`
				Lat        float64 `json:"lat"`
				Lng        float64 `json:"lng"`
			} `json:"metaData"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return Region{}, fmt.Errorf("resolve %q: %w", query, err)
	}
	if len(body.Results) == 0 {
		return Region{}, fmt.Errorf("%w: %q", ErrRegionNotFound, query)
	}

	r := body.Results[0]
	return Region{
		Name:   r.Display,
		ID:     r.MetaData.RegionID,
		Type:   r.MetaData.RegionType,
		Center: listing.Coordinates{Lat: r.MetaData.Lat, Lng: r.MetaData.Lng},
	}, nil
}
