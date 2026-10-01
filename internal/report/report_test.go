package report

import (
	"fmt"
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

var single = []Template{{Profile: testProfile}}

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

	r := Build(single, listings, Options{})
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

	byModel := Build(single, listings, Options{Model: "a/m1", Top: 2})
	if len(byModel.Rows) != 2 || byModel.Rows[0].Listing.SourceID != "both" || byModel.Rows[0].Score != 80 {
		t.Errorf("model filter + top: %+v", byModel.Rows)
	}

	sale := assessed("sale", listing.SourceZillow, 100, map[string]float64{"a/m1": 50})
	sale.Offer = listing.OfferSale
	sale = sale.WithAssessment(testProfile.ID, listing.Assessment{Model: "a/m1", Score: 50, ProfileHash: testProfile.Hash(), InputHash: profile.InputHash(sale)})
	rentOnly := Build(single, append(listings, sale), Options{Offer: listing.OfferRent})
	for _, row := range rentOnly.Rows {
		if row.Listing.Offer != listing.OfferRent {
			t.Errorf("offer filter leaked %s", row.Listing.SourceID)
		}
	}
	saleOnly := Build(single, append(listings, sale), Options{Offer: listing.OfferSale})
	if len(saleOnly.Rows) != 1 || saleOnly.Rows[0].Listing.SourceID != "sale" {
		t.Errorf("sale filter: %+v", saleOnly.Rows)
	}

	withStale := Build(single, listings, Options{IncludeStale: true, MinScore: 90})
	if len(withStale.Rows) != 1 || !withStale.Rows[0].Stale {
		t.Errorf("include stale + min score: %+v", withStale.Rows)
	}
}

func TestBuildHidesDealbreakers(t *testing.T) {
	bad := assessed("bad", listing.SourceZillow, 100, map[string]float64{"a/m1": 90})
	a := bad.Assessments["attic"]["a/m1"]
	a.Dealbreakers = []string{"corporate"}
	bad.Assessments["attic"]["a/m1"] = a
	good := assessed("good", listing.SourceZillow, 100, map[string]float64{"a/m1": 10})

	r := Build(single, []listing.Listing{bad, good}, Options{})
	if len(r.Rows) != 1 || r.Rows[0].Listing.SourceID != "good" || r.Rows[0].Rank != 1 || r.Dealbreakers != 1 {
		t.Errorf("dealbreakers should be hidden by default: %+v %d", r.Rows, r.Dealbreakers)
	}
	if r := Build(single, []listing.Listing{bad, good}, Options{Dealbreakers: true}); len(r.Rows) != 2 || r.Dealbreakers != 0 {
		t.Errorf("--include-dealbreakers should keep them: %+v", r.Rows)
	}
}

func TestBuildDuplicates(t *testing.T) {
	beds := 2
	at := func(l listing.Listing) listing.Listing {
		l.Address = listing.Address{Formatted: "9 Kidder Ave #2, Somerville, MA", Street: "9 Kidder Ave", Unit: "2", PostalCode: "02144"}
		l.Beds = &beds
		return l
	}
	z := at(listing.Listing{Source: listing.SourceZillow, SourceID: "z", URL: "https://example.com/z", Offer: listing.OfferRent, Price: listing.Money{Cents: 280000, Currency: "USD"}})
	c := at(assessed("c", listing.SourceCraigslist, 280000, map[string]float64{"a/m1": 60}))

	r := Build(single, []listing.Listing{z, c}, Options{})
	if len(r.Rows) != 1 {
		t.Fatalf("duplicates should share one row: %+v", r.Rows)
	}
	row := r.Rows[0]
	if row.Listing.SourceID != "c" || len(row.AlsoListed) != 1 || row.AlsoListed[0].URL != "https://example.com/z" {
		t.Errorf("row should use the assessed listing and link the other: %+v", row)
	}
}

func TestBuildTemplates(t *testing.T) {
	loft := profile.Profile{
		ID:   "loft",
		Want: []profile.Criterion{{ID: "arches", Label: "Arched windows", Importance: profile.High, Evidence: profile.EvidencePhotos}},
	}
	grade := func(l listing.Listing, p profile.Profile, score float64) listing.Listing {
		return l.WithAssessment(p.ID, listing.Assessment{Model: "a/m1", Score: score, ProfileHash: p.Hash(), InputHash: profile.InputHash(l)})
	}
	atticRef := grade(listing.Listing{Source: listing.SourceZillow, SourceID: "attic-ref"}, testProfile, 80)
	loftRef := grade(listing.Listing{Source: listing.SourceZillow, SourceID: "loft-ref"}, loft, 40)
	templates := []Template{{Profile: testProfile, References: []listing.Listing{atticRef}}, {Profile: loft, References: []listing.Listing{loftRef}}}

	atticish := grade(grade(listing.Listing{Source: listing.SourceZillow, SourceID: "atticish"}, testProfile, 60), loft, 10)
	loftish := grade(grade(listing.Listing{Source: listing.SourceZillow, SourceID: "loftish"}, testProfile, 20), loft, 36)
	loftOnly := grade(listing.Listing{Source: listing.SourceZillow, SourceID: "loft-only"}, loft, 20)

	r := Build(templates, []listing.Listing{atticish, loftish, loftOnly}, Options{})
	var got []string
	for _, row := range r.Rows {
		got = append(got, fmt.Sprintf("%s:%s:%g", row.Listing.SourceID, row.Profile, row.Match))
	}
	if want := "loftish:loft:90,atticish:attic:75,loft-only:loft:50"; strings.Join(got, ",") != want {
		t.Errorf("rows = %s, want %s", strings.Join(got, ","), want)
	}
	if g := r.Rows[0].Grades["attic"]; g.Match != 25 || !g.Calibrated || len(r.Rows[0].Grades) != 2 {
		t.Errorf("every profile's grade should be kept: %+v", r.Rows[0].Grades)
	}
	if len(r.Uncalibrated) != 0 {
		t.Errorf("uncalibrated = %v", r.Uncalibrated)
	}

	loftRef.Assessments = nil
	templates[1].References = []listing.Listing{loftRef}
	if r := Build(templates, []listing.Listing{loftOnly}, Options{}); r.Rows[0].Match != 20 || strings.Join(r.Uncalibrated, ",") != "loft" {
		t.Errorf("without a graded reference, match should fall back to the score: %+v %v", r.Rows[0].Grade, r.Uncalibrated)
	}
}
