package app

import (
	"cmp"
	"net/http"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/app/view"
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

type mapData struct {
	Collections []mapCollection
	Pins        []mapPin
	Approx      int
	Unplaced    int
	Filtered    int
	From        string
	To          string
	Default     bool
	First       string
	Last        string
	Min         string
	Max         string
}

func validDay(s string) string {
	if _, err := time.Parse(time.DateOnly, s); err != nil {
		return ""
	}
	return s
}

func (a *App) mapPage(w http.ResponseWriter, r *http.Request) {
	cs, err := a.svc.Collections(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	q := r.URL.Query()
	d := mapData{From: validDay(q.Get("from")), To: validDay(q.Get("to"))}
	if !q.Has("from") && !q.Has("to") {
		d.From, d.Default = a.now().AddDate(0, -1, 0).Format(time.DateOnly), true
	}
	if d.From != "" && d.To != "" && d.From > d.To {
		d.From, d.To = d.To, d.From
	}
	var pins []mapPin
	seen := map[string]int{}
	for i, c := range cs {
		v, err := a.svc.CollectionDay(r.Context(), c.ID, service.DayOptions{})
		if err != nil {
			a.fail(w, r, err)
			return
		}
		d.Collections = append(d.Collections, mapCollection{ID: c.ID, Day: v.Day, Color: mapColors[i%len(mapColors)]})
		for _, row := range v.Rows {
			key := string(row.Listing.Source) + "/" + row.Listing.SourceID
			if j, ok := seen[key]; ok {
				if j >= 0 {
					pins[j].Collections = append(pins[j].Collections, c.ID)
					pins[j].Matches[c.ID] = view.Match(row.Match, row.Calibrated, row.Stale)
					pins[j].Scores[c.ID] = row.Match
					pins[j].FirstSeen = min(pins[j].FirstSeen, row.FirstSeen)
				}
				continue
			}
			loc, approx, ok := geo.Locate(row.Listing)
			if !ok {
				seen[key] = -1
				d.Unplaced++
				continue
			}
			seen[key] = len(pins)
			p := mapPin{
				Lat: loc.Lat, Lng: loc.Lng, Approx: approx,
				Title:       cmp.Or(report.AddressLine(row.Listing), row.Listing.SourceID),
				Href:        listingURL(row.Listing, c.ID, v.Day),
				Price:       view.Money(row.Listing.Price.Cents, row.Listing.Offer),
				Beds:        view.Beds(row.Listing.Beds),
				Matches:     map[string]string{c.ID: view.Match(row.Match, row.Calibrated, row.Stale)},
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
	index := map[string]int{}
	for i, c := range d.Collections {
		index[c.ID] = i
	}
	for _, p := range pins {
		if d.First == "" || p.FirstSeen < d.First {
			d.First = p.FirstSeen
		}
		d.Last = max(d.Last, p.FirstSeen)
		if d.From != "" && p.FirstSeen < d.From || d.To != "" && p.FirstSeen > d.To {
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
	if d.From != "" {
		d.Min = min(d.Min, d.From)
	}
	d.Max = max(d.Max, d.To)
	a.render(w, r, http.StatusOK, "map", d)
}
