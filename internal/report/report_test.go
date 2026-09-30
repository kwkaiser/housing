package report

import (
	"bytes"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

var testProfile = profile.Profile{
	ID:   "attic",
	Name: "Attic",
	Want: []profile.Criterion{{ID: "skylights", Label: "Skylights", Importance: profile.Essential, Evidence: profile.EvidenceEither}},
}

func assessed(id string, source listing.Source, cents int64, scores map[string]float64) listing.Listing {
	l := listing.Listing{
		Source:   source,
		SourceID: id,
		URL:      "https://example.com/" + id,
		Offer:    listing.OfferRent,
		Price:    listing.Money{Cents: cents, Currency: "USD"},
		Address:  listing.Address{Formatted: id + " Main St"},
		Collages: []string{"c/" + id + ".jpg"},
	}
	for model, s := range scores {
		l = l.WithAssessment(testProfile.ID, listing.Assessment{
			Model:       model,
			Score:       s,
			Coverage:    100,
			Vibe:        3,
			ProfileHash: testProfile.Hash(),
			InputHash:   profile.InputHash(l),
			Summary:     id + " summary",
		})
	}
	return l
}

func TestBuild(t *testing.T) {
	stale := assessed("stale", listing.SourceZillow, 100, map[string]float64{"a/m1": 99})
	a := stale.Assessments["attic"]["a/m1"]
	a.ProfileHash = "old"
	stale.Assessments["attic"]["a/m1"] = a

	listings := []listing.Listing{
		assessed("low", listing.SourceZillow, 100, map[string]float64{"a/m1": 10}),
		assessed("both", listing.SourceCraigslist, 100, map[string]float64{"a/m1": 80, "b/m2": 60}),
		assessed("cheap", listing.SourceZillow, 50, map[string]float64{"a/m1": 70}),
		assessed("pricey", listing.SourceZillow, 500, map[string]float64{"a/m1": 70}),
		{SourceID: "unassessed"},
		stale,
	}

	r := Build(testProfile, listings, Options{})
	var ids []string
	for _, row := range r.Rows {
		ids = append(ids, row.Listing.SourceID)
	}
	if got := strings.Join(ids, ","); got != "cheap,both,pricey,low" {
		t.Errorf("order = %s, want combined score desc then price asc", got)
	}
	if r.Rows[1].Score != 70 || len(r.Rows[1].ByModel) != 2 {
		t.Errorf("multi-model row should average scores: %+v", r.Rows[1])
	}
	if r.Stale != 1 || strings.Join(r.Models, ",") != "a/m1,b/m2" {
		t.Errorf("stale=%d models=%v", r.Stale, r.Models)
	}

	byModel := Build(testProfile, listings, Options{Model: "a/m1", Top: 2})
	if len(byModel.Rows) != 2 || byModel.Rows[0].Listing.SourceID != "both" || byModel.Rows[0].Score != 80 {
		t.Errorf("model filter + top: %+v", byModel.Rows)
	}

	withStale := Build(testProfile, listings, Options{IncludeStale: true, MinScore: 90})
	if len(withStale.Rows) != 1 || !withStale.Rows[0].Stale {
		t.Errorf("include stale + min score: %+v", withStale.Rows)
	}
}

func TestRender(t *testing.T) {
	r := Build(testProfile, []listing.Listing{
		assessed("both", listing.SourceZillow, 250000, map[string]float64{"a/m1": 80, "b/m2": 60}),
	}, Options{})

	var md bytes.Buffer
	if err := Markdown(&md, r); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# Attic", "| m1 | m2 |", "**70.0**", "[both Main St](https://example.com/both) (zillow)", "$2,500/mo", "both summary"} {
		if !strings.Contains(md.String(), want) {
			t.Errorf("markdown missing %q:\n%s", want, md.String())
		}
	}

	var table bytes.Buffer
	if err := Table(&table, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(table.String(), "https://example.com/both") || !strings.Contains(table.String(), "M1") {
		t.Errorf("table:\n%s", table.String())
	}

	var js bytes.Buffer
	if err := JSON(&js, r); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(js.String(), `"score": 70`) || !strings.Contains(js.String(), `"by_model"`) {
		t.Errorf("json:\n%s", js.String())
	}
}
