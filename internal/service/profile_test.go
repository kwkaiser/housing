package service

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func seedProfiles(t *testing.T) (*Service, profile.Profile) {
	t.Helper()
	ctx := context.Background()
	svc := New(Config{DataDir: t.TempDir()})
	db, err := svc.OpenStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	crit := func(id, label string, imp profile.Importance) profile.Criterion {
		return profile.Criterion{ID: id, Label: label, LookFor: label, Keywords: []string{id}, Importance: imp, Evidence: profile.EvidenceEither}
	}
	ref := rental(listing.SourceZillow, "ref", "1 Attic Way")
	ref.Collages = []string{"collages/ab/one"}
	attic := profile.Profile{
		ID: "attic", Name: "Attic", Summary: "Top floor.\nSkylights.", Notes: []string{"skylights"}, Ignore: profile.DefaultIgnore,
		Want:       []profile.Criterion{crit("skylights", "Skylights", profile.High), crit("floors", "Wood floors", profile.Medium)},
		Avoid:      []profile.Criterion{crit("basement", "Basement", profile.Low)},
		References: []profile.Reference{{Source: ref.Source, SourceID: ref.SourceID, URL: ref.URL, Collages: ref.Collages}},
		Drafted:    &profile.Drafted{Model: "test/draft", CostUSD: 0.01},
	}
	corp := profile.Profile{ID: "corp", Kind: profile.KindAvoid, Name: "Corporate", Ignore: profile.DefaultIgnore,
		Avoid: []profile.Criterion{crit("carpet", "Carpet", profile.Essential)}}
	if err := db.SaveProfile(ctx, attic, ref); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveProfile(ctx, corp); err != nil {
		t.Fatal(err)
	}
	eff, err := db.EffectiveProfile(ctx, "attic")
	if err != nil {
		t.Fatal(err)
	}
	graded := ref.WithAssessment("attic", listing.Assessment{ProfileHash: eff.Hash(), InputHash: profile.InputHash(ref), Model: "test/a", Score: 75})
	graded = graded.WithAssessment("attic", listing.Assessment{ProfileHash: "old", InputHash: profile.InputHash(ref), Model: "test/b", Score: 50})
	if err := db.Save(ctx, []listing.Listing{graded}); err != nil {
		t.Fatal(err)
	}
	if err := db.SaveCollection(ctx, collection.Collection{ID: "somerville", Mode: profile.ModeRent, Sources: []listing.Source{listing.SourceZillow},
		Profiles: []string{"attic"}, Search: profile.Search{Location: "Somerville, MA"}}); err != nil {
		t.Fatal(err)
	}
	return svc, attic
}

func fieldErrors(err error) map[string]string {
	out := map[string]string{}
	var walk func(error)
	walk = func(err error) {
		if fe, ok := err.(profile.FieldError); ok {
			out[fe.Field] = fe.Error()
			return
		}
		if multi, ok := err.(interface{ Unwrap() []error }); ok {
			for _, e := range multi.Unwrap() {
				walk(e)
			}
		}
	}
	walk(err)
	return out
}

func TestValidateNewProfile(t *testing.T) {
	svc, _ := seedProfiles(t)
	ctx := context.Background()
	valid := CreateProfileOptions{URL: "https://www.zillow.com/homedetails/1", ID: "loft", Kind: profile.KindWant}
	if err := svc.ValidateNewProfile(ctx, valid); err != nil {
		t.Fatalf("valid options: %v", err)
	}
	for _, tc := range []struct {
		name  string
		edit  func(*CreateProfileOptions)
		field string
		msg   string
	}{
		{"missing url", func(o *CreateProfileOptions) { o.URL = " " }, "url", "a listing URL is required"},
		{"not http", func(o *CreateProfileOptions) { o.URL = "ftp://zillow.com/x" }, "url", "want an http or https link"},
		{"no host", func(o *CreateProfileOptions) { o.URL = "https://" }, "url", "want an http or https link"},
		{"unsupported host", func(o *CreateProfileOptions) { o.URL = "https://www.trulia.com/home/1" }, "url", `no lookup available for "www.trulia.com": use a listing from Zillow, Redfin, Craigslist or Facebook Marketplace`},
		{"redfin search page", func(o *CreateProfileOptions) { o.URL = "https://www.redfin.com/zipcode/02144" }, "url", "not a redfin listing page"},
		{"craigslist search page", func(o *CreateProfileOptions) { o.URL = "https://boston.craigslist.org/search/apa" }, "url", "not a craigslist posting"},
		{"facebook search page", func(o *CreateProfileOptions) { o.URL = "https://www.facebook.com/marketplace/boston/propertyrentals" }, "url", "not a facebook marketplace item"},
		{"bad id", func(o *CreateProfileOptions) { o.ID = "Bad ID" }, "id", "invalid profile id"},
		{"existing id", func(o *CreateProfileOptions) { o.ID = "attic" }, "id", "profile already exists: attic"},
		{"bad kind", func(o *CreateProfileOptions) { o.Kind = "maybe" }, "kind", "invalid kind"},
		{"negative charge", func(o *CreateProfileOptions) { o.MaxChargeUSD = -1 }, "max_charge_usd", "positive"},
	} {
		o := valid
		tc.edit(&o)
		err := svc.ValidateNewProfile(ctx, o)
		if got := fieldErrors(err)[tc.field]; !strings.Contains(got, tc.msg) {
			t.Errorf("%s: field %s = %q (err %v)", tc.name, tc.field, got, err)
		}
	}
	for _, u := range []string{
		"https://www.redfin.com/MA/Somerville/91-Heath-St-02145/home/8714201",
		"https://boston.craigslist.org/gbs/apa/d/somerville-sunny-2br/7881234567.html",
		"https://www.craigslist.org/view/d/medford-medford-tufts-2bed-in-unit/j2QRLoSwSfTzCLCkKJx9Q9",
		"https://www.facebook.com/marketplace/item/1071136835630251/?ref=search",
	} {
		o := valid
		o.URL = u
		if err := svc.ValidateNewProfile(ctx, o); err != nil {
			t.Errorf("%s: %v", u, err)
		}
	}
	forced := valid
	forced.ID, forced.Force = "attic", true
	if err := svc.ValidateNewProfile(ctx, forced); err != nil {
		t.Errorf("overwrite should allow an existing id: %v", err)
	}
	if o := (CreateProfileOptions{}).withDefaults(); o.Kind != profile.KindWant || o.Model != profile.DefaultDraftModel || o.MaxChargeUSD != DefaultLookupMaxChargeUSD {
		t.Errorf("defaults = %+v", o)
	}
}

func TestLookupFor(t *testing.T) {
	for u, want := range map[string]string{
		"https://www.zillow.com/homedetails/1-Main-St/123_zpid/":                                      "*zillow.Provider",
		"https://zillow.com/apartments/austin-tx/x/CjmTsL/":                                           "*zillow.Provider",
		"https://www.redfin.com/MA/Somerville/91-Heath-St-02145/home/8714201":                         "*redfin.Provider",
		"https://redfin.com/MA/Somerville/296-Highland-Ave-02144/unit-2/apartment/178268007":          "*redfin.Provider",
		"https://boston.craigslist.org/gbs/apa/d/somerville-sunny-2br/7881234567.html":                "*craigslist.Provider",
		"https://www.craigslist.org/view/d/medford-medford-tufts-2bed-in-unit/j2QRLoSwSfTzCLCkKJx9Q9": "*craigslist.Provider",
		"https://www.facebook.com/marketplace/item/1071136835630251/":                                 "*facebook.Provider",
		"https://m.facebook.com/marketplace/item/1071136835630251?ref=share":                          "*facebook.Provider",
	} {
		l, err := LookupFor(u, nil)
		if err != nil {
			t.Errorf("%s: %v", u, err)
			continue
		}
		if got := reflect.TypeOf(l).String(); got != want {
			t.Errorf("%s routed to %s, want %s", u, got, want)
		}
	}
	for u, want := range map[string]string{
		"https://www.trulia.com/home/1":                       `no lookup available for "www.trulia.com"`,
		"https://notredfin.com/MA/x/y/home/1":                 `no lookup available`,
		"https://www.redfin.com/city/16169/MA/Somerville":     "not a redfin listing page",
		"https://sfbay.craigslist.org/search/apa":             "not a craigslist posting",
		"https://www.facebook.com/marketplace/category/rent/": "not a facebook marketplace item",
	} {
		if _, err := LookupFor(u, nil); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", u, err, want)
		}
	}
}

func TestUpdateProfile(t *testing.T) {
	svc, attic := seedProfiles(t)
	ctx := context.Background()
	before, _ := svc.Profile(ctx, "attic")

	same, err := svc.UpdateProfile(ctx, "attic", ProfileEdit{Name: "Attic", Summary: attic.Summary, Notes: attic.Notes, Criteria: []CriterionEdit{
		{ID: "skylights", Label: "Skylights", Importance: profile.High},
		{ID: "basement", Label: "Basement", Importance: profile.Low},
	}})
	if err != nil || same.Hash() != before.Hash() {
		t.Fatalf("a no-op edit must keep the hash: %v", err)
	}

	got, err := svc.UpdateProfile(ctx, "attic", ProfileEdit{Name: " Sunny attic ", Summary: "Top floor.", Notes: []string{"quiet"}, Criteria: []CriterionEdit{
		{ID: "skylights", Label: "Big skylights", Importance: profile.Essential},
		{ID: "floors", Label: "Wood floors", Importance: profile.Medium, Remove: true},
		{ID: "basement", Label: "Basement", Importance: profile.High},
	}})
	if err != nil {
		t.Fatal(err)
	}
	stored, _ := svc.Profile(ctx, "attic")
	if !reflect.DeepEqual(got, stored) {
		t.Errorf("returned %+v\nstored %+v", got, stored)
	}
	want := before
	want.Name, want.Summary, want.Notes = "Sunny attic", "Top floor.", []string{"quiet"}
	want.Want = []profile.Criterion{before.Want[0]}
	want.Want[0].Label, want.Want[0].Importance = "Big skylights", profile.Essential
	want.Avoid = []profile.Criterion{before.Avoid[0]}
	want.Avoid[0].Importance = profile.High
	if !reflect.DeepEqual(stored, want) {
		t.Errorf("stored\n%+v\nwant\n%+v", stored, want)
	}
	if stored.Hash() == before.Hash() {
		t.Error("criterion edits must change the hash")
	}
	if d, err := svc.ProfileDetail(ctx, "attic"); err != nil || len(d.References) != 1 || d.References[0].SourceID != "ref" || len(d.References[0].Scores) != 0 {
		t.Errorf("references should survive and old grades become stale: %+v %v", d.References, err)
	}

	for _, tc := range []struct {
		name  string
		edit  CriterionEdit
		field string
	}{
		{"bad importance", CriterionEdit{ID: "skylights", Label: "x", Importance: "huge"}, "criteria.skylights.importance"},
		{"empty label", CriterionEdit{ID: "skylights", Label: "  ", Importance: profile.High}, "criteria.skylights.label"},
		{"unknown criterion", CriterionEdit{ID: "ghost", Label: "x", Importance: profile.High}, "criteria"},
	} {
		_, err := svc.UpdateProfile(ctx, "attic", ProfileEdit{Name: "x", Criteria: []CriterionEdit{tc.edit}})
		if _, ok := fieldErrors(err)[tc.field]; !ok {
			t.Errorf("%s: err = %v", tc.name, err)
		}
	}
	if after, _ := svc.Profile(ctx, "attic"); !reflect.DeepEqual(after, stored) {
		t.Error("invalid edits must not save")
	}
	if _, err := svc.UpdateProfile(ctx, "ghost", ProfileEdit{}); !errors.Is(err, profile.ErrNotFound) {
		t.Errorf("unknown profile = %v", err)
	}
}

func TestProfileDetail(t *testing.T) {
	svc, _ := seedProfiles(t)
	ctx := context.Background()
	v, err := svc.ProfileDetail(ctx, "attic")
	if err != nil {
		t.Fatal(err)
	}
	if len(v.Inherited) != 1 || v.Inherited[0].Profile != "corp" || v.Inherited[0].ProfileName != "Corporate" || v.Inherited[0].ID != "carpet" {
		t.Errorf("inherited = %+v", v.Inherited)
	}
	if len(v.References) != 1 {
		t.Fatalf("references = %+v", v.References)
	}
	r := v.References[0]
	if r.Address != "1 Attic Way, Somerville, MA 02144" || !slices.Equal(r.Collages, []string{"collages/ab/one"}) || !reflect.DeepEqual(r.Scores, map[string]float64{"test/a": 75}) {
		t.Errorf("reference = %+v", r)
	}
	if !slices.Equal(v.Collections, []string{"somerville"}) {
		t.Errorf("collections = %v", v.Collections)
	}

	corp, err := svc.ProfileDetail(ctx, "corp")
	if err != nil || len(corp.Inherited) != 0 || len(corp.References) != 0 || len(corp.Collections) != 0 {
		t.Errorf("avoid detail = %+v %v", corp, err)
	}
	if _, err := svc.ProfileDetail(ctx, "ghost"); !errors.Is(err, profile.ErrNotFound) {
		t.Errorf("unknown = %v", err)
	}
}
