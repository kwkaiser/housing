package profile

import (
	"os"
	"sync"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
)

func ReadCollages(store media.DiskStore, keys []string) ([][]byte, error) {
	out := make([][]byte, 0, len(keys))
	for _, key := range keys {
		b, err := os.ReadFile(store.Path(key))
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, nil
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
