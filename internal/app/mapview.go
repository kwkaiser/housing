package app

import (
	"cmp"
	"context"
	"net/http"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/app/view"
	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

var mapColors = []string{"#2563eb", "#dc2626", "#16a34a", "#9333ea", "#ea580c", "#0891b2", "#db2777", "#65a30d"}

type mapCollection struct {
	ID    string
	Day   string
	Color string
	Count int
}

type mapPin struct {
	Lat         float64            `json:"lat"`
	Lng         float64            `json:"lng"`
	Approx      bool               `json:"approx,omitempty"`
	Title       string             `json:"title"`
	Href        string             `json:"href"`
	Price       string             `json:"price"`
	Beds        string             `json:"beds"`
	Matches     map[string]string  `json:"matches"`
	Scores      map[string]float64 `json:"scores"`
	FirstSeen   string             `json:"firstSeen"`
	Preview     string             `json:"preview,omitempty"`
	Collections []string           `json:"collections"`
}

type rangeLink struct {
	Label  string
	Href   string
	Active bool
}

type mapData struct {
	Collections []mapCollection
	Pins        []mapPin
	Approx      int
	Unplaced    int
	Filtered    int
	From        string
	To          string
	Quick       []rangeLink
	First       string
	Last        string
	Min         string
	Max         string
	Extent      map[string][][2]float64
	Config      mapConfig
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
	pins, collections, unplaced, err := a.mapPins(r.Context(), cs)
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
	d := summarize(pins, collections, from, to)
	d.Unplaced = unplaced
	d.Quick = quickRanges(today, d.From, d.To)
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

func (a *App) mapPins(ctx context.Context, cs []collection.Collection) ([]mapPin, []mapCollection, int, error) {
	var (
		pins        []mapPin
		collections []mapCollection
		unplaced    int
		seen        = map[string]int{}
	)
	for i, c := range cs {
		v, err := a.svc.CollectionDay(ctx, c.ID, service.DayOptions{})
		if err != nil {
			return nil, nil, 0, err
		}
		collections = append(collections, mapCollection{ID: c.ID, Day: v.Day, Color: mapColors[i%len(mapColors)]})
		for _, row := range v.Rows {
			key := string(row.Listing.Source) + "/" + row.Listing.SourceID
			match := view.Match(row.Match, row.Calibrated, row.Stale)
			if j, ok := seen[key]; ok {
				if j >= 0 {
					p := &pins[j]
					p.Collections = append(p.Collections, c.ID)
					p.Matches[c.ID], p.Scores[c.ID] = match, row.Match
					p.FirstSeen = min(p.FirstSeen, row.FirstSeen)
				}
				continue
			}
			loc, approx, ok := geo.Locate(row.Listing)
			if !ok {
				seen[key] = -1
				unplaced++
				continue
			}
			seen[key] = len(pins)
			p := mapPin{
				Lat: loc.Lat, Lng: loc.Lng, Approx: approx,
				Title:       cmp.Or(report.AddressLine(row.Listing), row.Listing.SourceID),
				Href:        listingURL(row.Listing, c.ID, v.Day),
				Price:       view.Money(row.Listing.Price.Cents, row.Listing.Offer),
				Beds:        view.Beds(row.Listing.Beds),
				Matches:     map[string]string{c.ID: match},
				Scores:      map[string]float64{c.ID: row.Match},
				FirstSeen:   row.FirstSeen,
				Collections: []string{c.ID},
			}
			if len(row.Listing.Collages) > 0 {
				p.Preview = "/media/" + row.Listing.Collages[0]
			}
			pins = append(pins, p)
		}
	}
	return pins, collections, unplaced, nil
}

func summarize(pins []mapPin, collections []mapCollection, from, to string) mapData {
	if from != "" && to != "" && from > to {
		from, to = to, from
	}
	d := mapData{Collections: collections, Pins: []mapPin{}, Extent: map[string][][2]float64{}, From: from, To: to}
	index := map[string]int{}
	for i, c := range collections {
		index[c.ID] = i
	}
	for _, p := range pins {
		for _, id := range p.Collections {
			e, ok := d.Extent[id]
			if !ok {
				e = [][2]float64{{p.Lat, p.Lng}, {p.Lat, p.Lng}}
			}
			d.Extent[id] = [][2]float64{{min(e[0][0], p.Lat), min(e[0][1], p.Lng)}, {max(e[1][0], p.Lat), max(e[1][1], p.Lng)}}
		}
		if d.First == "" || p.FirstSeen < d.First {
			d.First = p.FirstSeen
		}
		d.Last = max(d.Last, p.FirstSeen)
		if from != "" && p.FirstSeen < from || to != "" && p.FirstSeen > to {
			d.Filtered++
			continue
		}
		if p.Approx {
			d.Approx++
		}
		for _, id := range p.Collections {
			d.Collections[index[id]].Count++
		}
		d.Pins = append(d.Pins, p)
	}
	d.Min, d.Max = d.First, d.Last
	if from != "" {
		d.Min = min(d.Min, from)
	}
	d.Max = max(d.Max, to)
	return d
}
