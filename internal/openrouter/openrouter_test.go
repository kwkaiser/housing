package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestComplete(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" || r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("unexpected request %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		json.NewDecoder(r.Body).Decode(&got)
		w.Write([]byte(`{"id":"x","model":"m","choices":[{"finish_reason":"stop","message":{"content":"{\"ok\":true}"}}],"usage":{"prompt_tokens":10,"completion_tokens":2,"cost":0.001}}`))
	}))
	defer srv.Close()

	c := NewClient("key")
	c.BaseURL = srv.URL
	resp, err := c.Complete(context.Background(), Request{
		Model:          "m",
		Messages:       []Message{{Role: "user", Content: []Part{TextPart("hi"), ImagePart("image/jpeg", []byte{1, 2})}}},
		ResponseFormat: JSONSchemaFormat("out", json.RawMessage(`{"type":"object"}`)),
	})
	if err != nil {
		t.Fatal(err)
	}
	text, err := resp.Text()
	if err != nil || text != `{"ok":true}` || resp.Usage.Cost != 0.001 {
		t.Fatalf("got %q %v %+v", text, err, resp.Usage)
	}

	body, _ := json.Marshal(got)
	for _, want := range []string{`"data:image/jpeg;base64,AQI="`, `"json_schema"`, `"strict":true`, `"include":true`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("request missing %s: %s", want, body)
		}
	}
}

func TestCompleteError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPaymentRequired)
		w.Write([]byte(`{"error":{"message":"insufficient credits","code":402}}`))
	}))
	defer srv.Close()

	c := NewClient("key")
	c.BaseURL = srv.URL
	_, err := c.Complete(context.Background(), Request{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "insufficient credits") {
		t.Fatalf("got %v", err)
	}
}
