package profile

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func avoidProfile() Profile {
	return Profile{
		ID:   "corporate",
		Kind: KindAvoid,
		Avoid: []Criterion{
			{ID: "large_building", Label: "Large building", Importance: Essential, Evidence: EvidenceEither},
			{ID: "amenity_package", Label: "Amenity package", Importance: High, Evidence: EvidenceDescription},
		},
		Ignore:     DefaultIgnore,
		References: []Reference{{Source: listing.SourceZillow, SourceID: "165", URL: "u", Collages: []string{"collages/c/0.jpg"}}},
	}
}

func TestEffectiveMergesAvoidProfiles(t *testing.T) {
	want := testProfile
	want.References = nil
	before, err := Effective(want, []Profile{want})
	if err != nil {
		t.Fatal(err)
	}

	ap := avoidProfile()
	eff, err := Effective(want, []Profile{ap, want})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, c := range eff.Avoid {
		ids = append(ids, c.ID)
	}
	if got := strings.Join(ids, ","); got != "basement,corporate.large_building,corporate.amenity_package" {
		t.Errorf("avoid ids = %s", got)
	}
	if len(eff.References) != 1 || !eff.References[0].Avoid || eff.References[0].Profile != "corporate" {
		t.Errorf("references = %+v", eff.References)
	}
	if eff.Hash() == before.Hash() {
		t.Error("adding an avoid profile should change the effective hash")
	}

	ap.Avoid[1].Importance = Low
	edited, _ := Effective(want, []Profile{ap, want})
	if edited.Hash() == eff.Hash() {
		t.Error("editing an avoid profile should change the effective hash")
	}

	if _, err := Effective(ap, []Profile{ap}); err == nil {
		t.Error("an avoid profile should not be usable as the profile to assess against")
	}
	if len(want.Avoid) != 1 || want.References != nil {
		t.Error("Effective should not modify the stored profile")
	}
}

func TestDealbreakers(t *testing.T) {
	p := testProfile
	p.Avoid = append(append([]Criterion{}, p.Avoid...), Criterion{ID: "corporate.large_building", Importance: Essential, Evidence: EvidenceEither})
	a := listing.Assessment{
		Want: []listing.CriterionResult{{Verdict: listing.VerdictPresent}, {Verdict: listing.VerdictPresent}, {Verdict: listing.VerdictPresent}},
		Avoid: []listing.CriterionResult{
			{ID: "basement", Verdict: listing.VerdictPartial},
			{ID: "corporate.large_building", Verdict: listing.VerdictPresent},
		},
	}
	Score(p, &a)
	if strings.Join(a.Dealbreakers, ",") != "corporate.large_building" {
		t.Errorf("dealbreakers = %v", a.Dealbreakers)
	}
	if strings.Join(a.AvoidsHit, ",") != "basement,corporate.large_building" {
		t.Errorf("avoids hit = %v", a.AvoidsHit)
	}
}

func TestDraftAvoid(t *testing.T) {
	fc := &fakeCompleter{reply: `{"name": "Corporate complex", "summary": "Big managed buildings.",
		"want": [{"id": "stray", "label": "Stray", "look_for": "x", "not_this": "", "keywords": [], "importance": "low", "evidence": "photos"}],
		"avoid": [{"id": "large_building", "label": "Large building", "look_for": "x", "not_this": "", "keywords": ["concierge"], "importance": "essential", "evidence": "either"}],
		"ignore": []}`}
	d := Drafter{Client: fc, Model: "m"}
	draft, meta, err := d.Draft(context.Background(), KindAvoid, []string{"soulless!!"}, []ReferenceInput{{Collages: [][]byte{{1}}}})
	if err != nil {
		t.Fatal(err)
	}
	req, _ := json.Marshal(fc.req)
	if !strings.Contains(string(req), "DISLIKES") || !strings.Contains(string(req), "What I dislike") {
		t.Error("avoid drafting should use the avoid prompt")
	}

	p := Profile{ID: "corporate", Kind: KindAvoid}
	if err := p.Apply(draft, meta); err != nil {
		t.Fatal(err)
	}
	if len(p.Want) != 0 || len(p.Avoid) != 2 {
		t.Errorf("avoid profile should hold only avoid criteria: want=%v avoid=%v", p.Want, p.Avoid)
	}
}

func TestAssessLabelsAvoidExamples(t *testing.T) {
	fc := &fakeCompleter{reply: assessReply}
	a := Assessor{Client: fc, Model: "m"}
	refs := []ReferenceInput{{Collages: [][]byte{{1}}}, {Collages: [][]byte{{2}}, Avoid: true}}
	c := Candidate{Listing: listing.Listing{Collages: []string{"x"}}, Collages: [][]byte{{3}}}
	if _, err := a.Assess(context.Background(), testProfile, refs, c); err != nil {
		t.Fatal(err)
	}
	req, _ := json.Marshal(fc.req)
	if !strings.Contains(string(req), "REFERENCE listing 1") || !strings.Contains(string(req), "AVOID EXAMPLE 1") {
		t.Errorf("request should label want and avoid references separately")
	}

	if _, err := a.Assess(context.Background(), Profile{ID: "corporate", Kind: KindAvoid}, nil, c); err == nil {
		t.Error("assessing against an avoid profile should fail")
	}
}
