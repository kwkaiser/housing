package app

import (
	"cmp"
	"errors"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/app/view"
	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

type profileRow struct {
	ID          string
	Href        string
	Kind        profile.Kind
	Name        string
	Wants       int
	Avoids      int
	References  int
	Collections []collectionLink
	Drafted     string
}

type profilesData struct {
	Title    string
	Profiles []profileRow
}

type referenceRow struct {
	service.ReferenceView
	Href     string
	Collages []string
}

type inheritedGroup struct {
	ID       string
	Name     string
	Href     string
	Criteria []profile.Criterion
}

type profileData struct {
	Title        string
	Profile      profile.Profile
	Kind         profile.Kind
	EditHref     string
	DraftAction  string
	Drafted      string
	Collections  []collectionLink
	References   []referenceRow
	Inherited    []inheritedGroup
	Job          *jobView
	DraftNotes   string
	DefaultModel string
}

type profileForm struct {
	Title         string
	URL           string
	ID            string
	Kinds         []choice
	Name          string
	Notes         string
	Model         string
	MaxChargeUSD  string
	Draft         bool
	Force         bool
	DefaultModel  string
	DefaultCharge string
	Errors        map[string]string
	General       []string
}

type criterionRow struct {
	ID              string
	Label           string
	Importance      string
	Evidence        profile.Evidence
	LookFor         string
	Remove          bool
	Options         []option
	LabelError      string
	ImportanceError string
}

type profileEditForm struct {
	Title    string
	Action   string
	BackHref string
	ID       string
	Kind     profile.Kind
	Name     string
	Summary  string
	Notes    string
	Want     []criterionRow
	Avoid    []criterionRow
	Errors   map[string]string
	General  []string
}

func (a *App) profilesPage(w http.ResponseWriter, r *http.Request) {
	ps, err := a.svc.Profiles(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	cs, err := a.svc.Collections(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d := profilesData{Title: "Profiles"}
	for _, p := range ps {
		row := profileRow{
			ID: p.ID, Href: profileURL(p.ID), Kind: kindOf(p), Name: p.Name,
			Wants: len(p.Want), Avoids: len(p.Avoid), References: len(p.References), Drafted: drafted(p.Drafted),
		}
		for _, c := range cs {
			if slices.Contains(c.Profiles, p.ID) {
				row.Collections = append(row.Collections, collectionLink{ID: c.ID, Href: collectionURL(c.ID)})
			}
		}
		d.Profiles = append(d.Profiles, row)
	}
	a.render(w, r, http.StatusOK, "profiles", d)
}

func (a *App) profilePage(w http.ResponseWriter, r *http.Request) {
	v, err := a.svc.ProfileDetail(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	p := v.Profile
	d := profileData{
		Title:        cmp.Or(p.Name, p.ID),
		Profile:      p,
		Kind:         kindOf(p),
		EditHref:     profileURL(p.ID) + "/edit",
		DraftAction:  profileURL(p.ID) + "/draft",
		Drafted:      drafted(p.Drafted),
		DraftNotes:   strings.Join(p.Notes, "\n"),
		DefaultModel: profile.DefaultDraftModel,
	}
	for _, id := range v.Collections {
		d.Collections = append(d.Collections, collectionLink{ID: id, Href: collectionURL(id)})
	}
	for _, ref := range v.References {
		row := referenceRow{ReferenceView: ref, Href: safeURL(ref.URL)}
		for _, k := range ref.Collages {
			row.Collages = append(row.Collages, "/media/"+k)
		}
		d.References = append(d.References, row)
	}
	for _, c := range v.Inherited {
		if n := len(d.Inherited); n == 0 || d.Inherited[n-1].ID != c.Profile {
			d.Inherited = append(d.Inherited, inheritedGroup{ID: c.Profile, Name: c.ProfileName, Href: profileURL(c.Profile)})
		}
		g := &d.Inherited[len(d.Inherited)-1]
		g.Criteria = append(g.Criteria, c.Criterion)
	}
	js, err := a.jobs.Jobs(r.Context(), jobs.Filter{ProfileID: p.ID, Limit: 1})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if len(js) > 0 {
		jv := a.jobView(js[0], a.now())
		d.Job = &jv
	}
	a.render(w, r, http.StatusOK, "profile", d)
}

func (a *App) newProfilePage(w http.ResponseWriter, r *http.Request) {
	a.render(w, r, http.StatusOK, "profile_form", newProfileForm(profile.KindWant))
}

func newProfileForm(kind profile.Kind) profileForm {
	f := profileForm{
		Title:         "New profile",
		Draft:         true,
		DefaultModel:  profile.DefaultDraftModel,
		DefaultCharge: strconv.FormatFloat(service.DefaultLookupMaxChargeUSD, 'f', 2, 64),
		Errors:        map[string]string{},
	}
	for _, k := range []profile.Kind{profile.KindWant, profile.KindAvoid} {
		f.Kinds = append(f.Kinds, choice{Value: string(k), Label: string(k), Checked: k == kind})
	}
	return f
}

func (a *App) createProfile(w http.ResponseWriter, r *http.Request) {
	if !a.parseForm(w, r) {
		return
	}
	f := r.PostForm
	form := newProfileForm(profile.Kind(f.Get("kind")))
	form.URL, form.ID, form.Name, form.Notes = f.Get("url"), f.Get("id"), f.Get("name"), f.Get("notes")
	form.Model, form.MaxChargeUSD = f.Get("model"), f.Get("max_charge_usd")
	form.Draft, form.Force = f.Get("draft") != "", f.Get("force") != ""

	p := jobs.CreateProfileParams{
		URL:          strings.TrimSpace(form.URL),
		ID:           strings.TrimSpace(form.ID),
		ProfileKind:  profile.Kind(f.Get("kind")),
		Name:         strings.TrimSpace(form.Name),
		Notes:        lines(form.Notes),
		NoDraft:      !form.Draft,
		Force:        form.Force,
		MaxChargeUSD: service.DefaultLookupMaxChargeUSD,
	}
	if !p.NoDraft {
		p.Model = cmp.Or(strings.TrimSpace(form.Model), profile.DefaultDraftModel)
	}
	if v := strings.TrimSpace(form.MaxChargeUSD); v != "" {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil || n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) {
			form.Errors["max_charge_usd"] = "max Apify charge must be a positive amount"
		} else {
			p.MaxChargeUSD = n
		}
	}
	err := a.svc.ValidateNewProfile(r.Context(), service.CreateProfileOptions{
		URL: p.URL, ID: p.ID, Kind: p.ProfileKind, Name: p.Name, Notes: p.Notes, Model: p.Model,
		NoDraft: p.NoDraft, Force: p.Force, MaxChargeUSD: p.MaxChargeUSD,
	})
	var fe profile.FieldError
	if err != nil && !errors.As(err, &fe) {
		a.fail(w, r, err)
		return
	}
	collectErrors(err, form.Errors, &form.General)
	if len(form.Errors) > 0 || len(form.General) > 0 {
		a.render(w, r, http.StatusUnprocessableEntity, "profile_form", form)
		return
	}
	id, err := a.jobs.Enqueue(r.Context(), p)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, jobURL(id), http.StatusSeeOther)
}

func (a *App) editProfilePage(w http.ResponseWriter, r *http.Request) {
	p, err := a.svc.Profile(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	a.render(w, r, http.StatusOK, "profile_edit", editForm(p, nil))
}

func editForm(p profile.Profile, f url.Values) profileEditForm {
	form := profileEditForm{
		Title:    "Edit " + p.ID,
		Action:   profileURL(p.ID),
		BackHref: profileURL(p.ID),
		ID:       p.ID,
		Kind:     kindOf(p),
		Name:     p.Name,
		Summary:  p.Summary,
		Notes:    strings.Join(p.Notes, "\n"),
		Errors:   map[string]string{},
	}
	shown := map[string]bool{}
	if f != nil {
		form.Name, form.Summary, form.Notes = f.Get("name"), f.Get("summary"), f.Get("notes")
		for _, id := range f["criteria"] {
			shown[id] = true
		}
	}
	rows := func(cs []profile.Criterion) []criterionRow {
		var out []criterionRow
		for _, c := range cs {
			row := criterionRow{ID: c.ID, Label: c.Label, Importance: string(c.Importance), Evidence: c.Evidence, LookFor: c.LookFor}
			if shown[c.ID] {
				row.Label, row.Importance, row.Remove = f.Get("label_"+c.ID), f.Get("importance_"+c.ID), f.Get("remove_"+c.ID) != ""
			}
			for _, imp := range profile.Importances {
				row.Options = append(row.Options, option{Value: string(imp), Selected: string(imp) == row.Importance})
			}
			if !slices.Contains(profile.Importances, profile.Importance(row.Importance)) {
				row.Options = append(row.Options, option{Value: row.Importance, Selected: true})
			}
			out = append(out, row)
		}
		return out
	}
	form.Want, form.Avoid = rows(p.Want), rows(p.Avoid)
	return form
}

func (a *App) updateProfile(w http.ResponseWriter, r *http.Request) {
	p, err := a.svc.Profile(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !a.parseForm(w, r) {
		return
	}
	f := r.PostForm
	e := service.ProfileEdit{Name: f.Get("name"), Summary: textarea(f.Get("summary")), Notes: lines(f.Get("notes"))}
	for _, id := range unique(f["criteria"]) {
		e.Criteria = append(e.Criteria, service.CriterionEdit{
			ID:         id,
			Label:      f.Get("label_" + id),
			Importance: profile.Importance(f.Get("importance_" + id)),
			Remove:     f.Get("remove_"+id) != "",
		})
	}
	_, err = a.svc.UpdateProfile(r.Context(), p.ID, e)
	if err == nil {
		http.Redirect(w, r, profileURL(p.ID), http.StatusSeeOther)
		return
	}
	var fe profile.FieldError
	if !errors.As(err, &fe) {
		a.fail(w, r, err)
		return
	}
	form := editForm(p, f)
	collectErrors(err, form.Errors, &form.General)
	for _, rows := range [][]criterionRow{form.Want, form.Avoid} {
		for i := range rows {
			rows[i].LabelError = form.Errors["criteria."+rows[i].ID+".label"]
			rows[i].ImportanceError = form.Errors["criteria."+rows[i].ID+".importance"]
		}
	}
	if msg, ok := form.Errors["criteria"]; ok {
		form.General = append(form.General, msg)
	}
	a.render(w, r, http.StatusUnprocessableEntity, "profile_edit", form)
}

func (a *App) draftProfile(w http.ResponseWriter, r *http.Request) {
	p, err := a.svc.Profile(r.Context(), r.PathValue("id"))
	if err != nil {
		a.fail(w, r, err)
		return
	}
	if !a.parseForm(w, r) {
		return
	}
	if len(p.References) == 0 {
		a.renderMessage(w, r, http.StatusUnprocessableEntity, "Nothing to draft from", "Profile "+p.ID+" has no reference listings, so its criteria can't be re-drafted.")
		return
	}
	params := jobs.DraftProfileParams{ProfileID: p.ID, Model: cmp.Or(strings.TrimSpace(r.PostForm.Get("model")), profile.DefaultDraftModel)}
	if notes := lines(r.PostForm.Get("notes")); !slices.Equal(notes, p.Notes) {
		params.Notes = append([]string{}, notes...)
	}
	id, err := a.jobs.Enqueue(r.Context(), params)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, jobURL(id), http.StatusSeeOther)
}

func (a *App) parseForm(w http.ResponseWriter, r *http.Request) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		a.renderMessage(w, r, http.StatusBadRequest, "Bad request", "The form could not be read.")
		return false
	}
	return true
}

func kindOf(p profile.Profile) profile.Kind {
	return cmp.Or(p.Kind, profile.KindWant)
}

func drafted(d *profile.Drafted) string {
	if d == nil {
		return ""
	}
	s := view.ShortModel(d.Model)
	if !d.At.IsZero() {
		s += " · " + d.At.Local().Format("2006-01-02 15:04")
	}
	return s
}

func textarea(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func lines(s string) []string {
	var out []string
	for _, l := range strings.Split(textarea(s), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}
