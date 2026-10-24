package report

import (
	"cmp"
	"maps"
	"math"
	"slices"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/dedupe"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

type Options struct {
	Offer        listing.OfferType
	Model        string
	IncludeStale bool
	Dealbreakers bool
	MinScore     float64
	Top          int
}

type Template struct {
	Profile    profile.Profile
	References []listing.Listing
}

type Grade struct {
	Profile           string                        `json:"profile"`
	Match             float64                       `json:"match"`
	Calibrated        bool                          `json:"calibrated"`
	Score             float64                       `json:"score"`
	Coverage          float64                       `json:"coverage"`
	MissingEssentials []string                      `json:"missing_essentials,omitempty"`
	Dealbreakers      []string                      `json:"dealbreakers,omitempty"`
	AvoidsHit         []string                      `json:"avoids_hit,omitempty"`
	Summary           string                        `json:"summary"`
	ByModel           map[string]listing.Assessment `json:"by_model"`
	Stale             bool                          `json:"stale,omitempty"`
}

type Row struct {
	Rank    int             `json:"rank"`
	Listing listing.Listing `json:"listing"`
	Grade
	Grades     map[string]Grade `json:"grades"`
	AlsoListed []Link           `json:"also_listed,omitempty"`
}

type Link struct {
	Source   listing.Source `json:"source"`
	SourceID string         `json:"source_id"`
	URL      string         `json:"url"`
	Price    listing.Money  `json:"price"`
}

type Report struct {
	Day          string
	Templates    []Template
	Models       []string
	Rows         []Row
	Stale        int
	Dealbreakers int
	Uncalibrated []string
}

func (r Report) ProfileIDs() []string {
	ids := make([]string, len(r.Templates))
	for i, t := range r.Templates {
		ids[i] = t.Profile.ID
	}
	return ids
}

func Build(templates []Template, listings []listing.Listing, opts Options) Report {
	r := Report{Templates: templates}
	models := map[string]bool{}
	uncalibrated := map[string]bool{}

	for _, g := range dedupe.Groups(listings) {
		if opts.Offer != "" && g.Primary.Offer != opts.Offer {
			continue
		}
		row, ok, stale := groupRow(templates, g.Members(), opts)
		if !ok {
			if stale {
				r.Stale++
			}
			continue
		}
		for _, grade := range row.Grades {
			for m := range grade.ByModel {
				models[m] = true
			}
			if !grade.Calibrated {
				uncalibrated[grade.Profile] = true
			}
		}
		if row.Match < opts.MinScore {
			continue
		}
		if len(row.Dealbreakers) > 0 && !opts.Dealbreakers {
			r.Dealbreakers++
			continue
		}
		r.Rows = append(r.Rows, row)
	}

	slices.SortFunc(r.Rows, func(a, b Row) int {
		return cmp.Or(
			better(a.Grade, b.Grade),
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
	r.Models = sortedKeys(models)
	for _, id := range r.ProfileIDs() {
		if uncalibrated[id] {
			r.Uncalibrated = append(r.Uncalibrated, id)
		}
	}
	return r
}

func better(a, b Grade) int {
	return cmp.Or(
		cmp.Compare(len(a.Dealbreakers), len(b.Dealbreakers)),
		cmp.Compare(b.Match, a.Match),
		cmp.Compare(len(a.MissingEssentials), len(b.MissingEssentials)),
		cmp.Compare(b.Coverage, a.Coverage),
	)
}

func groupRow(templates []Template, members []listing.Listing, opts Options) (Row, bool, bool) {
	row := Row{Grades: map[string]Grade{}}
	stale := false
	best := -1
	for _, t := range templates {
		hash := t.Profile.Hash()
		for i, l := range members {
			byModel, s := assessments(t.Profile.ID, hash, l, opts)
			stale = stale || s
			if len(byModel) == 0 {
				continue
			}
			grade := combine(t, byModel)
			grade.Stale = s
			row.Grades[t.Profile.ID] = grade
			if best < 0 || better(grade, row.Grade) < 0 {
				best = i
				row.Grade = grade
			}
			break
		}
	}
	if best < 0 {
		return Row{}, false, stale
	}
	row.Listing = members[best]
	row.AlsoListed = alsoListed(slices.Delete(slices.Clone(members), best, best+1))
	return row, true, stale
}

func assessments(profileID, hash string, l listing.Listing, opts Options) (map[string]listing.Assessment, bool) {
	byModel := map[string]listing.Assessment{}
	stale := false
	for model, a := range l.Assessments[profileID] {
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
	return byModel, stale
}

func alsoListed(ls []listing.Listing) []Link {
	if len(ls) == 0 {
		return nil
	}
	out := make([]Link, len(ls))
	for i, l := range ls {
		out[i] = Link{Source: l.Source, SourceID: l.SourceID, URL: l.URL, Price: l.Price}
	}
	return out
}

func combine(t Template, byModel map[string]listing.Assessment) Grade {
	grade := Grade{Profile: t.Profile.ID, ByModel: byModel}
	missing := map[string]bool{}
	dealbreakers := map[string]bool{}
	avoids := map[string]bool{}
	var latest listing.Assessment
	var relative float64
	calibrated := 0
	for i, model := range slices.Sorted(maps.Keys(byModel)) {
		a := byModel[model]
		grade.Score += a.Score
		grade.Coverage += a.Coverage
		if ref, ok := profile.ReferenceScore(t.Profile, t.References, model); ok {
			relative += 100 * a.Score / ref
			calibrated++
		}
		for _, id := range a.MissingEssentials {
			missing[id] = true
		}
		for _, id := range a.AvoidsHit {
			avoids[id] = true
		}
		for _, id := range a.Dealbreakers {
			dealbreakers[id] = true
		}
		if i == 0 || a.AssessedAt.After(latest.AssessedAt) {
			latest = a
		}
	}
	n := float64(len(byModel))
	grade.Score = round1(grade.Score / n)
	grade.Coverage = round1(grade.Coverage / n)
	grade.Match = grade.Score
	if calibrated > 0 {
		grade.Match = round1(relative / float64(calibrated))
		grade.Calibrated = true
	}
	grade.MissingEssentials = sortedKeys(missing)
	grade.AvoidsHit = sortedKeys(avoids)
	grade.Dealbreakers = sortedKeys(dealbreakers)
	grade.Summary = latest.Summary
	return grade
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
