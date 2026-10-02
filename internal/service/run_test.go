package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

const draftReply = `{
  "name": "Sunny attic",
  "summary": "Top floor with skylights.",
  "want": [
    {"id": "skylights", "label": "Skylights", "look_for": "roof windows", "not_this": "", "keywords": ["skylight"], "importance": "high", "evidence": "either"},
    {"id": "wood_floors", "label": "Wood floors", "look_for": "hardwood", "not_this": "", "keywords": ["hardwood"], "importance": "medium", "evidence": "either"}
  ],
  "avoid": [],
  "ignore": []
}`

const gradeReply = `{
  "want": [
    {"id": "skylights", "verdict": "present", "confidence": "high", "photos": [1], "evidence": "Roof windows."},
    {"id": "wood_floors", "verdict": "present", "confidence": "medium", "photos": [2], "evidence": "Oak boards."}
  ],
  "avoid": [],
  "summary": "Close to the reference."
}`

type fakeCompleter struct {
	drafts, grades atomic.Int32
}

func (f *fakeCompleter) Complete(_ context.Context, req openrouter.Request) (openrouter.Response, error) {
	if req.Schema != nil && req.Schema.Name == "housing_profile" {
		f.drafts.Add(1)
		return openrouter.Response{Model: "test/draft", CostUSD: 0.01, Content: draftReply}, nil
	}
	f.grades.Add(1)
	return openrouter.Response{Model: req.Model, CostUSD: 0.02, Content: gradeReply,
		Usage: openrouter.Usage{PromptTokens: 6000, CompletionTokens: 1500, ReasoningTokens: 900, CachedTokens: 2000}}, nil
}

type fakePhotos struct{ body []byte }

func (f fakePhotos) Fetch(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.body)), nil
}

type fakeProvider struct {
	source   listing.Source
	listings []listing.Listing
}

func (p fakeProvider) Source() listing.Source                                 { return p.source }
func (p fakeProvider) SupportedAmenities(listing.OfferType) []listing.Amenity { return nil }
func (p fakeProvider) Search(context.Context, listing.Query) ([]listing.Listing, error) {
	return slices.Clone(p.listings), nil
}

type fakeLookup struct{ ref listing.Listing }

func (f fakeLookup) Lookup(context.Context, string) (listing.Listing, error) { return f.ref, nil }

type logBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *logBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *logBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func (b *logBuffer) logger() *slog.Logger {
	return slog.New(slog.NewTextHandler(b, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
}

func openService(t *testing.T, cfg Config) *Service {
	t.Helper()
	svc, err := Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	return svc
}

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 40, 30))
	for i := range img.Pix {
		img.Pix[i] = 180
	}
	img.Set(0, 0, color.Black)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func rental(source listing.Source, id, street string) listing.Listing {
	beds := 2
	return listing.Listing{
		Source:   source,
		SourceID: id,
		URL:      "https://example.com/" + id,
		Offer:    listing.OfferRent,
		Price:    listing.Money{Cents: 300000, Currency: "USD"},
		Address:  listing.Address{Street: street, PostalCode: "02144", Formatted: street + ", Somerville, MA 02144"},
		Beds:     &beds,
		Photos:   []string{"https://example.com/" + id + "/1.jpg", "https://example.com/" + id + "/2.jpg"},
	}
}

func newTestService(t *testing.T, fc *fakeCompleter) *Service {
	t.Helper()
	root := t.TempDir()
	providers := map[listing.Source][]listing.Listing{
		listing.SourceZillow: {rental(listing.SourceZillow, "z1", "7 Windom St"), rental(listing.SourceZillow, "z2", "31 Fairmount Ave")},
		listing.SourceRedfin: {rental(listing.SourceRedfin, "r1", "7 Windom St")},
	}
	return openService(t, Config{
		DataDir:        root + "/data",
		ProfilesDir:    root + "/profiles",
		CollectionsDir: root + "/collections",
		Keys:           config.Config{ApifyToken: "apify", OpenRouterAPIKey: "openrouter"},
		Now:            func() time.Time { return time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC) },
		Clients: Clients{
			OpenRouter: func(string) openrouter.Completer { return fc },
			Apify:      func(string, float64) apify.Runner { return nil },
			Provider: func(source listing.Source, _ apify.Runner) (listing.Provider, error) {
				return fakeProvider{source: source, listings: providers[source]}, nil
			},
			Lookup: func(string, apify.Runner) (listing.Lookup, error) {
				return fakeLookup{ref: rental(listing.SourceZillow, "ref", "1 Attic Way")}, nil
			},
			Photos: fakePhotos{body: jpegBytes(t)},
		},
	})
}

func TestCreateAndDraftProfile(t *testing.T) {
	ctx := t.Context()
	fc := &fakeCompleter{}
	svc := newTestService(t, fc)
	var logs logBuffer
	p, err := svc.CreateProfileFromURL(ctx, CreateProfileOptions{
		URL: "https://www.zillow.com/homedetails/ref", ID: "attic", Kind: profile.KindWant, Notes: []string{"skylights"}, Model: "m",
	}, logs.logger())
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Sunny attic" || len(p.Want) != 2 || len(p.References) != 1 || len(p.References[0].Collages) == 0 || fc.drafts.Load() != 1 {
		t.Errorf("profile = %+v", p)
	}
	out := logs.String()
	for _, want := range []string{
		`msg="fetched reference" stage=profile address="1 Attic Way, Somerville, MA 02144" photos=2`,
		`msg="drafted profile" stage=profile want=2 avoid=0 model=test/draft cost_usd=0.01`,
		`msg="saved profile" stage=profile profile=attic`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}

	if _, err := svc.CreateProfileFromURL(ctx, CreateProfileOptions{URL: "x", ID: "attic", Kind: profile.KindWant}, nil); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("existing profile should be rejected: %v", err)
	}
	if _, err := svc.CreateProfileFromURL(ctx, CreateProfileOptions{URL: "x", ID: "loft", Kind: "maybe"}, nil); err == nil || !strings.Contains(err.Error(), "invalid kind") {
		t.Errorf("invalid kind should be rejected: %v", err)
	}

	redrafted, err := svc.DraftProfile(ctx, "attic", DraftOptions{Model: "m", Notes: []string{"wood floors"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := svc.Profile(ctx, "attic")
	if err != nil || !slices.Equal(stored.Notes, []string{"wood floors"}) || !slices.Equal(redrafted.Notes, stored.Notes) || fc.drafts.Load() != 2 {
		t.Errorf("redraft should replace notes and save: %+v %v", stored.Notes, err)
	}
	if _, err := svc.DraftProfile(ctx, "attic", DraftOptions{Model: "m"}, nil); err != nil {
		t.Fatal(err)
	}
	if stored, _ := svc.Profile(ctx, "attic"); !slices.Equal(stored.Notes, []string{"wood floors"}) {
		t.Errorf("nil notes should keep the stored notes: %v", stored.Notes)
	}
}

func TestRunCollection(t *testing.T) {
	ctx := t.Context()
	fc := &fakeCompleter{}
	svc := newTestService(t, fc)
	if _, err := svc.CreateProfileFromURL(ctx, CreateProfileOptions{URL: "https://www.zillow.com/homedetails/ref", ID: "attic", Kind: profile.KindWant, Model: "m"}, nil); err != nil {
		t.Fatal(err)
	}
	c := collection.Collection{
		ID: "somerville", Mode: profile.ModeRent, Profiles: []string{"attic"}, Model: "test/grader",
		Sources: []listing.Source{listing.SourceZillow, listing.SourceRedfin}, Search: profile.Search{Location: "Somerville, MA", Limit: 5},
	}
	if err := svc.SaveCollection(ctx, c); err != nil {
		t.Fatal(err)
	}

	var logs logBuffer
	res, err := svc.RunCollection(ctx, "somerville", logs.logger())
	if err != nil {
		t.Fatal(err)
	}
	if res.Day != "2026-09-30" || res.Fetched != 3 || res.Distinct != 2 || res.Duplicates != 1 || res.Collaged != 2 || res.WithoutCollages != 0 ||
		res.Stats.Updated != 3 || res.Stats.Calls != 3 || res.Stats.Failed != 0 || res.AssessErr != nil || res.BudgetReached {
		t.Errorf("result = %+v", res)
	}
	out := logs.String()
	for _, want := range []string{
		`msg=searched stage=fetch source=zillow mode=rent listings=2`,
		`msg=searched stage=fetch source=redfin mode=rent listings=1`,
		`msg="stored observations" stage=fetch listings=3 day=2026-09-30`,
		`msg=deduplicated stage=dedupe distinct=2 duplicates=1`,
		`msg="photos downloaded" stage=photos`,
		`msg=collaged stage=collage listings=2 collages=2 without_collages=0`,
		`msg="assessed profile" stage=assess profile=attic model=test/grader calls=2 cost_usd=0.04 tokens=15000 updated=2 cached=0 failed=0`,
		`msg="grading listings" stage=assess profile=attic model=test/grader batch=reference listings=1 concurrency=1`,
		`batch=reference step=1/1 listing="1 Attic Way, Somerville, MA 02144" score=100 coverage=100`,
		fmt.Sprintf(`batch=listings listings=2 concurrency=%d cached=0`, DefaultAssessConcurrency),
		`msg=grading stage=assess profile=attic model=test/grader batch=listings step=`,
		`listing="31 Fairmount Ave, Somerville, MA 02144" score=100 coverage=100`,
		`listing="7 Windom St, Somerville, MA 02144" score=100`,
		`tokens=7500 cost_usd=0.02`,
		`run_cost_usd=0.06 run_tokens=22500`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in\n%s", want, out)
		}
	}

	r, err := svc.CollectionDay(ctx, "somerville", DayOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Day != "2026-09-30" || len(r.Rows) != 2 || len(r.Rows[0].AlsoListed)+len(r.Rows[1].AlsoListed) != 1 {
		t.Errorf("report = %d rows on %s", len(r.Rows), r.Day)
	}

	again, err := svc.RunCollection(ctx, "somerville", nil)
	if err != nil {
		t.Fatal(err)
	}
	if again.Stats.Calls != 0 || again.Stats.Cached != 3 {
		t.Errorf("a second run should reuse current grades: %+v", again.Stats)
	}
}

func TestRunNeedsOpenRouterKey(t *testing.T) {
	svc := newTestService(t, &fakeCompleter{})
	svc.cfg.Keys.OpenRouterAPIKey = ""
	ctx := t.Context()
	if err := svc.SaveProfile(ctx, profile.Profile{ID: "x", Ignore: profile.DefaultIgnore}); err != nil {
		t.Fatal(err)
	}
	c := collection.Collection{ID: "c", Mode: profile.ModeRent, Profiles: []string{"x"}, Sources: []listing.Source{listing.SourceZillow}, Search: profile.Search{Location: "x", Limit: 1}}
	if err := svc.SaveCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunCollection(ctx, "c", nil); err != config.ErrMissingOpenRouterAPIKey {
		t.Errorf("got %v", err)
	}
}

func TestCollectionRunOptions(t *testing.T) {
	c := collection.Collection{ID: "c", Mode: profile.ModeRent, Profiles: []string{"x"}}
	o := CollectionRunOptions(c).Assess
	if o.Limit != 0 || o.MaxCostUSD != 0 || o.MaxRunUSD != DefaultCollectionBudgetUSD || o.Model != profile.DefaultAssessModel {
		t.Errorf("defaults = %+v", o)
	}
	c.MaxRunCostUSD, c.Model = 2.5, "m"
	if o := CollectionRunOptions(c).Assess; o.MaxRunUSD != 2.5 || o.Limit != 0 || o.MaxCostUSD != 0 || o.Model != "m" {
		t.Errorf("collection budget = %+v", o)
	}
}

func TestRunUsesHeldLock(t *testing.T) {
	fc := &fakeCompleter{}
	svc := newTestService(t, fc)
	ctx := t.Context()
	if _, err := svc.CreateProfileFromURL(ctx, CreateProfileOptions{URL: "https://www.zillow.com/homedetails/ref", ID: "attic", Kind: profile.KindWant, Model: "m"}, nil); err != nil {
		t.Fatal(err)
	}
	c := collection.Collection{ID: "c", Mode: profile.ModeRent, Profiles: []string{"attic"}, Sources: []listing.Source{listing.SourceZillow}, Search: profile.Search{Location: "x", Limit: 5}}
	if err := svc.SaveCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	held, err := Open(ctx, Config{DataDir: svc.cfg.DataDir, Exclusive: true, Clients: svc.cfg.Clients, Keys: svc.cfg.Keys, Now: svc.cfg.Now})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunCollection(ctx, "c", nil); !errors.Is(err, ErrLocked) {
		t.Errorf("another run should be locked out: %v", err)
	}
	if _, err := held.RunCollection(ctx, "c", nil); err != nil {
		t.Errorf("the lock holder should still run: %v", err)
	}
	if _, err := Open(ctx, Config{DataDir: svc.cfg.DataDir, Exclusive: true}); !errors.Is(err, ErrLocked) {
		t.Errorf("a second exclusive service should be locked out: %v", err)
	}
	held.Close()
	if _, err := svc.RunCollection(ctx, "c", nil); err != nil {
		t.Errorf("run after release: %v", err)
	}
}
