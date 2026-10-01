package app

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
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
		Score: score, Coverage: 90, Vibe: 4, Summary: "summary of " + l.SourceID, Dealbreakers: dealbreakers,
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
	ctx := context.Background()
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

	a, err := New(service.New(service.Config{DataDir: dir}), openQueue(t, dir), slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	return a
}

var rowAddress = regexp.MustCompile(`data-col="address"><a href="/listing/[^"]*">([^<]*)</a>`)

func addresses(body string) []string {
	var out []string
	for _, m := range rowAddress.FindAllStringSubmatch(body, -1) {
		out = append(out, strings.SplitN(m[1], ",", 2)[0])
	}
	return out
}

func TestHomeRedirect(t *testing.T) {
	a := seededApp(t)
	res, _ := get(t, a.Handler(), "/")
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/c/somerville" {
		t.Errorf("GET / = %d %q", res.StatusCode, res.Header.Get("Location"))
	}
}

func TestCollectionPage(t *testing.T) {
	a := seededApp(t)
	h := a.Handler()

	for _, tc := range []struct {
		target   string
		order    []string
		contains []string
		missing  []string
	}{
		{"/c/somerville", []string{"7 Windom St", "5 New St", "31 Fairmount Ave"}, []string{
			"<title>Listings · somerville · 2026-09-30 · housing</title>",
			`<a href="/" aria-current="page">Listings</a>`,
			"4 of 5 listings graded · 1 hidden by dealbreakers",
			`<span class="badge down">↓ $100</span>`,
			`5 New St, Somerville, MA 02144</a> <a class="ext" href="https://example.com/z4" target="_blank" rel="noopener" aria-label="Open on zillow">↗</a><span class="badge new">new</span>`,
			`<td data-col="match" class="num score">88%</td>`,
			`<td data-col="p:loft" class="num">30.0</td>`, `<td data-col="p:loft" class="num">-</td>`,
			`<a href="https://example.com/z1" target="_blank" rel="noopener">zillow</a>, <a href="https://example.com/r1" target="_blank" rel="noopener">redfin</a>`,
			`href="/c/somerville?day=2026-09-29" rel="prev"`, `<span class="button" aria-disabled="true">→</span>`,
			`min="2026-09-29" max="2026-09-30"`,
			`<a class="button" href="/c/somerville?dealbreakers=1">Show dealbreakers</a>`,
			`<th data-col="price" class="num"><a href="/c/somerville?dir=asc&amp;sort=price">Price</a></th>`,
			`<th data-col="dealbreakers" class="tags bad off">`,
			`<a href="/c/zempty">zempty</a>`, `<a href="/c/somerville" aria-current="page">somerville</a>`,
			"Sunny attic", "Skylights", "dealbreaker</span>", "model 80.0",
			`/static/listings.js?v=`,
		}, []string{"9 Carpet Ct", "No run on", "&lt;script&gt;"}},
		{"/c/somerville?dealbreakers=1", []string{"7 Windom St", "5 New St", "31 Fairmount Ave", "9 Carpet Ct"}, []string{
			"4 of 5 listings graded", `<td data-col="dealbreakers" class="tags bad off">corp.carpet</td>`, "Hide dealbreakers",
		}, []string{"hidden by dealbreakers"}},
		{"/c/somerville?sort=price&dir=asc", []string{"5 New St", "7 Windom St", "31 Fairmount Ave"}, []string{
			`aria-sort="ascending"><a href="/c/somerville?dir=desc&amp;sort=price">`,
			`<input type="hidden" name="sort" value="price"><input type="hidden" name="dir" value="asc">`,
		}, nil},
		{"/c/somerville?sort=price&dir=desc", []string{"31 Fairmount Ave", "7 Windom St", "5 New St"}, nil, nil},
		{"/c/somerville?sort=bogus", []string{"7 Windom St", "5 New St", "31 Fairmount Ave"}, nil, []string{"aria-sort"}},
		{"/c/somerville?day=2026-09-29", []string{"7 Windom St", "31 Fairmount Ave"}, []string{
			"3 of 3 listings graded · 1 hidden by dealbreakers", `href="/c/somerville?day=2026-09-30" rel="next"`, `<span class="badge new">new</span>`,
		}, []string{"badge down", `rel="prev"`}},
		{"/c/somerville?day=2026-10-15", []string{"7 Windom St", "5 New St", "31 Fairmount Ave"}, []string{
			"No run on 2026-10-15; showing 2026-09-30.",
		}, nil},
		{"/c/zempty", nil, []string{"No listings have been fetched"}, []string{"<table class=\"listings\""}},
	} {
		res, body := get(t, h, tc.target)
		if res.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d\n%s", tc.target, res.StatusCode, body)
			continue
		}
		if got := addresses(body); strings.Join(got, "|") != strings.Join(tc.order, "|") {
			t.Errorf("GET %s order = %q, want %q", tc.target, got, tc.order)
		}
		for _, s := range tc.contains {
			if !strings.Contains(body, s) {
				t.Errorf("GET %s: missing %q", tc.target, s)
			}
		}
		for _, s := range tc.missing {
			if strings.Contains(body, s) {
				t.Errorf("GET %s: unexpected %q", tc.target, s)
			}
		}
	}
}

func TestListingPage(t *testing.T) {
	a := seededApp(t)
	h := a.Handler()

	res, body := get(t, h, "/listing/zillow/z1?collection=somerville")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d\n%s", res.StatusCode, body)
	}
	for _, s := range []string{
		"<title>7 Windom St, Somerville, MA 02144 · housing</title>",
		`<a href="/c/somerville">← somerville · 2026-09-30</a>`,
		"$2,900/mo", `<a href="https://example.com/r1" target="_blank" rel="noopener">redfin</a>`,
		"Sunny attic", `Match <span class="score">88%</span>`, "Loft",
		`<tr><td>Skylights</td><td class="tags">high</td><td class="verdict present">present</td><td class="tags">high</td><td class="summary">seen skylights</td><td class="num">1, 2</td></tr>`,
		`<td>Wall-to-wall carpet</td><td class="tags"><span class="bad">dealbreaker</span></td>`,
		`<img src="/media/collages/ab/abcdef"`,
		`<tr><td>2026-09-29</td><td class="num">$3,000/mo</td></tr>`, `<tr><td>2026-09-30</td><td class="num">$2,900/mo</td></tr>`,
	} {
		if !strings.Contains(body, s) {
			t.Errorf("detail missing %q", s)
		}
	}

	if !strings.Contains(body, `<details class="section criteria-section">`) || !strings.Contains(body, "<summary>Assessment summary</summary>") || strings.Contains(body, "<details class=\"section criteria-section\" open") {
		t.Errorf("criteria and the assessment summary should be collapsed by default")
	}
	if strings.Index(body, `<div class="collages">`) > strings.Index(body, `<section class="grade">`) {
		t.Errorf("photos should come before the grades")
	}

	_, body = get(t, h, "/listing/zillow/z2")
	if strings.Contains(body, "<script>alert") || !strings.Contains(body, "&lt;script&gt;alert(1)&lt;/script&gt; sunny") {
		t.Errorf("description not escaped")
	}
	if !strings.Contains(body, "Sunny attic") || strings.Contains(body, "Loft") {
		t.Errorf("profiles without collection context should come from assessments")
	}

	for _, target := range []string{"/c/nope", "/listing/zillow/nope", "/listing/zillow/z1?collection=nope"} {
		if res, _ := get(t, h, target); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, res.StatusCode)
		}
	}
}

func TestMedia(t *testing.T) {
	a := seededApp(t)
	h := a.Handler()

	res, body := get(t, h, "/media/"+collageKey)
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/jpeg" || len(body) == 0 {
		t.Fatalf("GET collage = %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("Cache-Control = %q", cc)
	}
	for _, target := range []string{
		"/media/../x", "/media/photos/../../etc/passwd", "/media/photos/..%2f..%2fetc%2fpasswd", "/media/collages/../collages/ab/abcdef",
		"/media/store.sqlite3", "/media/housing.sqlite3", "/media/other/ab/abcdef", "/media/photos/ab/missing.jpg",
		"/media/photos/ab/page.html", "/media/collages/ab", "/media/",
	} {
		if res, _ := get(t, h, target); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, res.StatusCode)
		}
	}
}
