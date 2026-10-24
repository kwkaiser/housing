package app

import (
	"cmp"
	"context"
	"net/http"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/app/view"
	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
)

var mapColors = []string{"#2563eb", "#dc2626", "#16a34a", "#9333ea", "#ea580c", "#0891b2", "#db2777", "#65a30d"}

const defaultAreaMiles = 2

type mapCollection struct {
	ID    string
	Day   string
	Color string
	Count int
}

type mapPin struct {
	Lat          float64             `json:"lat"`
	Lng          float64             `json:"lng"`
	Approx       bool                `json:"approx,omitempty"`
	Title        string              `json:"title"`
	Href         string              `json:"href"`
	Price        string              `json:"price"`
	Beds         string              `json:"beds"`
	Matches      map[string]string   `json:"matches"`
	Scores       map[string]float64  `json:"scores"`
	Dealbreakers map[string][]string `json:"dealbreakers,omitempty"`
	Ungraded     bool                `json:"ungraded,omitempty"`
	FirstSeen    string              `json:"firstSeen"`
	LastSeen     string              `json:"lastSeen"`
	Preview      string              `json:"preview,omitempty"`
	Collections  []string            `json:"collections"`
}

type rangeLink struct {
	Label  string
	Href   string
	Active bool
}

type mapData struct {
	Collections  []mapCollection
	Pins         []mapPin
	Approx       int
	Unplaced     int
	Dealbreakers int
	Ungraded     int
	From         string
	To           string
	Quick        []rangeLink
	RunFirst     string
	RunLast      string
	Min          string
	Max          string
	Extent       map[string][][2]float64
	Config       mapConfig
}

func validDay(s string) string {
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return ""
	}
	return s
}

func day(t time.Time, years, months, days int) string {
	return t.AddDate(years, months, days).Format(time.DateOnly)
}

func (a *App) mapPage(w http.ResponseWriter, r *http.Request) {
	cs, err := a.svc.Collections(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	q := r.URL.Query()
	today := a.now()
	from, to := validDay(q.Get("from")), validDay(q.Get("to"))
	if !q.Has("from") && !q.Has("to") {
		from = day(today, 0, -1, 0)
	}
	if from != "" && to != "" && from > to {
		from, to = to, from
	}
	d, err := a.mapData(r.Context(), cs, from, to)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d.Quick = quickRanges(today, from, to)
	d.Config = a.mapConfig()
	a.render(w, r, http.StatusOK, "map", d)
}

func quickRanges(today time.Time, from, to string) []rangeLink {
	var links []rangeLink
	for _, l := range []struct {
		label, from, href string
	}{
		{"Last day", day(today, 0, 0, -1), ""},
		{"Last week", day(today, 0, 0, -7), ""},
		{"Last 2 weeks", day(today, 0, 0, -14), ""},
		{"Last month", day(today, 0, -1, 0), "/map"},
		{"All dates", "", "/map?from="},
	} {
		links = append(links, rangeLink{Label: l.label, Href: cmp.Or(l.href, "/map?from="+l.from), Active: from == l.from && to == ""})
	}
	return links
}

func (a *App) mapData(ctx context.Context, cs []collection.Collection, from, to string) (mapData, error) {
	d := mapData{Pins: []mapPin{}, Extent: map[string][][2]float64{}, From: from, To: to}
	seen := map[string]int{}
	var pins []mapPin
	for i, c := range cs {
		v, err := a.svc.CollectionRange(ctx, c.ID, from, to)
		if err != nil {
			return d, err
		}
		mc := mapCollection{ID: c.ID, Color: mapColors[i%len(mapColors)]}
		if n := len(v.Days); n > 0 {
			mc.Day = v.Days[n-1].Day
			if d.RunFirst == "" || v.Days[0].Day < d.RunFirst {
				d.RunFirst = v.Days[0].Day
			}
			d.RunLast = max(d.RunLast, mc.Day)
		}
		if area, ok := searchArea(c); ok {
			d.Extent[c.ID] = area
		}
		for _, row := range v.Rows {
			key := string(row.Listing.Source) + "/" + row.Listing.SourceID
			j, ok := seen[key]
			if !ok {
				loc, approx, placed := geo.Locate(row.Listing)
				if !placed {
					seen[key] = -1
					d.Unplaced++
					continue
				}
				j = len(pins)
				seen[key] = j
				pins = append(pins, mapPin{
					Lat: loc.Lat, Lng: loc.Lng, Approx: approx,
					Title:     cmp.Or(report.AddressLine(row.Listing), row.Listing.SourceID),
					Href:      listingURL(row.Listing, c.ID, row.Day),
					Price:     view.Money(row.Listing.Price.Cents, row.Listing.Offer),
					Beds:      view.Beds(row.Listing.Beds),
					Matches:   map[string]string{},
					Scores:    map[string]float64{},
					Ungraded:  row.Ungraded,
					FirstSeen: row.FirstSeen,
					LastSeen:  row.Day,
				})
				if len(row.Listing.Collages) > 0 {
					pins[j].Preview = "/media/" + row.Listing.Collages[0]
				}
			} else if j < 0 {
				continue
			}
			p := &pins[j]
			p.Collections = append(p.Collections, c.ID)
			p.FirstSeen = min(p.FirstSeen, row.FirstSeen)
			p.LastSeen = max(p.LastSeen, row.Day)
			if !row.Ungraded {
				p.Ungraded = false
				p.Matches[c.ID] = view.Match(row.Match, row.Calibrated, row.Stale)
				p.Scores[c.ID] = row.Match
			}
			if len(row.Dealbreakers) > 0 {
				if p.Dealbreakers == nil {
					p.Dealbreakers = map[string][]string{}
				}
				p.Dealbreakers[c.ID] = row.Dealbreakers
			} else if !row.Ungraded {
				mc.Count++
			}
		}
		d.Collections = append(d.Collections, mc)
	}
	for _, p := range pins {
		if p.Approx {
			d.Approx++
		}
		switch {
		case p.Ungraded:
			d.Ungraded++
		case len(p.Dealbreakers) == len(p.Collections):
			d.Dealbreakers++
		}
		d.Pins = append(d.Pins, p)
	}
	d.Min, d.Max = d.RunFirst, d.RunLast
	if from != "" {
		d.Min = min(d.Min, from)
	}
	d.Max = max(d.Max, to)
	return d, nil
}

func searchArea(c collection.Collection) ([][2]float64, bool) {
	center, ok := geo.ZipCentroid(strings.TrimSpace(c.Search.Location))
	if !ok {
		return nil, false
	}
	b := geo.BoundsAround(center, cmp.Or(c.Search.RadiusMiles, defaultAreaMiles))
	return [][2]float64{{b.South, b.West}, {b.North, b.East}}, true
}
