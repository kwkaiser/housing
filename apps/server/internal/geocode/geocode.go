package geocode

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/telemetry"
)

const (
	DefaultURL = "https://photon.komoot.io"
	MaxResults = 8
	userAgent  = "housing (+https://git.kwkaiser.io/kwkaiser/housing)"
)

var ErrUpstream = errors.New("geocoder failed")

type Result struct {
	Name    string
	Address string
	Lat     float64
	Lng     float64
}

type Near struct {
	Lat, Lng float64
}

type Photon struct {
	URL    string
	Client *http.Client
}

func NewPhoton() *Photon {
	return &Photon{URL: DefaultURL, Client: telemetry.Client(10 * time.Second)}
}

type response struct {
	Features []struct {
		Properties properties `json:"properties"`
		Geometry   struct {
			Coordinates []float64 `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
}

type properties struct {
	Name        string `json:"name"`
	HouseNumber string `json:"housenumber"`
	Street      string `json:"street"`
	City        string `json:"city"`
	Town        string `json:"town"`
	Village     string `json:"village"`
	District    string `json:"district"`
	State       string `json:"state"`
	Postcode    string `json:"postcode"`
	Country     string `json:"country"`
}

func (p *Photon) Search(ctx context.Context, query string, near *Near, limit int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return []Result{}, nil
	}
	q := url.Values{"q": {query}, "lang": {"en"}, "limit": {strconv.Itoa(min(max(limit, 1), MaxResults))}}
	if near != nil {
		q.Set("lat", strconv.FormatFloat(near.Lat, 'f', 5, 64))
		q.Set("lon", strconv.FormatFloat(near.Lng, 'f', 5, 64))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(p.URL, "/")+"/api/?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	res, err := p.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUpstream, err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrUpstream, res.StatusCode)
	}
	var body response
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUpstream, err)
	}
	out := []Result{}
	seen := map[string]bool{}
	for _, f := range body.Features {
		if len(f.Geometry.Coordinates) != 2 {
			continue
		}
		r := f.Properties.result()
		r.Lng, r.Lat = f.Geometry.Coordinates[0], f.Geometry.Coordinates[1]
		key := r.Name + "|" + r.Address
		if r.Address == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, r)
	}
	return out, nil
}

func (p properties) result() Result {
	street := strings.TrimSpace(p.HouseNumber + " " + p.Street)
	name := p.Name
	if street == "" || p.HouseNumber == "" && startsWithDigit(name) {
		street, name = cmp.Or(name, street), ""
	}
	if name == street {
		name = ""
	}
	region := strings.TrimSpace(p.State + " " + p.Postcode)
	var parts []string
	for _, s := range []string{street, cmp.Or(p.City, p.Town, p.Village, p.District), region} {
		if s != "" {
			parts = append(parts, s)
		}
	}
	if p.Country != "" && p.Country != "United States" {
		parts = append(parts, p.Country)
	}
	return Result{Name: name, Address: strings.Join(parts, ", ")}
}

func startsWithDigit(s string) bool {
	return s != "" && s[0] >= '0' && s[0] <= '9'
}
