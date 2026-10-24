package app

import (
	"bytes"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

const (
	day1       = "2026-09-29"
	day2       = "2026-09-30"
	model      = "test/model"
	collageKey = "collages/ab/abcdef"
)

func rental(source listing.Source, id, street string, cents int64) listing.Listing {
	beds := 2
	return listing.Listing{
		Source:   source,
		SourceID: id,
		URL:      "https://example.com/" + id,
		Offer:    listing.OfferRent,
		Price:    listing.Money{Cents: cents, Currency: "USD"},
		Address:  listing.Address{Street: street, PostalCode: "02144", Formatted: street + ", Somerville, MA 02144"},
		Beds:     &beds,
	}
}

func assessed(l listing.Listing, p profile.Profile, score float64, dealbreakers ...string) listing.Listing {
	a := listing.Assessment{
		ProfileHash: p.Hash(), InputHash: profile.InputHash(l), Model: model,
		Score: score, Coverage: 90, Summary: "summary of " + l.SourceID, Dealbreakers: dealbreakers,
	}
	for _, c := range p.Want {
		a.Want = append(a.Want, listing.CriterionResult{ID: c.ID, Verdict: listing.VerdictPresent, Confidence: listing.ConfidenceHigh, Photos: []int{1, 2}, Evidence: "seen " + c.ID})
	}
	for _, c := range p.Avoid {
		a.Avoid = append(a.Avoid, listing.CriterionResult{ID: c.ID, Verdict: listing.VerdictAbsent, Confidence: listing.ConfidenceMedium, Evidence: "none"})
	}
	return l.WithAssessment(p.ID, a)
}

func seededApp(t *testing.T) *App {
	t.Helper()
	ctx := t.Context()
	dir := t.TempDir()
	db, err := store.Open(ctx, filepath.Join(dir, store.FileName))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	criterion := func(id, label string, imp profile.Importance) profile.Criterion {
		return profile.Criterion{ID: id, Label: label, LookFor: label, Importance: imp, Evidence: profile.EvidenceEither}
	}
	ref := rental(listing.SourceZillow, "ref", "1 Reference Rd", 400000)
	profiles := []profile.Profile{
		{ID: "corp", Kind: profile.KindAvoid, Name: "Corporate", Avoid: []profile.Criterion{criterion("carpet", "Wall-to-wall carpet", profile.Essential)}},
		{ID: "attic", Name: "Sunny attic", Summary: "Top floor with skylights.",
			Want:       []profile.Criterion{criterion("skylights", "Skylights", profile.High), criterion("floors", "Wood floors", profile.Medium)},
			References: []profile.Reference{{Source: ref.Source, SourceID: ref.SourceID, URL: ref.URL}}},
		{ID: "loft", Name: "Loft", Want: []profile.Criterion{criterion("open", "Open plan", profile.High)}},
	}
	for _, p := range profiles {
		var refs []listing.Listing
		if len(p.References) > 0 {
			refs = append(refs, ref)
		}
		if err := db.SaveProfile(ctx, p, refs...); err != nil {
			t.Fatal(err)
		}
	}
	attic, err := db.EffectiveProfile(ctx, "attic")
	if err != nil {
		t.Fatal(err)
	}
	loft, err := db.EffectiveProfile(ctx, "loft")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Save(ctx, []listing.Listing{assessed(ref, attic, 80)}); err != nil {
		t.Fatal(err)
	}
	for _, c := range []collection.Collection{
		{ID: "somerville", Mode: profile.ModeRent, Sources: []listing.Source{listing.SourceZillow}, Profiles: []string{"attic", "loft"}, Search: profile.Search{Location: "Somerville, MA"}},
		{ID: "zempty", Mode: profile.ModeRent, Sources: []listing.Source{listing.SourceZillow}, Profiles: []string{"attic"}, Search: profile.Search{Location: "Boston, MA"}},
	} {
		if err := db.SaveCollection(ctx, c); err != nil {
			t.Fatal(err)
		}
	}

	a1 := rental(listing.SourceZillow, "z1", "7 Windom St", 300000)
	a1.Collages = []string{collageKey}
	b := rental(listing.SourceZillow, "z2", "31 Fairmount Ave", 310000)
	b.Description = "<script>alert(1)</script> sunny"
	c := rental(listing.SourceZillow, "z3", "9 Carpet Ct", 200000)
	d := rental(listing.SourceZillow, "z4", "5 New St", 250000)
	dup := rental(listing.SourceRedfin, "r1", "7 Windom St", 290000)

	grade := func(l listing.Listing) listing.Listing {
		switch l.SourceID {
		case "z1":
			return assessed(assessed(l, attic, 70), loft, 30)
		case "z2":
			return assessed(l, attic, 48)
		case "z3":
			return assessed(l, attic, 90, "corp.carpet")
		case "z4":
			return assessed(l, attic, 60)
		}
		return l
	}
	a2 := a1
	a2.Price.Cents = 290000
	for day, ls := range map[string][]listing.Listing{day1: {a1, b, c}, day2: {a2, b, c, d, dup}} {
		for i := range ls {
			ls[i] = grade(ls[i])
		}
		if err := db.Observe(ctx, day, "somerville", ls); err != nil {
			t.Fatal(err)
		}
	}

	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	for key, data := range map[string][]byte{collageKey: buf.Bytes(), "photos/ab/page.html": []byte("<html><script>x</script>")} {
		p := filepath.Join(dir, filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	svc := openService(t, service.Config{DataDir: dir})
	a, err := New(svc, jobs.New(svc.Store(), nil), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func newApp(t *testing.T) (*App, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	dir := t.TempDir()
	svc := openService(t, service.Config{DataDir: dir, ProfilesDir: dir, CollectionsDir: dir})
	a, err := New(svc, jobs.New(svc.Store(), nil), slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return a, &logs
}

func openService(t *testing.T, cfg service.Config) *service.Service {
	t.Helper()
	svc, err := service.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	return svc
}

func get(t *testing.T, h http.Handler, target string) (*http.Response, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	res := rec.Result()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}
