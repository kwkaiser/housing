package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const okBody = `{"id":"x","model":"m","object":"chat.completion","created":1,"choices":[{"index":0,"finish_reason":"stop","message":{"role":"assistant","content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"cost":0.001}}`

func TestComplete(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") || r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("unexpected request %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(okBody))
	}))
	defer srv.Close()

	temp := 0.0
	c := NewClient("key", WithServerURL(srv.URL))
	resp, err := c.Complete(context.Background(), Request{
		Model:       "m",
		System:      "be terse",
		User:        []Part{TextPart("hi"), ImagePart("image/jpeg", []byte{1, 2})},
		Schema:      &Schema{Name: "out", Schema: map[string]any{"type": "object"}},
		Temperature: &temp,
	})
	if err != nil {
		t.Fatal(err)
	}
	text, err := resp.Text()
	if err != nil || text != `{"ok":true}` || resp.CostUSD != 0.001 || resp.Model != "m" {
		t.Fatalf("got %q %v %+v", text, err, resp)
	}

	body, _ := json.Marshal(got)
	for _, want := range []string{`"data:image/jpeg;base64,AQI="`, `"json_schema"`, `"strict":true`, `"be terse"`, `"role":"system"`, `"temperature":0`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("request missing %s: %s", want, body)
		}
	}
}

func TestCompleteRetriesServerErrors(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(okBody))
	}))
	defer srv.Close()

	c := NewClient("key", WithServerURL(srv.URL), WithRetries(4, time.Millisecond, time.Millisecond))
	if _, err := c.Complete(context.Background(), Request{Model: "m", User: []Part{TextPart("hi")}}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 3 {
		t.Fatalf("calls = %d", calls.Load())
	}
}

func TestCompleteRetriesRateLimits(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(okBody))
	}))
	defer srv.Close()

	c := NewClient("key", WithServerURL(srv.URL), WithRetries(4, time.Millisecond, time.Millisecond))
	if _, err := c.Complete(context.Background(), Request{Model: "m", User: []Part{TextPart("hi")}}); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 2 {
		t.Fatalf("calls = %d, want a retry after 429", calls.Load())
	}
}

func TestRateLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(okBody))
	}))
	defer srv.Close()

	c := NewClient("key", WithServerURL(srv.URL), WithRateLimit(20, 1))
	start := time.Now()
	for range 3 {
		if _, err := c.Complete(context.Background(), Request{Model: "m", User: []Part{TextPart("hi")}}); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed < 90*time.Millisecond {
		t.Errorf("3 calls at 20/s with burst 1 took %v, want >= 100ms", elapsed)
	}
}

func TestCompleteClientError(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":{"message":"bad schema","code":400}}`))
	}))
	defer srv.Close()

	c := NewClient("key", WithServerURL(srv.URL))
	_, err := c.Complete(context.Background(), Request{Model: "m", User: []Part{TextPart("hi")}})
	if err == nil || calls.Load() != 1 {
		t.Fatalf("err=%v calls=%d", err, calls.Load())
	}
	if !strings.Contains(err.Error(), "bad schema") {
		t.Errorf("error should surface the API message: %v", err)
	}
}
