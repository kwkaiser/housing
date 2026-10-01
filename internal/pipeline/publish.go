package pipeline

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
	"git.kwkaiser.io/kwkaiser/housing/internal/site"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

func (e *Env) Publish(ctx context.Context, dir string) (site.Current, error) {
	collections, err := e.collections().List()
	if err != nil {
		return site.Current{}, err
	}
	if len(collections) == 0 {
		return site.Current{}, fmt.Errorf("no collections in %s; create one with `housing collection set`", e.CollectionsDir)
	}
	db, err := e.openStore(ctx)
	if err != nil {
		return site.Current{}, err
	}
	defer db.Close()

	var out site.Site
	for _, c := range collections {
		if err := e.publishCollection(ctx, db, c, &out); err != nil {
			return site.Current{}, fmt.Errorf("collection %s: %w", c.ID, err)
		}
	}
	cur, err := site.Publish(ctx, dir, out, time.Now())
	if err != nil {
		return site.Current{}, err
	}
	e.printf("publish: %d collections, %d days, %d rows to %s/%s\n", len(out.Collections), len(out.Days), len(out.Rows), dir, cur.DB)
	return cur, nil
}

func (e *Env) publishCollection(ctx context.Context, db *store.Store, c collection.Collection, out *site.Site) error {
	templates, err := e.templates(ctx, c.Profiles)
	if err != nil {
		return err
	}
	days, err := db.CollectionDays(ctx, c.ID)
	if err != nil {
		return err
	}
	history, err := db.History(ctx, c.ID)
	if err != nil {
		return err
	}

	models := map[string]bool{}
	for _, d := range days {
		listings, err := db.LoadDay(ctx, d.Day, c.ID)
		if err != nil {
			return err
		}
		r := report.Build(templates, listings, report.Options{Offer: c.Mode.Offer(), Model: c.Model, IncludeStale: true, Dealbreakers: true})
		for _, m := range r.Models {
			models[m] = true
		}
		for i, row := range r.Rows {
			out.Rows = append(out.Rows, siteRow(c.ID, d.Day, i, row, history[store.Key(row.Listing)]))
		}
		out.Days = append(out.Days, site.Day{Collection: c.ID, Day: d.Day, Listings: d.Listings, Graded: len(r.Rows)})
	}

	modelList := slices.Sorted(maps.Keys(models))
	out.Collections = append(out.Collections, site.Collection{
		ID: c.ID, Mode: string(c.Mode), Location: c.Search.Location, Profiles: c.Profiles, Models: modelList,
	})
	for _, t := range templates {
		out.Profiles = append(out.Profiles, siteProfile(c.ID, t, modelList))
	}
	return nil
}

func siteRow(collectionID, day string, order int, row report.Row, seen []store.Sighting) site.Row {
	l := row.Listing
	r := site.Row{
		Collection:   collectionID,
		Day:          day,
		Order:        order,
		Source:       string(l.Source),
		SourceID:     l.SourceID,
		URL:          l.URL,
		Address:      report.AddressLine(l),
		Offer:        string(l.Offer),
		PriceCents:   l.Price.Cents,
		FirstSeen:    day,
		Beds:         l.Beds,
		Profile:      row.Profile,
		Match:        row.Match,
		Calibrated:   row.Calibrated,
		Score:        row.Score,
		Coverage:     row.Coverage,
		Vibe:         row.Vibe,
		Stale:        row.Stale,
		Missing:      row.MissingEssentials,
		Dealbreakers: row.Dealbreakers,
		Avoids:       row.AvoidsHit,
		Summary:      row.Summary,
		Grades:       map[string]site.Grade{},
	}
	for id, g := range row.Grades {
		r.Grades[id] = site.Grade{Match: g.Match, Calibrated: g.Calibrated, Stale: g.Stale}
	}
	for _, d := range row.AlsoListed {
		r.AlsoListed = append(r.AlsoListed, site.Link{Source: string(d.Source), URL: d.URL, PriceCents: d.Price.Cents})
	}
	if len(seen) > 0 {
		r.FirstSeen = min(seen[0].Day, day)
	}
	for _, s := range seen {
		if s.Day >= day {
			break
		}
		price := s.PriceCents
		r.PreviousPriceCents = &price
	}
	return r
}

func siteProfile(collectionID string, t report.Template, models []string) site.Profile {
	p := t.Profile
	sp := site.Profile{Collection: collectionID, ID: p.ID, Name: p.Name, Summary: p.Summary}
	for _, c := range p.Want {
		sp.Wants = append(sp.Wants, site.Want{Label: c.Label, Importance: string(c.Importance)})
	}
	for _, c := range p.Avoid {
		sp.Avoids = append(sp.Avoids, site.Avoid{Label: c.Label, Dealbreaker: c.Importance == profile.Essential})
	}
	for _, ref := range t.References {
		r := site.Reference{URL: ref.URL, Address: report.AddressLine(ref), Scores: map[string]float64{}}
		for _, m := range models {
			if s, ok := profile.ReferenceScore(p, []listing.Listing{ref}, m); ok {
				r.Scores[m] = s
			}
		}
		sp.References = append(sp.References, r)
	}
	return sp
}
