package app

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

const maxFormBytes = 64 << 10

type collectionRow struct {
	collection.Collection
	Href          string
	EditHref      string
	SourceList    string
	ScheduleLabel string
	Budget        string
	DefaultBudget bool
	Run           *jobView
}

type collectionsData struct {
	Title       string
	Collections []collectionRow
}

type choice struct {
	Value   string
	Label   string
	Checked bool
}

type profileChoice struct {
	ID      string
	Name    string
	Checked bool
	Order   string
}

type collectionForm struct {
	Title         string
	Create        bool
	Action        string
	ID            string
	Modes         []choice
	Sources       []choice
	Profiles      []profileChoice
	Avoids        []profile.Profile
	Location      string
	RadiusMiles   string
	MinPrice      string
	MaxPrice      string
	MinBeds       string
	MaxBeds       string
	MaxAgeDays    string
	Amenities     []choice
	Limit         string
	Model         string
	MaxRunCostUSD string
	Schedule      string
	DefaultModel  string
	DefaultBudget string
	Errors        map[string]string
	General       []string
	selected      []string
	orders        map[string]string
}

func (a *App) collectionsPage(w http.ResponseWriter, r *http.Request) {
	cs, err := a.svc.Collections(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d := collectionsData{Title: "Collections"}
	for _, c := range cs {
		row := collectionRow{
			Collection:    c,
			Href:          collectionURL(c.ID),
			EditHref:      editCollectionURL(c.ID),
			SourceList:    joinSources(c.Sources),
			ScheduleLabel: "manual",
			Budget:        dollars(cmp.Or(c.MaxRunCostUSD, service.DefaultCollectionBudgetUSD)),
			DefaultBudget: c.MaxRunCostUSD == 0,
		}
		if c.Schedule != "" {
			row.ScheduleLabel = "daily at " + c.Schedule
		}
		if row.Run, err = a.runStatus(r.Context(), c.ID); err != nil {
			a.fail(w, r, err)
			return
		}
		d.Collections = append(d.Collections, row)
	}
	a.render(w, r, http.StatusOK, "collections", d)
}

func (a *App) newCollectionPage(w http.ResponseWriter, r *http.Request) {
	c := collection.Collection{Mode: profile.ModeRent, Sources: []listing.Source{listing.SourceZillow}, Search: profile.Search{Limit: 50}}
	a.renderCollectionForm(w, r, http.StatusOK, a.formFor(c, true))
}

func (a *App) editCollectionPage(w http.ResponseWriter, r *http.Request) {
	c, err := a.svc.Collection(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.renderCollectionForm(w, r, http.StatusOK, a.formFor(c, false))
}

func (a *App) createCollection(w http.ResponseWriter, r *http.Request) {
	a.saveCollection(w, r, true)
}

func (a *App) updateCollection(w http.ResponseWriter, r *http.Request) {
	if _, err := a.svc.Collection(r.Context(), r.PathValue("id")); err != nil {
		a.fail(w, r, err)
		return
	}
	a.saveCollection(w, r, false)
}

func (a *App) saveCollection(w http.ResponseWriter, r *http.Request, create bool) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		a.renderMessage(w, r, http.StatusBadRequest, "Bad request", "The form could not be read.")
		return
	}
	f := r.PostForm
	c, fields := parseCollection(f)
	if create {
		c.ID = strings.TrimSpace(f.Get("id"))
	} else {
		c.ID = r.PathValue("id")
	}
	form := a.formFor(c, create)
	form.Location, form.RadiusMiles, form.Limit = f.Get("location"), f.Get("radius_miles"), f.Get("limit")
	form.MinPrice, form.MaxPrice, form.MinBeds, form.MaxBeds = f.Get("min_price"), f.Get("max_price"), f.Get("min_beds"), f.Get("max_beds")
	form.MaxAgeDays, form.MaxRunCostUSD, form.Schedule, form.Model = f.Get("max_age_days"), f.Get("max_run_cost_usd"), f.Get("schedule"), f.Get("model")
	form.orders = map[string]string{}
	for k, v := range f {
		if id, ok := strings.CutPrefix(k, "order_"); ok && len(v) > 0 {
			form.orders[id] = v[0]
		}
	}

	var err error
	if len(fields) == 0 {
		save := a.svc.SaveCollection
		if create {
			save = a.svc.CreateCollection
		}
		if err = save(r.Context(), c); err == nil {
			http.Redirect(w, r, "/collections", http.StatusSeeOther)
			return
		}
		var fe profile.FieldError
		if !errors.As(err, &fe) {
			a.fail(w, r, err)
			return
		}
	} else {
		err = c.Validate()
	}
	collectErrors(err, fields, &form.General)
	form.Errors = fields
	a.renderCollectionForm(w, r, http.StatusUnprocessableEntity, form)
}

func (a *App) renderCollectionForm(w http.ResponseWriter, r *http.Request, status int, form collectionForm) {
	ps, err := a.svc.Profiles(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var checked, rest []profileChoice
	for _, p := range ps {
		if p.IsAvoid() {
			form.Avoids = append(form.Avoids, p)
			continue
		}
		pc := profileChoice{ID: p.ID, Name: p.Name, Order: form.orders[p.ID], Checked: slices.Contains(form.selected, p.ID)}
		if pc.Checked {
			checked = append(checked, pc)
		} else {
			rest = append(rest, pc)
		}
	}
	slices.SortStableFunc(checked, func(x, y profileChoice) int {
		return cmp.Compare(slices.Index(form.selected, x.ID), slices.Index(form.selected, y.ID))
	})
	form.Profiles = append(checked, rest...)
	a.render(w, r, status, "collection_form", form)
}

func (a *App) formFor(c collection.Collection, create bool) collectionForm {
	form := collectionForm{
		Title:         "Edit " + c.ID,
		Create:        create,
		Action:        "/collections/" + url.PathEscape(c.ID),
		ID:            c.ID,
		Location:      c.Search.Location,
		RadiusMiles:   formatFloat(c.Search.RadiusMiles),
		MinPrice:      formatIntPtr(c.Search.MinPrice),
		MaxPrice:      formatIntPtr(c.Search.MaxPrice),
		MinBeds:       formatIntPtr(c.Search.MinBeds),
		MaxBeds:       formatIntPtr(c.Search.MaxBeds),
		MaxAgeDays:    formatInt(c.Search.MaxAgeDays),
		Limit:         formatInt(c.Search.Limit),
		Model:         c.Model,
		MaxRunCostUSD: formatFloat(c.MaxRunCostUSD),
		Schedule:      c.Schedule,
		DefaultModel:  profile.DefaultAssessModel,
		DefaultBudget: strconv.FormatFloat(service.DefaultCollectionBudgetUSD, 'f', 2, 64),
		selected:      c.Profiles,
		orders:        map[string]string{},
	}
	if create {
		form.Title, form.Action = "New collection", "/collections"
	}
	for _, m := range []profile.Mode{profile.ModeRent, profile.ModeBuy} {
		form.Modes = append(form.Modes, choice{Value: string(m), Label: string(m), Checked: m == c.Mode})
	}
	for _, s := range service.SupportedSources() {
		form.Sources = append(form.Sources, choice{Value: string(s), Label: string(s), Checked: slices.Contains(c.Sources, s)})
	}
	for _, am := range listing.Amenities {
		form.Amenities = append(form.Amenities, choice{Value: string(am), Label: strings.ReplaceAll(string(am), "_", " "), Checked: slices.Contains(c.Search.Amenities, am)})
	}
	for i, id := range c.Profiles {
		form.orders[id] = strconv.Itoa(i + 1)
	}
	return form
}

func parseCollection(f url.Values) (collection.Collection, map[string]string) {
	errs := map[string]string{}
	c := collection.Collection{
		Mode:     profile.Mode(f.Get("mode")),
		Model:    strings.TrimSpace(f.Get("model")),
		Schedule: strings.TrimSpace(f.Get("schedule")),
	}
	for _, s := range unique(f["sources"]) {
		c.Sources = append(c.Sources, listing.Source(s))
	}
	c.Profiles = orderedProfiles(f, errs)

	s := profile.Search{Location: strings.TrimSpace(f.Get("location"))}
	for _, am := range unique(f["amenities"]) {
		s.Amenities = append(s.Amenities, listing.Amenity(am))
	}
	s.RadiusMiles = parseFloat(f, "radius_miles", "radius", errs)
	s.MinPrice = parseIntPtr(f, "min_price", "min price", errs)
	s.MaxPrice = parseIntPtr(f, "max_price", "max price", errs)
	s.MinBeds = parseIntPtr(f, "min_beds", "min beds", errs)
	s.MaxBeds = parseIntPtr(f, "max_beds", "max beds", errs)
	if n := parseIntPtr(f, "max_age_days", "max listing age", errs); n != nil {
		s.MaxAgeDays = *n
	}
	if n := parseIntPtr(f, "limit", "result limit", errs); n != nil {
		s.Limit = *n
	}
	c.Search = s
	c.MaxRunCostUSD = parseFloat(f, "max_run_cost_usd", "max run cost", errs)
	return c, errs
}

func orderedProfiles(f url.Values, errs map[string]string) []string {
	type pick struct {
		id    string
		order int
		set   bool
	}
	var picks []pick
	for _, id := range unique(f["profiles"]) {
		p := pick{id: id}
		if v := strings.TrimSpace(f.Get("order_" + id)); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil {
				errs["profiles"] = fmt.Sprintf("order for %s must be a whole number", id)
			} else {
				p.order, p.set = n, true
			}
		}
		picks = append(picks, p)
	}
	slices.SortStableFunc(picks, func(x, y pick) int {
		switch {
		case x.set && y.set:
			return cmp.Compare(x.order, y.order)
		case x.set:
			return -1
		case y.set:
			return 1
		}
		return 0
	})
	out := make([]string, len(picks))
	for i, p := range picks {
		out[i] = p.id
	}
	return out
}

func parseIntPtr(f url.Values, name, label string, errs map[string]string) *int {
	v := strings.TrimSpace(f.Get(name))
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		errs[name] = label + " must be a whole number"
		return nil
	}
	return &n
}

func parseFloat(f url.Values, name, label string, errs map[string]string) float64 {
	v := strings.TrimSpace(f.Get(name))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseFloat(v, 64)
	if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
		errs[name] = label + " must be a number"
		return 0
	}
	return n
}

func collectErrors(err error, fields map[string]string, general *[]string) {
	if err == nil {
		return
	}
	if fe, ok := err.(profile.FieldError); ok {
		if prev, dup := fields[fe.Field]; dup {
			fields[fe.Field] = prev + "; " + fe.Error()
		} else {
			fields[fe.Field] = fe.Error()
		}
		return
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range multi.Unwrap() {
			collectErrors(e, fields, general)
		}
		return
	}
	var fe profile.FieldError
	if errors.As(err, &fe) {
		collectErrors(errors.Unwrap(err), fields, general)
		return
	}
	*general = append(*general, err.Error())
}

func unique(vs []string) []string {
	var out []string
	for _, v := range vs {
		if v = strings.TrimSpace(v); v != "" && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}

func joinSources(ss []listing.Source) string {
	names := make([]string, len(ss))
	for i, s := range ss {
		names[i] = string(s)
	}
	return strings.Join(names, ", ")
}

func dollars(usd float64) string {
	return fmt.Sprintf("$%.2f", usd)
}

func formatFloat(f float64) string {
	if f == 0 {
		return ""
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func formatInt(n int) string {
	if n == 0 {
		return ""
	}
	return strconv.Itoa(n)
}

func formatIntPtr(n *int) string {
	if n == nil {
		return ""
	}
	return strconv.Itoa(*n)
}

func editCollectionURL(id string) string {
	return "/collections/" + url.PathEscape(id) + "/edit"
}
