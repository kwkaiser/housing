package app

import (
	"cmp"
	"errors"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/app/view"
	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

type column struct {
	Key   string
	Label string
	Class string
	Off   bool
	Href  string
	Sort  string
	desc  bool
	cmp   func(a, b service.ListingRow) int
}

type listingsState struct {
	collection   string
	day          string
	sort         string
	desc         bool
	dealbreakers bool
}

func (s listingsState) href() string {
	q := url.Values{}
	if s.day != "" {
		q.Set("day", s.day)
	}
	if s.sort != "" {
		q.Set("sort", s.sort)
		q.Set("dir", map[bool]string{false: "asc", true: "desc"}[s.desc])
	}
	if s.dealbreakers {
		q.Set("dealbreakers", "1")
	}
	u := collectionURL(s.collection)
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

type collectionLink struct {
	ID     string
	Href   string
	Active bool
}

type rowView struct {
	service.ListingRow
	Href        string
	Grades      []gradeCell
	Change      string
	ChangeClass string
}

type gradeCell struct {
	report.Grade
	OK bool
}

type listingsData struct {
	service.CollectionDayView
	Title          string
	Columns        []column
	Rows           []rowView
	Multi          bool
	Sort           string
	Dir            string
	Dealbreakers   bool
	ToggleHref     string
	PrevHref       string
	NextHref       string
	First          string
	Last           string
	CollectionLink []collectionLink
	Run            *jobView
	EditHref       string
}

func (a *App) home(w http.ResponseWriter, r *http.Request) {
	cs, err := a.svc.Collections(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if len(cs) > 0 {
		http.Redirect(w, r, collectionURL(cs[0].ID), http.StatusFound)
		return
	}
	a.render(w, r, http.StatusOK, "home", nil)
}

func (a *App) collectionPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	st := listingsState{
		collection:   r.PathValue("collection"),
		day:          q.Get("day"),
		sort:         q.Get("sort"),
		dealbreakers: q.Get("dealbreakers") == "1",
	}
	v, err := a.svc.CollectionDay(r.Context(), st.collection, service.DayOptions{Day: st.day, Dealbreakers: st.dealbreakers})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d := listingsData{CollectionDayView: v, Title: v.Collection.ID, Multi: len(v.Collection.Profiles) > 1, Dealbreakers: st.dealbreakers, EditHref: editCollectionURL(v.Collection.ID)}
	cols := columns(v.Collection.Profiles, d.Multi)
	i := slices.IndexFunc(cols, func(c column) bool { return c.Key == st.sort })
	if i < 0 {
		st.sort = ""
	} else {
		st.desc = cols[i].desc
		switch q.Get("dir") {
		case "asc":
			st.desc = false
		case "desc":
			st.desc = true
		}
		sortRows(v.Rows, cols[i], st.desc)
		d.Sort, d.Dir = st.sort, map[bool]string{false: "asc", true: "desc"}[st.desc]
	}
	for i := range cols {
		c := &cols[i]
		next := st
		next.sort, next.desc = c.Key, c.desc
		if c.Key == st.sort {
			next.desc = !st.desc
			c.Sort = map[bool]string{false: "ascending", true: "descending"}[st.desc]
		}
		if c.Key == "rank" && st.sort == "rank" && st.desc {
			next.sort = ""
		}
		c.Href = next.href()
	}
	d.Columns = cols
	if d.Run, err = a.runStatus(r.Context(), v.Collection.ID); err != nil {
		a.fail(w, r, err)
		return
	}

	toggle := st
	toggle.dealbreakers = !st.dealbreakers
	d.ToggleHref = toggle.href()
	if v.Prev != "" {
		prev := st
		prev.day = v.Prev
		d.PrevHref = prev.href()
	}
	if v.Next != "" {
		next := st
		next.day = v.Next
		d.NextHref = next.href()
	}
	if len(v.Days) > 0 {
		d.First, d.Last = v.Days[0].Day, v.Days[len(v.Days)-1].Day
	}
	if len(v.Collections) > 1 {
		for _, c := range v.Collections {
			d.CollectionLink = append(d.CollectionLink, collectionLink{ID: c.ID, Href: collectionURL(c.ID), Active: c.ID == v.Collection.ID})
		}
	}
	for _, row := range v.Rows {
		rv := rowView{ListingRow: row, Href: listingURL(row.Listing, v.Collection.ID, v.Day)}
		for _, id := range v.Collection.Profiles {
			g, ok := row.Grades[id]
			rv.Grades = append(rv.Grades, gradeCell{Grade: g, OK: ok})
		}
		if diff := row.PriceChange(); diff != 0 {
			rv.Change, rv.ChangeClass = "↑ "+view.Money(diff, ""), "up"
			if diff < 0 {
				rv.Change, rv.ChangeClass = "↓ "+view.Money(-diff, ""), "down"
			}
		}
		d.Rows = append(d.Rows, rv)
	}
	a.render(w, r, http.StatusOK, "listings", d)
}

func columns(profiles []string, multi bool) []column {
	num := func(f func(service.ListingRow) float64) func(a, b service.ListingRow) int {
		return func(a, b service.ListingRow) int { return cmp.Compare(f(a), f(b)) }
	}
	text := func(f func(service.ListingRow) string) func(a, b service.ListingRow) int {
		return func(a, b service.ListingRow) int {
			return strings.Compare(strings.ToLower(f(a)), strings.ToLower(f(b)))
		}
	}
	cols := []column{
		{Key: "rank", Label: "#", Class: "num", cmp: num(func(r service.ListingRow) float64 { return float64(r.Rank) })},
		{Key: "match", Label: "Match", Class: "num score", desc: true, cmp: num(func(r service.ListingRow) float64 { return r.Match })},
		{Key: "score", Label: "Score", Class: "num", desc: true, cmp: num(func(r service.ListingRow) float64 { return r.Score })},
	}
	if multi {
		cols = append(cols, column{Key: "profile", Label: "Profile", cmp: text(func(r service.ListingRow) string { return r.Profile })})
		for _, id := range profiles {
			cols = append(cols, column{Key: "p:" + id, Label: id, Class: "num", desc: true, cmp: num(func(r service.ListingRow) float64 {
				if g, ok := r.Grades[id]; ok {
					return g.Match
				}
				return -1
			})})
		}
	}
	return append(cols,
		column{Key: "coverage", Label: "Coverage", Class: "num", desc: true, cmp: num(func(r service.ListingRow) float64 { return r.Coverage })},
		column{Key: "price", Label: "Price", Class: "num", cmp: num(func(r service.ListingRow) float64 { return float64(r.Listing.Price.Cents) })},
		column{Key: "beds", Label: "Beds", Class: "num", cmp: num(func(r service.ListingRow) float64 {
			if r.Listing.Beds == nil {
				return -1
			}
			return float64(*r.Listing.Beds)
		})},
		column{Key: "address", Label: "Address", cmp: text(func(r service.ListingRow) string { return report.AddressLine(r.Listing) })},
		column{Key: "sources", Label: "Sources", cmp: text(func(r service.ListingRow) string { return string(r.Listing.Source) })},
		column{Key: "dealbreakers", Label: "Dealbreakers", Class: "tags bad", Off: true, cmp: text(func(r service.ListingRow) string { return strings.Join(r.Dealbreakers, ",") })},
		column{Key: "missing", Label: "Missing", Class: "tags", Off: true, cmp: text(func(r service.ListingRow) string { return strings.Join(r.MissingEssentials, ",") })},
		column{Key: "avoids", Label: "Avoids", Class: "tags", Off: true, cmp: text(func(r service.ListingRow) string { return strings.Join(r.AvoidsHit, ",") })},
		column{Key: "summary", Label: "Summary", Class: "summary", Off: true, cmp: text(func(r service.ListingRow) string { return r.Summary })},
	)
}

func sortRows(rows []service.ListingRow, c column, desc bool) {
	slices.SortStableFunc(rows, func(a, b service.ListingRow) int {
		if desc {
			return c.cmp(b, a)
		}
		return c.cmp(a, b)
	})
}

type listingData struct {
	service.ListingView
	Title    string
	BackHref string
	Collages []string
	Offer    listing.OfferType
}

func (a *App) listingPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	o := service.ListingOptions{Collection: q.Get("collection"), Day: q.Get("day")}
	v, err := a.svc.ListingDetail(r.Context(), listing.Source(r.PathValue("source")), r.PathValue("id"), o)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d := listingData{ListingView: v, Title: report.AddressLine(v.Listing), Offer: v.Listing.Offer}
	if d.Title == "" {
		d.Title = string(v.Listing.Source) + " " + v.Listing.SourceID
	}
	if v.Collection != "" {
		d.BackHref = listingsState{collection: v.Collection, day: o.Day}.href()
	}
	for _, k := range v.Listing.Collages {
		d.Collages = append(d.Collages, "/media/"+k)
	}
	a.render(w, r, http.StatusOK, "listing", d)
}

func (a *App) media(w http.ResponseWriter, r *http.Request) {
	key := r.PathValue("key")
	p, err := a.svc.MediaPath(key)
	if err != nil {
		a.notFound(w, r)
		return
	}
	f, err := os.Open(p)
	if err != nil {
		a.notFound(w, r)
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.IsDir() {
		a.notFound(w, r)
		return
	}
	ctype := mime.TypeByExtension(path.Ext(key))
	if ctype == "" {
		buf := make([]byte, 512)
		n, _ := io.ReadFull(f, buf)
		ctype = http.DetectContentType(buf[:n])
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			a.fail(w, r, err)
			return
		}
	}
	if !strings.HasPrefix(ctype, "image/") {
		a.notFound(w, r)
		return
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, "", info.ModTime(), f)
}

func (a *App) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, collection.ErrNotFound) || errors.Is(err, store.ErrListingNotFound) || errors.Is(err, store.ErrJobNotFound) || errors.Is(err, profile.ErrNotFound) || errors.Is(err, store.ErrAPIKeyNotFound) {
		a.notFound(w, r)
		return
	}
	a.log.Error("request", "path", r.URL.Path, "err", err)
	a.renderError(w, r, http.StatusInternalServerError)
}

func collectionURL(id string) string {
	return "/c/" + url.PathEscape(id)
}

func listingURL(l listing.Listing, collectionID, day string) string {
	u := "/listing/" + url.PathEscape(string(l.Source)) + "/" + url.PathEscape(l.SourceID)
	q := url.Values{}
	if collectionID != "" {
		q.Set("collection", collectionID)
	}
	if day != "" {
		q.Set("day", day)
	}
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}
