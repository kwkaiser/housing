package apify

import (
	"context"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

type slowRunner struct {
	live, peak atomic.Int32
}

func (s *slowRunner) Run(context.Context, string, any) ([]json.RawMessage, error) {
	n := s.live.Add(1)
	defer s.live.Add(-1)
	for p := s.peak.Load(); n > p && !s.peak.CompareAndSwap(p, n); p = s.peak.Load() {
	}
	time.Sleep(10 * time.Millisecond)
	return nil, nil
}

func TestLimit(t *testing.T) {
	inner := &slowRunner{}
	r := Limit(inner, 2)
	var g errgroup.Group
	for range 6 {
		g.Go(func() error {
			_, err := r.Run(t.Context(), "a/b", nil)
			return err
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
	if inner.peak.Load() != 2 {
		t.Errorf("peak concurrency = %d, want 2", inner.peak.Load())
	}
}
