package service

import (
	"context"
	"errors"
	"maps"
	"path"
	"slices"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/dedupe"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

type DayOptions struct {
	Day          string
	Dealbreakers bool
}

type ListingRow struct {
	report.Row
	FirstSeen          string
	New                bool
	PreviousPriceCents *int64
}

func (r ListingRow) PriceChange() int64 {
	if r.PreviousPriceCents == nil {
		return 0
	}
	return r.Listing.Price.Cents - *r.PreviousPriceCents
}

type CollectionDayView struct {
	Collection  collection.Collection
	Collections []collection.Collection
	Days        []store.DayCount
	Day         string
	Requested   string
	Prev        string
	Next        string
	Listings    int
	Graded      int
	Hidden      int
	Stale       bool
	Models      []string
	Profiles    []ProfileView
	Rows        []ListingRow
}

type ProfileView struct {
	ID         string
	Name       string
	Summary    string
	Wants      []CriterionView
	Avoids     []CriterionView
	References []ReferenceView
}

type CriterionView struct {
	ID          string
	Label       string
	Importance  profile.Importance
	Dealbreaker bool
}

type ReferenceView struct {
	Source   listing.Source
	SourceID string
	URL      string
	Address  string
	Collages []string
	Scores   map[string]float64
}

func (s *Service) CollectionDay(ctx context.Context, collectionID string, o DayOptions) (CollectionDayView, error) {
	db, err := s.catalog(ctx)
	if err != nil {
		return CollectionDayView{}, err
	}
	var v CollectionDayView
	if v.Collection, err = db.Collection(ctx, collectionID); err != nil {
		return v, err
	}
	if v.Collections, err = db.Collections(ctx); err != nil {
		return v, err
	}
	templates, err := s.templates(ctx, db, v.Collection.Profiles)
	if err != nil {
		return v, err
	}
	if v.Days, err = db.CollectionDays(ctx, collectionID); err != nil {
		return v, err
	}
	i := pickDay(v.Days, o.Day)
	if i < 0 {
		v.Profiles = profileViews(templates, modelsFor(v.Collection, nil))
		return v, nil
	}
	v.Day, v.Listings = v.Days[i].Day, v.Days[i].Listings
	if o.Day != "" && o.Day != v.Day {
		v.Requested = o.Day
	}
	if i > 0 {
		v.Prev = v.Days[i-1].Day
	}
	if i < len(v.Days)-1 {
		v.Next = v.Days[i+1].Day
	}

	listings, err := db.LoadDay(ctx, v.Day, collectionID)
	if err != nil {
		return v, err
	}
	history, err := db.History(ctx, collectionID)
	if err != nil {
		return v, err
	}
	r := report.Build(reportTemplates(templates), listings, report.Options{
		Offer: v.Collection.Mode.Offer(), Model: v.Collection.Model, IncludeStale: true, Dealbreakers: true,
	})
	v.Graded = len(r.Rows)
	v.Models = modelsFor(v.Collection, r.Models)
	for _, row := range r.Rows {
		if len(row.Dealbreakers) > 0 && !o.Dealbreakers {
			v.Hidden++
			continue
		}
		lr := ListingRow{Row: row}
		lr.FirstSeen, lr.PreviousPriceCents = sightings(v.Day, history[store.Key(row.Listing)])
		lr.New = lr.FirstSeen == v.Day
		lr.Rank = len(v.Rows) + 1
		v.Stale = v.Stale || row.Stale
		for _, g := range row.Grades {
			v.Stale = v.Stale || g.Stale
		}
		v.Rows = append(v.Rows, lr)
	}
	v.Profiles = profileViews(templates, v.Models)
	return v, nil
}

func pickDay(days []store.DayCount, want string) int {
	if len(days) == 0 {
		return -1
	}
	if _, err := time.Parse(store.DayLayout, want); err != nil {
		return len(days) - 1
	}
	best := 0
	for i, d := range days {
		if d.Day <= want {
			best = i
		}
	}
	return best
}

func modelsFor(c collection.Collection, seen []string) []string {
	if len(seen) == 0 && c.Model != "" {
		return []string{c.Model}
	}
	return seen
}

func sightings(day string, seen []store.Sighting) (string, *int64) {
	first := day
	if len(seen) > 0 {
		first = min(seen[0].Day, day)
	}
	var previous *int64
	for _, s := range seen {
		if s.Day >= day {
			break
		}
		price := s.PriceCents
		previous = &price
	}
	return first, previous
}

func profileViews(templates []template, models []string) []ProfileView {
	out := make([]ProfileView, len(templates))
	for i, t := range templates {
		out[i] = profileView(t.Template, models)
	}
	return out
}

func profileView(t report.Template, models []string) ProfileView {
	p := t.Profile
	v := ProfileView{ID: p.ID, Name: p.Name, Summary: p.Summary}
	for _, c := range p.Want {
		v.Wants = append(v.Wants, CriterionView{ID: c.ID, Label: c.Label, Importance: c.Importance})
	}
	for _, c := range p.Avoid {
		v.Avoids = append(v.Avoids, CriterionView{ID: c.ID, Label: c.Label, Importance: c.Importance, Dealbreaker: c.Importance == profile.Essential})
	}
	for _, ref := range t.References {
		v.References = append(v.References, referenceView(p, ref, models))
	}
	return v
}

func referenceView(p profile.Profile, ref listing.Listing, models []string) ReferenceView {
	r := ReferenceView{Source: ref.Source, SourceID: ref.SourceID, URL: ref.URL, Address: report.AddressLine(ref), Collages: ref.Collages, Scores: map[string]float64{}}
	for _, m := range models {
		if s, ok := profile.ReferenceScore(p, []listing.Listing{ref}, m); ok {
			r.Scores[m] = s
		}
	}
	return r
}

type ProfileDetailView struct {
	Profile     profile.Profile
	Inherited   []InheritedCriterion
	References  []ReferenceView
	Collections []string
}

type InheritedCriterion struct {
	Profile     string
	ProfileName string
	profile.Criterion
}

func (s *Service) ProfileDetail(ctx context.Context, id string) (ProfileDetailView, error) {
	db, err := s.catalog(ctx)
	if err != nil {
		return ProfileDetailView{}, err
	}
	var v ProfileDetailView
	if v.Profile, err = db.Profile(ctx, id); err != nil {
		return v, err
	}
	all, err := db.Profiles(ctx)
	if err != nil {
		return v, err
	}
	effective := v.Profile
	if !v.Profile.IsAvoid() {
		for _, ap := range all {
			if !ap.IsAvoid() {
				continue
			}
			for _, c := range ap.Avoid {
				v.Inherited = append(v.Inherited, InheritedCriterion{Profile: ap.ID, ProfileName: ap.Name, Criterion: c})
			}
		}
		if effective, err = profile.Effective(v.Profile, all); err != nil {
			return v, err
		}
	}
	refs, err := db.ReferenceListings(ctx, effective)
	if err != nil {
		return v, err
	}
	for _, ref := range own(effective, refs) {
		v.References = append(v.References, referenceView(effective, ref, slices.Sorted(maps.Keys(ref.Assessments[id]))))
	}
	cs, err := db.Collections(ctx)
	if err != nil {
		return v, err
	}
	for _, c := range cs {
		if slices.Contains(c.Profiles, id) {
			v.Collections = append(v.Collections, c.ID)
		}
	}
	return v, nil
}

type ListingView struct {
	Listing    listing.Listing
	Collection string
	Day        string
	AlsoListed []listing.Listing
	History    []store.Sighting
	Grades     []GradeView
}

type GradeView struct {
	Profile ProfileView
	report.Grade
	Models []ModelView
}

type ModelView struct {
	Model string
	Stale bool
	listing.Assessment
	Wants  []ResultView
	Avoids []ResultView
}

type ResultView struct {
	CriterionView
	listing.CriterionResult
}

type ListingOptions struct {
	Collection string
	Day        string
}

func (s *Service) ListingDetail(ctx context.Context, source listing.Source, sourceID string, o ListingOptions) (ListingView, error) {
	db, err := s.catalog(ctx)
	if err != nil {
		return ListingView{}, err
	}
	v := ListingView{Collection: o.Collection}
	if v.Listing, err = db.CurrentListing(ctx, source, sourceID); err != nil {
		return v, err
	}
	var c collection.Collection
	if o.Collection != "" {
		if c, err = db.Collection(ctx, o.Collection); err != nil {
			return v, err
		}
		if err := s.listingOnDay(ctx, db, &v, o.Day); err != nil {
			return v, err
		}
	}
	if v.History, err = db.ListingHistory(ctx, source, sourceID); err != nil {
		return v, err
	}

	ids := c.Profiles
	if o.Collection == "" {
		if ids, err = assessedProfiles(ctx, db, v.Listing); err != nil {
			return v, err
		}
	}
	if len(ids) == 0 {
		return v, nil
	}
	templates, err := s.templates(ctx, db, ids)
	if err != nil {
		return v, err
	}
	r := report.Build(reportTemplates(templates), []listing.Listing{v.Listing}, report.Options{Model: c.Model, IncludeStale: true, Dealbreakers: true})
	if len(r.Rows) == 0 {
		return v, nil
	}
	for _, t := range templates {
		g, ok := r.Rows[0].Grades[t.Profile.ID]
		if !ok {
			continue
		}
		v.Grades = append(v.Grades, gradeView(t.Template, g, v.Listing, r.Models))
	}
	return v, nil
}

func (s *Service) listingOnDay(ctx context.Context, db *store.Store, v *ListingView, day string) error {
	days, err := db.CollectionDays(ctx, v.Collection)
	if err != nil {
		return err
	}
	i := pickDay(days, day)
	if i < 0 {
		return nil
	}
	v.Day = days[i].Day
	listings, err := db.LoadDay(ctx, v.Day, v.Collection)
	if err != nil {
		return err
	}
	k := store.Key(v.Listing)
	for _, g := range dedupe.Groups(listings) {
		members := g.Members()
		j := slices.IndexFunc(members, func(l listing.Listing) bool { return store.Key(l) == k })
		if j < 0 {
			continue
		}
		v.Listing = members[j]
		v.AlsoListed = slices.Delete(members, j, j+1)
		return nil
	}
	return nil
}

func assessedProfiles(ctx context.Context, db *store.Store, l listing.Listing) ([]string, error) {
	ps, err := db.Profiles(ctx)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, p := range ps {
		if _, ok := l.Assessments[p.ID]; ok && !p.IsAvoid() {
			ids = append(ids, p.ID)
		}
	}
	return ids, nil
}

func gradeView(t report.Template, g report.Grade, l listing.Listing, models []string) GradeView {
	pv := profileView(t, models)
	v := GradeView{Profile: pv, Grade: g}
	hash, input := t.Profile.Hash(), profile.InputHash(l)
	for _, m := range slices.Sorted(maps.Keys(g.ByModel)) {
		a := g.ByModel[m]
		v.Models = append(v.Models, ModelView{
			Model:      m,
			Stale:      a.ProfileHash != hash || a.InputHash != input,
			Assessment: a,
			Wants:      results(pv.Wants, a.Want),
			Avoids:     results(pv.Avoids, a.Avoid),
		})
	}
	return v
}

func results(criteria []CriterionView, rs []listing.CriterionResult) []ResultView {
	byID := make(map[string]CriterionView, len(criteria))
	for _, c := range criteria {
		byID[c.ID] = c
	}
	out := make([]ResultView, len(rs))
	for i, r := range rs {
		c, ok := byID[r.ID]
		if !ok {
			c = CriterionView{ID: r.ID, Label: r.ID}
		}
		out[i] = ResultView{CriterionView: c, CriterionResult: r}
	}
	return out
}

var ErrInvalidMediaKey = errors.New("invalid media key")

func (s *Service) MediaPath(key string) (string, error) {
	if !ValidMediaKey(key) {
		return "", ErrInvalidMediaKey
	}
	return s.images().Path(key), nil
}

func ValidMediaKey(key string) bool {
	if !strings.HasPrefix(key, "photos/") && !strings.HasPrefix(key, "collages/") {
		return false
	}
	if strings.ContainsAny(key, "\\\x00") || path.Clean(key) != key {
		return false
	}
	return !slices.Contains(strings.Split(key, "/"), "..")
}
