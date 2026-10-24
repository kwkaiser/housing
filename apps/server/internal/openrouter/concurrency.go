package openrouter

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

func inFlightExhausted(resp *http.Response) bool {
	return resp != nil && resp.StatusCode == http.StatusPaymentRequired && resp.Header.Get("Retry-After") != ""
}

func retryPolicy(ctx context.Context, resp *http.Response, err error) (bool, error) {
	if ctx.Err() == nil && inFlightExhausted(resp) {
		return true, nil
	}
	return retryablehttp.DefaultRetryPolicy(ctx, resp, err)
}

func backoff(min, max time.Duration, attempt int, resp *http.Response) time.Duration {
	if inFlightExhausted(resp) {
		if s, err := strconv.Atoi(resp.Header.Get("Retry-After")); err == nil && s >= 0 {
			return time.Duration(s) * time.Second
		}
	}
	return retryablehttp.DefaultBackoff(min, max, attempt, resp)
}

type adaptiveTransport struct {
	next http.RoundTripper

	mu       sync.Mutex
	limit    int
	ceiling  int
	inFlight int
	streak   int
	epoch    uint64
	ready    chan struct{}
}

func newAdaptiveTransport(next http.RoundTripper, ceiling int) *adaptiveTransport {
	ceiling = max(ceiling, 1)
	return &adaptiveTransport{next: next, limit: ceiling, ceiling: ceiling, ready: make(chan struct{})}
}

func (t *adaptiveTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	epoch, err := t.acquire(req.Context())
	if err != nil {
		return nil, err
	}
	resp, err := t.next.RoundTrip(req)
	t.release(epoch, err == nil && inFlightExhausted(resp), err == nil && resp.StatusCode < http.StatusBadRequest)
	return resp, err
}

func (t *adaptiveTransport) acquire(ctx context.Context) (uint64, error) {
	for {
		t.mu.Lock()
		if t.inFlight < t.limit {
			t.inFlight++
			epoch := t.epoch
			t.mu.Unlock()
			return epoch, nil
		}
		ready := t.ready
		t.mu.Unlock()
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-ready:
		}
	}
}

func (t *adaptiveTransport) release(epoch uint64, throttled, ok bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.inFlight--
	switch {
	case throttled:
		t.streak = 0
		if epoch == t.epoch {
			t.limit = max(t.limit/2, 1)
			t.epoch++
		}
	case ok:
		t.streak++
		if t.streak >= t.limit && t.limit < t.ceiling {
			t.limit++
			t.streak = 0
		}
	}
	close(t.ready)
	t.ready = make(chan struct{})
}
