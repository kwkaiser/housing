package service

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

type fakeEmbedder struct {
	calls  atomic.Int32
	images atomic.Int32
}

func (f *fakeEmbedder) Embed(_ context.Context, req openrouter.EmbedRequest) (openrouter.Embedding, error) {
	n := f.calls.Add(1)
	for _, p := range req.Parts {
		if p.Data != nil {
			f.images.Add(1)
		}
	}
	v := make([]float32, req.Dimensions)
	v[int(n)%len(v)] = 1
	return openrouter.Embedding{Model: req.Model, Vector: v, CostUSD: 0.01, Tokens: 500}, nil
}

func collectedService(t *testing.T) (*Service, *fakeEmbedder) {
	t.Helper()
	ctx := t.Context()
	svc := newTestService(t, &fakeCompleter{})
	fe := &fakeEmbedder{}
	svc.cfg.Clients.Embedder = func(string) openrouter.Embedder { return fe }
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
	if _, err := svc.RunCollection(ctx, "somerville", nil); err != nil {
		t.Fatal(err)
	}
	return svc, fe
}

func TestEmbed(t *testing.T) {
	ctx := t.Context()
	svc, fe := collectedService(t)

	o := DefaultEmbedOptions()
	o.Dimensions = 4
	o.DryRun = true
	dry, err := svc.Embed(ctx, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	if dry.Pending == 0 || dry.Collages < dry.Pending || dry.Embedded != 0 || fe.calls.Load() != 0 {
		t.Fatalf("dry run = %+v, calls = %d", dry, fe.calls.Load())
	}

	o.DryRun = false
	o.Collection = "somerville"
	var logs logBuffer
	res, err := svc.Embed(ctx, o, logs.logger())
	if err != nil {
		t.Fatal(err)
	}
	if res.Embedded != res.Pending || res.Embedded == 0 || res.Failed != 0 || res.CostUSD != 0.01*float64(res.Embedded) || res.Tokens != 500*int64(res.Embedded) {
		t.Errorf("result = %+v", res)
	}
	if int(fe.images.Load()) != res.Collages {
		t.Errorf("sent %d images, want %d collages", fe.images.Load(), res.Collages)
	}
	if !strings.Contains(logs.String(), `msg=embedded stage=embed model=google/gemini-embedding-2`) {
		t.Errorf("missing embed log in\n%s", logs.String())
	}

	again, err := svc.Embed(ctx, o, nil)
	if err != nil || again.Pending != 0 {
		t.Errorf("second run = %+v %v, want nothing pending", again, err)
	}
	ns, err := svc.db.Nearest(ctx, o.Model, []float32{0, 1, 0, 0}, 10)
	if err != nil || len(ns) != res.Embedded {
		t.Errorf("nearest = %+v %v, want %d neighbors", ns, err, res.Embedded)
	}
}

func TestEmbedStopsAtBudget(t *testing.T) {
	ctx := t.Context()
	svc, fe := collectedService(t)

	o := DefaultEmbedOptions()
	o.Dimensions = 4
	o.Concurrency = 1
	o.MaxCostUSD = 0.005
	res, err := svc.Embed(ctx, o, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Pending < 2 || res.Embedded != 1 || !res.BudgetReached || fe.calls.Load() != 1 {
		t.Errorf("result = %+v, calls = %d, want one embedding before the budget stops the run", res, fe.calls.Load())
	}
}
