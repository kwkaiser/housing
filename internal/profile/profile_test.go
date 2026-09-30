package profile

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
)

type fakeCompleter struct {
	reply string
	req   openrouter.Request
}

func (f *fakeCompleter) Complete(_ context.Context, req openrouter.Request) (openrouter.Response, error) {
	f.req = req
	return openrouter.Response{Model: "test/model", CostUSD: 0.02, Content: f.reply}, nil
}

const reply = `{
  "name": "Sunny attic",
  "summary": "Top floor with skylights.",
  "want": [
    {"id": "skylights", "label": "Skylights", "look_for": "roof windows", "not_this": "", "keywords": ["skylight"], "importance": "essential", "evidence": "either"},
    {"id": "wood_floors", "label": "Wood floors", "look_for": "hardwood", "not_this": "vinyl plank", "keywords": ["hardwood"], "importance": "high", "evidence": "either"}
  ],
  "avoid": [
    {"id": "basement", "label": "Basement", "look_for": "half windows", "not_this": "", "keywords": [], "importance": "high", "evidence": "either"}
  ],
  "ignore": ["Furniture and decor", "rugs"]
}`

func TestDraftAndApply(t *testing.T) {
	fc := &fakeCompleter{reply: reply}
	d := Drafter{Client: fc, Model: DefaultDraftModel}
	beds := 2
	refs := []ReferenceInput{{
		Listing:  listing.Listing{Address: listing.Address{Formatted: "7 Adams St"}, Beds: &beds, Description: "Skylights throughout"},
		Collages: [][]byte{{1}, {2}},
	}}

	draft, meta, err := d.Draft(context.Background(), KindWant, []string{"skylights!!", "wood floors"}, refs)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Model != "test/model" || meta.CostUSD != 0.02 {
		t.Errorf("meta = %+v", meta)
	}

	req, _ := json.Marshal(fc.req)
	for _, want := range []string{"skylights!!", "Skylights throughout", "Bedrooms: 2", "collage 2", "housing_profile"} {
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
	if images != 2 {
		t.Errorf("sent %d images, want 2", images)
	}

	p := Profile{ID: "attic", Notes: []string{"skylights!!"}}
	if err := p.Apply(draft, meta); err != nil {
		t.Fatal(err)
	}
	if p.Name != "Sunny attic" || len(p.Want) != 2 || len(p.Avoid) != 1 || p.Drafted == nil {
		t.Errorf("profile = %+v", p)
	}
	if len(p.Ignore) != len(DefaultIgnore)+1 {
		t.Errorf("ignore should merge defaults case-insensitively: %v", p.Ignore)
	}
}

func TestApplyRejectsInvalidDraft(t *testing.T) {
	p := Profile{ID: "attic"}
	dup := Criterion{ID: "a", Importance: High, Evidence: EvidencePhotos}
	err := p.Apply(Draft{Want: []Criterion{dup, dup}}, Drafted{})
	if err == nil || p.Want != nil {
		t.Fatalf("expected duplicate id error without mutating profile, got %v", err)
	}
}

func TestStoreRoundTrip(t *testing.T) {
	s := Store{Root: t.TempDir()}
	p := Profile{
		ID:         "attic",
		Name:       "Attic",
		Want:       []Criterion{{ID: "skylights", Label: "Skylights", Importance: Essential, Evidence: EvidenceEither}},
		Ignore:     DefaultIgnore,
		References: []Reference{{Source: listing.SourceZillow, SourceID: "1", URL: "u", Collages: []string{"collages/x/0.jpg"}}},
	}
	if err := s.Save(p); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("attic")
	if err != nil {
		t.Fatal(err)
	}
	if got.Want[0].ID != "skylights" || got.References[0].Collages[0] != "collages/x/0.jpg" {
		t.Errorf("got %+v", got)
	}
	if _, err := s.Load("missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("got %v", err)
	}
}

func TestValidID(t *testing.T) {
	for _, id := range []string{"attic", "somerville-attic-2"} {
		if ValidID(id) != nil {
			t.Errorf("%q should be valid", id)
		}
	}
	for _, id := range []string{"", "Attic", "../x", "a b", "-x"} {
		if ValidID(id) == nil {
			t.Errorf("%q should be invalid", id)
		}
	}
}
