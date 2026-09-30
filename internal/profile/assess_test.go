package profile

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
)

var testProfile = Profile{
	ID:      "attic",
	Name:    "Attic",
	Summary: "Sunny attic",
	Want: []Criterion{
		{ID: "skylights", Label: "Skylights", Importance: Essential, Evidence: EvidenceEither},
		{ID: "wood_floors", Label: "Wood floors", Importance: High, Evidence: EvidenceEither, NotThis: "vinyl plank"},
		{ID: "radiators", Label: "Radiators", Importance: Low, Evidence: EvidencePhotos},
	},
	Avoid: []Criterion{
		{ID: "basement", Label: "Basement", Importance: Medium, Evidence: EvidenceEither},
	},
	Ignore: DefaultIgnore,
}

const assessReply = `{
  "want": [
    {"id": "skylights", "verdict": "absent", "confidence": "high", "photos": [], "evidence": "Flat ceilings throughout."},
    {"id": "wood_floors", "verdict": "present", "confidence": "medium", "photos": [2, 3], "evidence": "Oak boards visible."},
    {"id": "made_up", "verdict": "present", "confidence": "high", "photos": [], "evidence": "hallucinated"}
  ],
  "avoid": [
    {"id": "basement", "verdict": "present", "confidence": "high", "photos": [1], "evidence": "Garden level."}
  ],
  "vibe": 2,
  "summary": "Not much like the reference."
}`

type countingCompleter struct {
	fakeCompleter
	calls int
}

func (c *countingCompleter) Complete(ctx context.Context, req openrouter.Request) (openrouter.Response, error) {
	c.calls++
	return c.fakeCompleter.Complete(ctx, req)
}

func TestScore(t *testing.T) {
	a := listing.Assessment{
		Want: []listing.CriterionResult{
			{ID: "skylights", Verdict: listing.VerdictAbsent},
			{ID: "wood_floors", Verdict: listing.VerdictPresent},
			{ID: "radiators", Verdict: listing.VerdictUnknown},
		},
		Avoid: []listing.CriterionResult{{ID: "basement", Verdict: listing.VerdictPresent}},
	}
	Score(testProfile, &a)
	if a.Score != 12.5 || a.Coverage != 87.5 {
		t.Errorf("score=%v coverage=%v, want 12.5 and 87.5", a.Score, a.Coverage)
	}
	if strings.Join(a.MissingEssentials, ",") != "skylights" || strings.Join(a.AvoidsHit, ",") != "basement" {
		t.Errorf("missing=%v avoids=%v", a.MissingEssentials, a.AvoidsHit)
	}

	a.Avoid[0].Verdict = listing.VerdictAbsent
	a.Want[0].Verdict = listing.VerdictAbsent
	a.Want[1].Verdict = listing.VerdictAbsent
	Score(testProfile, &a)
	if a.Score != 0 {
		t.Errorf("score should floor at 0, got %v", a.Score)
	}
}

func TestAssess(t *testing.T) {
	fc := &fakeCompleter{reply: assessReply}
	a := Assessor{Client: fc, Model: "test/model"}
	beds := 1
	c := Candidate{
		Listing:  listing.Listing{Source: listing.SourceZillow, SourceID: "1", Address: listing.Address{Formatted: "1 Main St"}, Beds: &beds, Collages: []string{"c/0.jpg"}},
		Collages: [][]byte{{9}},
	}
	refs := []ReferenceInput{{Collages: [][]byte{{1}, {2}}}}

	got, err := a.Assess(context.Background(), testProfile, refs, c)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Want) != 3 || got.Want[2].ID != "radiators" || got.Want[2].Verdict != listing.VerdictUnknown {
		t.Errorf("criteria missing from the reply should be unknown: %+v", got.Want)
	}
	if got.Score != 12.5 || got.Vibe != 2 || got.Model != "test/model" || got.ProfileHash != testProfile.Hash() || got.InputHash != InputHash(c.Listing) {
		t.Errorf("assessment = %+v", got)
	}

	req, _ := json.Marshal(fc.req)
	for _, want := range []string{"REFERENCE listing 1", "CANDIDATE collage 1", "vinyl plank", "Bedrooms: 1", "listing_assessment"} {
		if !strings.Contains(string(req), want) {
			t.Errorf("request missing %q", want)
		}
	}
	images := 0
	for _, p := range fc.req.Messages[1].Content {
		if p.ImageURL != nil {
			images++
		}
	}
	if images != 3 {
		t.Errorf("sent %d images, want 2 reference + 1 candidate", images)
	}
}

func TestAssessRejectsEmptyReply(t *testing.T) {
	c := Candidate{Listing: listing.Listing{Collages: []string{"x"}}, Collages: [][]byte{{1}}}
	for _, reply := range []string{`{}`, `{"want": [], "avoid": [], "vibe": 3, "summary": ""}`, strings.Replace(assessReply, `"vibe": 2`, `"vibe": 0`, 1)} {
		a := Assessor{Client: &fakeCompleter{reply: reply}, Model: "m"}
		if _, err := a.Assess(context.Background(), testProfile, nil, c); err == nil || !strings.Contains(err.Error(), "unusable") {
			t.Errorf("reply %.40s: got %v", reply, err)
		}
	}
}

func TestAssessRequiresCriteria(t *testing.T) {
	a := Assessor{Client: &fakeCompleter{}, Model: "m"}
	_, err := a.Assess(context.Background(), Profile{ID: "empty"}, nil, Candidate{Collages: [][]byte{{1}}})
	if err == nil || !strings.Contains(err.Error(), "profile draft") {
		t.Fatalf("got %v", err)
	}
}

func TestAssessListings(t *testing.T) {
	cc := &countingCompleter{fakeCompleter: fakeCompleter{reply: assessReply}}
	a := Assessor{Client: cc, Model: "test/model"}
	read := func(keys []string) ([][]byte, error) { return [][]byte{{1}}, nil }

	listings := []listing.Listing{
		{SourceID: "unit1", Collages: []string{"b/0.jpg"}, Description: "building"},
		{SourceID: "unit2", Collages: []string{"b/0.jpg"}, Description: "building"},
		{SourceID: "home", Collages: []string{"h/0.jpg"}},
		{SourceID: "other", Collages: []string{"o/0.jpg"}},
		{SourceID: "nophotos"},
	}

	out, stats, err := a.AssessListings(context.Background(), testProfile, nil, listings, read, BatchOptions{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if cc.calls != 2 || stats.Calls != 2 || stats.Updated != 3 || stats.OverLimit != 1 || stats.NoCollages != 1 {
		t.Errorf("calls=%d stats=%+v", cc.calls, stats)
	}
	a0, _ := out[0].Assessment("attic", "test/model")
	a1, _ := out[1].Assessment("attic", "test/model")
	if a0.Score != 12.5 || a1.Score != 12.5 {
		t.Error("units sharing collages and description should share one assessment")
	}
	if listings[0].Assessments != nil {
		t.Error("AssessListings should not mutate its input")
	}
	if _, ok := out[3].Assessment("attic", "test/model"); ok {
		t.Error("listing over the limit should not be assessed")
	}

	out, stats, err = a.AssessListings(context.Background(), testProfile, nil, out, read, BatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if cc.calls != 3 || stats.Cached != 3 || stats.Calls != 1 {
		t.Errorf("second run should only assess the remaining listing: calls=%d stats=%+v", cc.calls, stats)
	}

	_, stats, _ = a.AssessListings(context.Background(), testProfile, nil, out, read, BatchOptions{Force: true})
	if stats.Calls != 3 || stats.Cached != 0 {
		t.Errorf("force should reassess every distinct input: %+v", stats)
	}

	other := Assessor{Client: cc, Model: "other/model"}
	if other.Current(testProfile, out[0]) {
		t.Error("an assessment by one model should not count as current for another")
	}
	withOther, _, err := other.AssessListings(context.Background(), testProfile, nil, out[:1], read, BatchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(withOther[0].Assessments["attic"]) != 2 {
		t.Errorf("assessments should be kept per model: %v", withOther[0].Assessments["attic"])
	}

	changed := testProfile
	changed.Want = append([]Criterion{}, testProfile.Want...)
	changed.Want[1].Importance = Medium
	if a.Current(changed, out[0]) {
		t.Error("editing the profile should invalidate cached assessments")
	}
}
