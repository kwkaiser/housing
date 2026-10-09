package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const inFlightBody = `{"error":{"code":402,"message":"This request would exceed your available credits given your current in-flight requests.","metadata":{"reason":"in_flight_budget_exhausted"}}}`

func TestCompleteRetriesInFlightBudget(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusPaymentRequired)
			w.Write([]byte(inFlightBody))
			return
		}
		w.Write([]byte(okBody))
	}))
	defer srv.Close()

	c := NewClient("key", WithServerURL(srv.URL), WithRetries(4, time.Millisecond, time.Millisecond))
	if _, err := c.Complete(t.Context(), Request{Model: "m", User: []Part{TextPart("hi")}}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want a retry after an in-flight 402", calls.Load())
	}
}

func TestCompleteOutOfCreditsIsTerminal(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"error":{"code":402,"message":"Insufficient credits"}}`))
	}))
	defer srv.Close()

	c := NewClient("key", WithServerURL(srv.URL), WithRetries(4, time.Millisecond, time.Millisecond))
	if _, err := c.Complete(t.Context(), Request{Model: "m", User: []Part{TextPart("hi")}}); err == nil || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d, want one failed call", err, calls.Load())
	}
}

func TestAdaptiveTransportHalvesOncePerEpoch(t *testing.T) {
	a := newAdaptiveTransport(nil, 8)
	ctx := t.Context()
	e1, _ := a.acquire(ctx)
	e2, _ := a.acquire(ctx)
	e3, _ := a.acquire(ctx)
	a.release(e1, true, false)
	a.release(e2, true, false)
	a.release(e3, true, false)
	if a.limit != 4 {
		t.Fatalf("limit = %d, want 4 after throttles from one epoch", a.limit)
	}

	e, _ := a.acquire(ctx)
	a.release(e, true, false)
	if a.limit != 2 {
		t.Fatalf("limit = %d, want 2 after a throttle from a later epoch", a.limit)
	}

	for range 2 {
		e, _ := a.acquire(ctx)
		a.release(e, false, true)
	}
	if a.limit != 3 {
		t.Fatalf("limit = %d, want 3 after a full window of successes", a.limit)
	}
}

func TestAdaptiveTransportRecoversToCeiling(t *testing.T) {
	a := newAdaptiveTransport(nil, 4)
	e, _ := a.acquire(t.Context())
	a.release(e, true, false)
	for range 100 {
		e, _ := a.acquire(t.Context())
		a.release(e, false, true)
	}
	if a.limit != 4 {
		t.Fatalf("limit = %d, want ceiling 4", a.limit)
	}
}

func TestAdaptiveTransportBlocksAtLimit(t *testing.T) {
	a := newAdaptiveTransport(nil, 1)
	e, err := a.acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if _, err := a.acquire(ctx); err == nil {
		t.Fatal("second acquire should block while the only slot is held")
	}

	got := make(chan error, 1)
	go func() {
		_, err := a.acquire(t.Context())
		got <- err
	}()
	a.release(e, false, true)
	select {
	case err := <-got:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("waiter was not woken by release")
	}
}

func TestClientCapsInFlightRequests(t *testing.T) {
	var cur, peak atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := cur.Add(1)
		defer cur.Add(-1)
		for p := peak.Load(); n > p && !peak.CompareAndSwap(p, n); p = peak.Load() {
		}
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(okBody))
	}))
	defer srv.Close()

	c := NewClient("key", WithServerURL(srv.URL), WithRateLimit(0, 1), WithMaxConcurrency(2))
	done := make(chan error)
	for range 6 {
		go func() {
			_, err := c.Complete(t.Context(), Request{Model: "m", User: []Part{TextPart("hi")}})
			done <- err
		}()
	}
	for range 6 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
	if peak.Load() > 2 {
		t.Fatalf("peak in-flight = %d, want <= 2", peak.Load())
	}
}
