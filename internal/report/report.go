package report

import (
	"cmp"
	"math"
	"slices"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

type Options struct {
	Model        string
	IncludeStale bool
	MinScore     float64
	Top          int
}

type Row struct {
	Rank              int
	Listing           listing.Listing
	Score             float64
	Coverage          float64
	Vibe              float64
	MissingEssentials []string
	AvoidsHit         []string
	Summary           string
	ByModel           map[string]listing.Assessment
	Stale             bool
}

type Report struct {
	Profile profile.Profile
	Models  []string
	Rows    []Row
	Stale   int
}

func Build(p profile.Profile, listings []listing.Listing, opts Options) Report {
	hash := p.Hash()
	r := Report{Profile: p}
	models := map[string]bool{}

	for _, l := range listings {
		byModel := map[string]listing.Assessment{}
		stale := false
		for model, a := range l.Assessments[p.ID] {
			if opts.Model != "" && model != opts.Model {
				continue
			}
			if a.ProfileHash != hash || a.InputHash != profile.InputHash(l) {
				stale = true
				if !opts.IncludeStale {
					continue
				}
			}
			byModel[model] = a
		}
		if len(byModel) == 0 {
			if stale {
				r.Stale++
			}
			continue
		}
		for m := range byModel {
			models[m] = true
		}
		row := combine(l, byModel)
		row.Stale = stale
		if row.Score < opts.MinScore {
			continue
		}
		r.Rows = append(r.Rows, row)
	}

	slices.SortFunc(r.Rows, func(a, b Row) int {
		return cmp.Or(
			cmp.Compare(b.Score, a.Score),
			cmp.Compare(len(a.MissingEssentials), len(b.MissingEssentials)),
			cmp.Compare(b.Coverage, a.Coverage),
			cmp.Compare(a.Listing.Price.Cents, b.Listing.Price.Cents),
			cmp.Compare(a.Listing.SourceID, b.Listing.SourceID),
		)
	})
	if opts.Top > 0 && len(r.Rows) > opts.Top {
		r.Rows = r.Rows[:opts.Top]
	}
	for i := range r.Rows {
		r.Rows[i].Rank = i + 1
	}
	for m := range models {
		r.Models = append(r.Models, m)
	}
	slices.Sort(r.Models)
	return r
}

func combine(l listing.Listing, byModel map[string]listing.Assessment) Row {
	row := Row{Listing: l, ByModel: byModel}
	missing := map[string]bool{}
	avoids := map[string]bool{}
	var latest listing.Assessment
	models := sortedKeys(func() map[string]bool {
		m := map[string]bool{}
		for k := range byModel {
			m[k] = true
		}
		return m
	}())
	for i, model := range models {
		a := byModel[model]
		row.Score += a.Score
		row.Coverage += a.Coverage
		row.Vibe += float64(a.Vibe)
		for _, id := range a.MissingEssentials {
			missing[id] = true
		}
		for _, id := range a.AvoidsHit {
			avoids[id] = true
		}
		if i == 0 || a.AssessedAt.After(latest.AssessedAt) {
			latest = a
		}
	}
	n := float64(len(byModel))
	row.Score = round1(row.Score / n)
	row.Coverage = round1(row.Coverage / n)
	row.Vibe = round1(row.Vibe / n)
	row.MissingEssentials = sortedKeys(missing)
	row.AvoidsHit = sortedKeys(avoids)
	row.Summary = latest.Summary
	return row
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func round1(f float64) float64 {
	return math.Round(f*10) / 10
}
