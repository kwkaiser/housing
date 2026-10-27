package openrouter

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func embedServer(t *testing.T, body string, got *map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/embeddings") || r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("unexpected request %s %s", r.URL.Path, r.Header.Get("Authorization"))
		}
		if got != nil {
			json.NewDecoder(r.Body).Decode(got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestEmbed(t *testing.T) {
	var got map[string]any
	srv := embedServer(t, `{"object":"list","model":"m","data":[{"object":"embedding","index":0,"embedding":[0.5,-0.25,1]}],"usage":{"prompt_tokens":300,"total_tokens":300,"cost":0.0002}}`, &got)

	c := NewClient("key", WithServerURL(srv.URL))
	e, err := c.Embed(t.Context(), EmbedRequest{
		Model:      "m",
		Parts:      []Part{TextPart("sunny"), ImagePart("image/jpeg", []byte{1, 2})},
		Dimensions: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if e.Model != "m" || e.CostUSD != 0.0002 || e.Tokens != 300 {
		t.Errorf("got %+v", e)
	}
	if len(e.Vector) != 3 || e.Vector[0] != 0.5 || e.Vector[1] != -0.25 || e.Vector[2] != 1 {
		t.Errorf("vector = %v", e.Vector)
	}

	body, _ := json.Marshal(got)
	for _, want := range []string{`"dimensions":3`, `"encoding_format":"float"`, `"data:image/jpeg;base64,AQI="`, `"text":"sunny"`, `"input":[{"content":[`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("request missing %s: %s", want, body)
		}
	}
}

func TestEmbedDimensionMismatch(t *testing.T) {
	srv := embedServer(t, `{"object":"list","model":"m","data":[{"object":"embedding","index":0,"embedding":[0.5,1]}]}`, nil)
	c := NewClient("key", WithServerURL(srv.URL))
	if _, err := c.Embed(t.Context(), EmbedRequest{Model: "m", Parts: []Part{TextPart("x")}, Dimensions: 3}); err == nil {
		t.Error("a vector with the wrong dimensions should be rejected")
	}
}

func TestEmbedEmpty(t *testing.T) {
	srv := embedServer(t, `{"object":"list","model":"m","data":[]}`, nil)
	c := NewClient("key", WithServerURL(srv.URL))
	if _, err := c.Embed(t.Context(), EmbedRequest{Model: "m", Parts: []Part{TextPart("x")}}); !errors.Is(err, ErrEmptyEmbedding) {
		t.Errorf("err = %v, want ErrEmptyEmbedding", err)
	}
}
