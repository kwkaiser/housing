package profile

import (
	"context"
	"sync"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
)

func (s Store) ReferenceListings(ctx context.Context, p Profile) ([]listing.Listing, error) {
	own := map[string]bool{}
	for _, r := range p.References {
		if r.Profile == "" && !r.Avoid {
			own[string(r.Source)+"/"+r.SourceID] = true
		}
	}
	all, err := (jsonfile.Persister{}).Load(ctx, s.ListingsDir(p.ID))
	if err != nil {
		return nil, err
	}
	var out []listing.Listing
	for _, l := range all {
		if own[string(l.Source)+"/"+l.SourceID] {
			out = append(out, l)
		}
	}
	return out, nil
}

func (s Store) SaveReferenceListings(ctx context.Context, id string, ls []listing.Listing) error {
	return (jsonfile.Persister{}).Persist(ctx, s.ListingsDir(id), ls)
}

func ReferenceScore(p Profile, refs []listing.Listing, model string) (float64, bool) {
	var sum float64
	var n int
	for _, l := range refs {
		a, ok := l.Assessment(p.ID, model)
		if !ok || a.ProfileHash != p.Hash() || a.InputHash != InputHash(l) {
			continue
		}
		sum += a.Score
		n++
	}
	if n == 0 || sum == 0 {
		return 0, false
	}
	return sum / float64(n), true
}

func (s *BatchStats) Add(o BatchStats) {
	s.Calls += o.Calls
	s.Updated += o.Updated
	s.Cached += o.Cached
	s.NoCollages += o.NoCollages
	s.OverLimit += o.OverLimit
	s.OverBudget += o.OverBudget
	s.Failed += o.Failed
	s.CostUSD += o.CostUSD
}

type Budget struct {
	mu    sync.Mutex
	max   float64
	spent float64
}

func NewBudget(maxUSD float64) *Budget {
	return &Budget{max: maxUSD}
}

func (b *Budget) Exhausted() bool {
	if b == nil || b.max <= 0 {
		return false
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent >= b.max
}

func (b *Budget) Spend(usd float64) {
	if b == nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	b.spent += usd
}

func (b *Budget) Spent() float64 {
	if b == nil {
		return 0
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.spent
}
