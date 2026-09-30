package apify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunInputMaxItems(t *testing.T) {
	var startQuery string
	var startBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/runs"):
			startQuery = r.URL.RawQuery
			json.NewDecoder(r.Body).Decode(&startBody)
			w.Write([]byte(`{"data":{"id":"r","status":"SUCCEEDED","defaultDatasetId":"d"}}`))
		case strings.Contains(r.URL.Path, "/datasets/d/items"):
			w.Write([]byte(`[{"a":1}]`))
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	c := NewClient("tok")
	c.BaseURL = srv.URL
	items, err := c.Run(context.Background(), "a/b", RunInput{Input: map[string]any{"x": 1}, MaxItems: 7})
	if err != nil || len(items) != 1 {
		t.Fatalf("items=%v err=%v", items, err)
	}
	if !strings.Contains(startQuery, "maxItems=7") {
		t.Errorf("query = %s", startQuery)
	}
	if startBody["x"] != 1.0 || startBody["Input"] != nil {
		t.Errorf("body should be the unwrapped input: %v", startBody)
	}
}
