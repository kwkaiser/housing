package profile

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync/atomic"
	"testing"
	"time"

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
	reply string
	cost  float64
	fail  string
	calls atomic.Int32
	live  atomic.Int32
	peak  atomic.Int32
	delay time.Duration
}

func (c *countingCompleter) Complete(ctx context.Context, req openrouter.Request) (openrouter.Response, error) {
	c.calls.Add(1)
	n := c.live.Add(1)
	defer c.live.Add(-1)
	for {
		p := c.peak.Load()
		if n <= p || c.peak.CompareAndSwap(p, n) {
			break
		}
	}
	if c.delay > 0 {
		time.Sleep(c.delay)
	}
	for _, part := range req.User {
		if c.fail != "" && strings.Contains(part.Text, c.fail) {
			return openrouter.Response{}, errors.New("boom")
		}
	}
	return openrouter.Response{Model: "test/model", CostUSD: c.cost, Content: c.reply}, nil
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
	for _, p := range fc.req.User {
		if p.Data != nil {
			images++
		}
	}
	if images != 3 {
		t.Errorf("sent %d images, want 2 reference + 1 candidate", images)
	}
}

func manyListings(n int) []listing.Listing {
	out := make([]listing.Listing, n)
	for i := range out {
		id := string(rune('a' + i))
		out[i] = listing.Listing{SourceID: id, Collages: []string{id + ".jpg"}, Address: listing.Address{Formatted: "addr-" + id}}
	}
	return out
}

func readAny(keys []string) ([][]byte, error) { return [][]byte{{1}}, nil }

func TestAssessListingsConcurrency(t *testing.T) {
	cc := &countingCompleter{reply: assessReply, delay: 20 * time.Millisecond}
	a := Assessor{Client: cc, Model: "test/model"}
	out, stats, err := a.AssessListings(context.Background(), testProfile, nil, manyListings(8), readAny, BatchOptions{Concurrency: 4})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Calls != 8 || stats.Updated != 8 || cc.peak.Load() != 4 {
		t.Errorf("stats=%+v peak=%d, want 8 calls with 4 in flight", stats, cc.peak.Load())
	}
	for _, l := range out {
		if _, ok := l.Assessment("attic", "test/model"); !ok {
			t.Errorf("%s not assessed", l.SourceID)
		}
	}
}

func TestAssessListingsBudget(t *testing.T) {
	cc := &countingCompleter{reply: assessReply, cost: 0.01}
	a := Assessor{Client: cc, Model: "test/model"}
	_, stats, err := a.AssessListings(context.Background(), testProfile, nil, manyListings(6), readAny, BatchOptions{Concurrency: 1, MaxCostUSD: 0.025})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Calls != 3 || stats.OverBudget != 3 {
		t.Errorf("stats=%+v, want calls to stop once spend reaches the cap", stats)
	}
}

func TestAssessListingsSharedBudget(t *testing.T) {
	cc := &countingCompleter{reply: assessReply, cost: 0.01}
	a := Assessor{Client: cc, Model: "test/model"}
	budget := NewBudget(0.035)
	opts := BatchOptions{Concurrency: 1, MaxCostUSD: 1, Budget: budget}
	_, first, _ := a.AssessListings(context.Background(), testProfile, nil, manyListings(2), readAny, opts)
	_, second, _ := a.AssessListings(context.Background(), testProfile, nil, manyListings(4), readAny, opts)
	if first.Calls != 2 || second.Calls != 2 || second.OverBudget != 2 || budget.Spent() < 0.039 || !budget.Exhausted() {
		t.Errorf("first=%+v second=%+v spent=%v, want the budget shared across batches", first, second, budget.Spent())
	}
	if (*Budget)(nil).Exhausted() || NewBudget(0).Exhausted() {
		t.Error("a missing or zero budget should never be exhausted")
	}
}

func TestAssessListingsCheckpoints(t *testing.T) {
	a := Assessor{Client: &countingCompleter{reply: assessReply}, Model: "test/model"}
	var saved []int
	opts := BatchOptions{Concurrency: 2, CheckpointEvery: 2, Checkpoint: func(ls []listing.Listing) error {
		n := 0
		for _, l := range ls {
			if _, ok := l.Assessment("attic", "test/model"); ok {
				n++
			}
		}
		saved = append(saved, n)
		return nil
	}}
	if _, _, err := a.AssessListings(context.Background(), testProfile, nil, manyListings(5), readAny, opts); err != nil {
		t.Fatal(err)
	}
	if len(saved) != 2 || saved[0] != 2 || saved[1] != 4 {
		t.Errorf("checkpoints = %v, want progress saved after every 2 calls", saved)
	}
}

func TestAssessListingsContinuesPastFailures(t *testing.T) {
	cc := &countingCompleter{reply: assessReply, fail: "addr-c"}
	a := Assessor{Client: cc, Model: "test/model"}
	out, stats, err := a.AssessListings(context.Background(), testProfile, nil, manyListings(6), readAny, BatchOptions{Concurrency: 2})
	if err == nil || !strings.Contains(err.Error(), "boom") || !strings.Contains(err.Error(), "/c") {
		t.Fatalf("got %v", err)
	}
	if stats.Calls != 5 || stats.Failed != 1 || stats.Updated != 5 {
		t.Errorf("stats=%+v, want the other 5 listings assessed", stats)
	}
	if _, ok := out[2].Assessment("attic", "test/model"); ok {
		t.Error("failed listing should have no assessment")
	}
	if _, ok := out[5].Assessment("attic", "test/model"); !ok {
		t.Error("listings after the failure should still be assessed")
	}
}

type flakyCompleter struct {
	replies []string
	calls   atomic.Int32
}

func (f *flakyCompleter) Complete(context.Context, openrouter.Request) (openrouter.Response, error) {
	i := int(f.calls.Add(1)) - 1
	return openrouter.Response{Model: "m", CostUSD: 0.01, Content: f.replies[min(i, len(f.replies)-1)]}, nil
}

func TestAssessRetriesUnusableOutput(t *testing.T) {
	c := Candidate{Listing: listing.Listing{Collages: []string{"x"}}, Collages: [][]byte{{1}}}
	fc := &flakyCompleter{replies: []string{"", "{}", assessReply}}
	a := Assessor{Client: fc, Model: "m", Attempts: 3}
	got, err := a.Assess(context.Background(), testProfile, nil, c)
	if err != nil {
		t.Fatal(err)
	}
	if fc.calls.Load() != 3 || got.CostUSD != 0.03 {
		t.Errorf("calls=%d cost=%v, want 3 attempts with their cost summed", fc.calls.Load(), got.CostUSD)
	}

	always := &flakyCompleter{replies: []string{""}}
	if _, err := (Assessor{Client: always, Model: "m", Attempts: 2}).Assess(context.Background(), testProfile, nil, c); err == nil || always.calls.Load() != 2 {
		t.Errorf("err=%v calls=%d, want failure after 2 attempts", err, always.calls.Load())
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
	cc := &countingCompleter{reply: assessReply}
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
	if cc.calls.Load() != 2 || stats.Calls != 2 || stats.Updated != 3 || stats.OverLimit != 1 || stats.NoCollages != 1 {
		t.Errorf("calls=%d stats=%+v", cc.calls.Load(), stats)
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
	if cc.calls.Load() != 3 || stats.Cached != 3 || stats.Calls != 1 {
		t.Errorf("second run should only assess the remaining listing: calls=%d stats=%+v", cc.calls.Load(), stats)
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
