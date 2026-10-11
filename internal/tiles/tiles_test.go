package tiles

import (
	"errors"
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
		z, x, file string
		want       Tile
		ok         bool
	}{
		{"14", "4958", "6060.png", Tile{Z: 14, X: 4958, Y: 6060}, true},
		{"14", "4958", "6060@2x.png", Tile{Z: 14, X: 4958, Y: 6060, Retina: true}, true},
		{"0", "0", "0.png", Tile{}, true},
		{"1", "2", "0.png", Tile{}, false},
		{"21", "0", "0.png", Tile{}, false},
		{"-1", "0", "0.png", Tile{}, false},
		{"3", "0", "0.jpg", Tile{}, false},
		{"3", "0", "../0.png", Tile{}, false},
		{"3", "x", "0.png", Tile{}, false},
	} {
		got, err := Parse(tc.z, tc.x, tc.file)
		if tc.ok && (err != nil || got != tc.want) || !tc.ok && !errors.Is(err, ErrBadTile) {
			t.Errorf("Parse(%s, %s, %s) = %+v, %v", tc.z, tc.x, tc.file, got, err)
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
}

func (u *upstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	u.hits.Add(1)
	u.lastURL.Store(r.URL.String())
	if u.etag != "" {
		w.Header().Set("ETag", u.etag)
	}
	w.Header().Set("Content-Type", u.ctype)
	w.WriteHeader(u.status)
	w.Write([]byte(u.body))
}

func newCache(t *testing.T, u *upstream) (*Cache, *time.Time) {
	t.Helper()
	srv := httptest.NewServer(u)
	t.Cleanup(srv.Close)
	now := time.Now()
	c := New(t.TempDir(), "s3cret key")
	c.Now = func() time.Time { return now }
	c.Upstream = func(tl Tile) string { return srv.URL + "/light_all/" + filepath.Base(c.path(tl)) }
	return c, &now
}

func TestGet(t *testing.T) {
	u := &upstream{status: http.StatusOK, ctype: "image/png", body: "v1"}
	c, now := newCache(t, u)
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
		c, _ := newCache(t, u)
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

	c, _ := newCache(t, &upstream{})
	c.Upstream = func(Tile) string { return "http://127.0.0.1:1/tile.png" }
	if _, err := c.Get(t.Context(), tile); !errors.Is(err, ErrUpstream) || strings.Contains(err.Error(), "s3cret") {
		t.Errorf("unreachable upstream err = %v", err)
	}

	if _, err := New(t.TempDir(), "").Get(t.Context(), tile); !errors.Is(err, ErrNoKey) {
		t.Errorf("Get without key err = %v, want ErrNoKey", err)
	}
}
