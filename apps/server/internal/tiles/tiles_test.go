package tiles

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestParse(t *testing.T) {
	for _, tc := range []struct {
		z, x, file, ext string
		want            Tile
		ok              bool
	}{
		{"14", "4958", "6060.png", ".png", Tile{Z: 14, X: 4958, Y: 6060}, true},
		{"14", "4958", "6060@2x.png", ".png", Tile{Z: 14, X: 4958, Y: 6060, Retina: true}, true},
		{"14", "4958", "6060.json", ".json", Tile{Z: 14, X: 4958, Y: 6060}, true},
		{"14", "4958", "6060@2x.json", ".json", Tile{}, false},
		{"0", "0", "0.png", ".png", Tile{}, true},
		{"1", "2", "0.png", ".png", Tile{}, false},
		{"21", "0", "0.png", ".png", Tile{}, false},
		{"-1", "0", "0.png", ".png", Tile{}, false},
		{"3", "0", "0.jpg", ".png", Tile{}, false},
		{"3", "0", "../0.png", ".png", Tile{}, false},
		{"3", "x", "0.png", ".png", Tile{}, false},
	} {
		got, err := Parse(tc.z, tc.x, tc.file, tc.ext)
		if tc.ok && (err != nil || got != tc.want) || !tc.ok && !errors.Is(err, ErrBadTile) {
			t.Errorf("Parse(%s, %s, %s, %s) = %+v, %v", tc.z, tc.x, tc.file, tc.ext, got, err)
		}
	}
}

type upstream struct {
	hits    atomic.Int32
	status  int
	etag    string
	ctype   string
	body    string
	lastURL atomic.Value
	lastKey atomic.Value
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.hits.Add(1)
	u.lastURL.Store(r.URL.String())
	u.lastKey.Store(r.Header.Get("apikey"))
	if u.etag != "" {
		w.Header().Set("ETag", u.etag)
	}
	w.Header().Set("Content-Type", u.ctype)
	w.WriteHeader(u.status)
	w.Write([]byte(u.body))
}

func serve(t *testing.T, c *Cache, u *upstream) *time.Time {
	t.Helper()
	srv := httptest.NewServer(u)
	t.Cleanup(srv.Close)
	now := time.Now()
	c.Now = func() time.Time { return now }
	c.Upstream = func(_ context.Context, tl Tile) (string, error) {
		return srv.URL + "/tiles/" + filepath.Base(c.path(tl)), nil
	}
	return &now
}

func TestGet(t *testing.T) {
	u := &upstream{status: http.StatusOK, ctype: "image/png", body: "v1"}
	c := NewPositron(t.TempDir(), "s3cret key")
	now := serve(t, c, u)
	tile := Tile{Z: 3, X: 1, Y: 2, Retina: true}
	ctx := t.Context()

	b, err := c.Get(ctx, tile)
	if err != nil || string(b) != "v1" {
		t.Fatalf("Get = %q, %v", b, err)
	}
	if got := u.lastURL.Load().(string); !strings.Contains(got, "key=s3cret+key") || !strings.HasSuffix(strings.SplitN(got, "?", 2)[0], "/2@2x.png") {
		t.Errorf("upstream url = %s", got)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "3", "1", "2@2x.png")); err != nil {
		t.Errorf("tile not cached: %v", err)
	}

	u.body = "v2"
	if b, _ := c.Get(ctx, tile); string(b) != "v1" || u.hits.Load() != 1 {
		t.Errorf("fresh Get = %q after %d hits, want cached v1 after 1", b, u.hits.Load())
	}

	*now = now.Add(DefaultTTL + time.Hour)
	if b, _ := c.Get(ctx, tile); string(b) != "v2" || u.hits.Load() != 2 {
		t.Errorf("expired Get = %q after %d hits, want v2 after 2", b, u.hits.Load())
	}

	*now = now.Add(DefaultTTL + time.Hour)
	u.status = http.StatusInternalServerError
	if b, err := c.Get(ctx, tile); err != nil || string(b) != "v2" {
		t.Errorf("Get with failing upstream = %q, %v; want stale v2", b, err)
	}
}

func TestGetErrors(t *testing.T) {
	tile := Tile{Z: 1, X: 1, Y: 1}
	for name, u := range map[string]*upstream{
		"status":      {status: http.StatusForbidden, ctype: "image/png"},
		"placeholder": {status: http.StatusOK, ctype: "image/png", etag: `"wm-da89c20e77c1-light"`, body: "API KEY REQUIRED"},
		"not image":   {status: http.StatusOK, ctype: "text/html", body: "<html>"},
	} {
		c := NewPositron(t.TempDir(), "s3cret key")
		serve(t, c, u)
		_, err := c.Get(t.Context(), tile)
		if !errors.Is(err, ErrUpstream) {
			t.Errorf("%s: Get err = %v, want ErrUpstream", name, err)
		}
		if err != nil && strings.Contains(err.Error(), "s3cret") {
			t.Errorf("%s: error leaks key: %v", name, err)
		}
		if _, statErr := os.Stat(c.path(tile)); statErr == nil {
			t.Errorf("%s: failed tile was cached", name)
		}
	}

	c := NewPositron(t.TempDir(), "s3cret key")
	c.Upstream = func(context.Context, Tile) (string, error) { return "http://127.0.0.1:1/tile.png", nil }
	if _, err := c.Get(t.Context(), tile); !errors.Is(err, ErrUpstream) || strings.Contains(err.Error(), "s3cret") {
		t.Errorf("unreachable upstream err = %v", err)
	}

	if _, err := NewPositron(t.TempDir(), "").Get(t.Context(), tile); !errors.Is(err, ErrNoKey) {
		t.Errorf("Get without key err = %v, want ErrNoKey", err)
	}
	if _, err := NewTransitland(t.TempDir(), "").Get(t.Context(), tile); !errors.Is(err, ErrNoKey) {
		t.Errorf("Transitland without key err = %v, want ErrNoKey", err)
	}
	if _, err := NewTransitland(t.TempDir(), "k").Get(t.Context(), Tile{Z: 15}); !errors.Is(err, ErrBadTile) {
		t.Errorf("Transitland z15 err = %v, want ErrBadTile", err)
	}
}

func TestTransitland(t *testing.T) {
	u := &upstream{status: http.StatusOK, ctype: "application/vnd.mapbox-vector-tile", body: "pbf"}
	c := NewTransitland(t.TempDir(), "tl-key")
	serve(t, c, u)
	b, err := c.Get(t.Context(), Tile{Z: 14, X: 4958, Y: 6060})
	if err != nil || string(b) != "pbf" {
		t.Fatalf("Get = %q, %v", b, err)
	}
	if got := u.lastKey.Load().(string); got != "tl-key" {
		t.Errorf("apikey header = %q", got)
	}
	if got := u.lastURL.Load().(string); strings.Contains(got, "tl-key") {
		t.Errorf("key leaked into url %s", got)
	}

	u.status = http.StatusNoContent
	tile := Tile{Z: 14, X: 1, Y: 1}
	if b, err := c.Get(t.Context(), tile); err != nil || len(b) != 0 {
		t.Errorf("empty tile = %q, %v", b, err)
	}
	hits := u.hits.Load()
	if _, err := c.Get(t.Context(), tile); err != nil || u.hits.Load() != hits {
		t.Errorf("empty tile not cached: %v after %d hits", err, u.hits.Load())
	}

	u.status = http.StatusNotFound
	if _, err := c.Get(t.Context(), Tile{Z: 14, X: 3, Y: 3}); !errors.Is(err, ErrUpstream) {
		t.Errorf("404 err = %v, want ErrUpstream", err)
	}
	if _, err := os.Stat(c.path(Tile{Z: 14, X: 3, Y: 3})); err == nil {
		t.Error("404 tile was cached")
	}

	u.status, u.ctype = http.StatusOK, "text/html"
	if _, err := c.Get(t.Context(), Tile{Z: 14, X: 2, Y: 2}); !errors.Is(err, ErrUpstream) {
		t.Errorf("html response err = %v, want ErrUpstream", err)
	}
}

func TestOpenFreeMap(t *testing.T) {
	var tileHits, jsonHits atomic.Int32
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()
	mux.HandleFunc("/planet", func(w http.ResponseWriter, r *http.Request) {
		jsonHits.Add(1)
		fmt.Fprintf(w, `{"tiles":["%s/planet/v1/{z}/{x}/{y}.pbf"]}`, srv.URL)
	})
	mux.HandleFunc("/planet/v1/", func(w http.ResponseWriter, r *http.Request) {
		tileHits.Add(1)
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Write([]byte(r.URL.Path))
	})
	c := newOpenFreeMap(t.TempDir(), srv.URL+"/planet")
	if !c.Enabled() {
		t.Fatal("OpenFreeMap should not need a key")
	}
	for i, tile := range []Tile{{Z: 14, X: 1, Y: 2}, {Z: 14, X: 3, Y: 4}} {
		b, err := c.Get(t.Context(), tile)
		want := fmt.Sprintf("/planet/v1/14/%d/%d.pbf", tile.X, tile.Y)
		if err != nil || string(b) != want {
			t.Errorf("Get %d = %q, %v; want %q", i, b, err, want)
		}
	}
	if jsonHits.Load() != 1 || tileHits.Load() != 2 {
		t.Errorf("tilejson hits = %d, tile hits = %d; want 1 and 2", jsonHits.Load(), tileHits.Load())
	}
}

func TestTerrain(t *testing.T) {
	u := &upstream{status: http.StatusOK, ctype: "image/webp", body: "dem"}
	c := NewTerrain(t.TempDir())
	if !c.Enabled() {
		t.Fatal("terrain should not need a key")
	}
	serve(t, c, u)
	if b, err := c.Get(t.Context(), Tile{Z: 12, X: 1205, Y: 1540}); err != nil || string(b) != "dem" {
		t.Fatalf("Get = %q, %v", b, err)
	}
	if _, err := os.Stat(filepath.Join(c.Dir, "12", "1205", "1540.webp")); err != nil {
		t.Errorf("tile not cached: %v", err)
	}
	if _, err := c.Get(t.Context(), Tile{Z: TerrainMaxZoom + 1}); !errors.Is(err, ErrBadTile) {
		t.Errorf("past max zoom err = %v, want ErrBadTile", err)
	}
	u.ctype = "text/html"
	if _, err := c.Get(t.Context(), Tile{Z: 12, X: 1, Y: 1}); !errors.Is(err, ErrUpstream) {
		t.Errorf("html response err = %v, want ErrUpstream", err)
	}
}
