package media

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func jpegBytes(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, solid(60, 40, color.RGBA{G: 200, A: 255}), nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type server struct {
	*httptest.Server
	hits atomic.Int64
}

func newServer(t *testing.T) *server {
	body := jpegBytes(t)
	s := &server{}
	s.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.hits.Add(1)
		if strings.Contains(r.URL.Path, "missing") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write(body)
	}))
	t.Cleanup(s.Close)
	return s
}

func newTestProcessor(t *testing.T) (*Processor, DiskStore) {
	store := DiskStore{Root: t.TempDir()}
	g := NewGrid()
	g.CellSize = 32
	p := NewProcessor(NewHTTPFetcher(), store, g)
	p.Logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	return p, store
}

func photoURLs(base string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = base + "/p" + string(rune('a'+i)) + ".jpg"
	}
	return out
}

func TestProcess(t *testing.T) {
	srv := newServer(t)
	p, store := newTestProcessor(t)

	shared := photoURLs(srv.URL, 20)
	listings := []listing.Listing{
		{Source: listing.SourceZillow, SourceID: "unit1", Photos: shared},
		{Source: listing.SourceZillow, SourceID: "unit2", Photos: shared},
		{Source: listing.SourceZillow, SourceID: "home", Photos: []string{srv.URL + "/solo.jpg", srv.URL + "/missing.jpg"}},
		{Source: listing.SourceZillow, SourceID: "none", Photos: []string{srv.URL + "/missing2.jpg"}},
	}

	got, err := p.Process(context.Background(), listings)
	if err != nil {
		t.Fatal(err)
	}

	if hits := srv.hits.Load(); hits != 18+1+1+1 {
		t.Errorf("got %d fetches, want 18 shared + solo + 2 missing", hits)
	}
	if len(got[0].Collages) != 2 {
		t.Errorf("18 photos should make 2 collages, got %v", got[0].Collages)
	}
	if strings.Join(got[0].Collages, ",") != strings.Join(got[1].Collages, ",") {
		t.Error("listings with identical photos should share collages")
	}
	if len(got[2].Collages) != 1 {
		t.Errorf("missing photo should be skipped, got %v", got[2].Collages)
	}
	if got[3].Collages != nil {
		t.Errorf("listing with no fetchable photos should have no collages, got %v", got[3].Collages)
	}
	if listings[0].Collages != nil {
		t.Error("Process should not mutate its input")
	}

	for _, key := range append(got[0].Collages, PhotoKey(shared[0])) {
		f, err := os.Open(store.Path(key))
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := image.Decode(f); err != nil {
			t.Errorf("%s: %v", key, err)
		}
		f.Close()
	}
	if ok, _ := store.Has(context.Background(), PhotoKey(shared[18])); ok {
		t.Error("photos beyond MaxPhotos should not be fetched")
	}

	before := srv.hits.Load()
	info, _ := os.Stat(store.Path(got[0].Collages[0]))
	again, err := p.Process(context.Background(), listings)
	if err != nil {
		t.Fatal(err)
	}
	if hits := srv.hits.Load() - before; hits != 2 {
		t.Errorf("second run should only retry the 2 missing photos, got %d fetches", hits)
	}
	info2, _ := os.Stat(store.Path(again[0].Collages[0]))
	if !info.ModTime().Equal(info2.ModTime()) {
		t.Error("second run should reuse existing collages")
	}
}

func TestCollageIsOffline(t *testing.T) {
	srv := newServer(t)
	p, store := newTestProcessor(t)
	urls := photoURLs(srv.URL, 3)
	listings := []listing.Listing{{Source: listing.SourceZillow, SourceID: "a", Photos: urls}}

	got, err := p.Collage(context.Background(), listings)
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Collages != nil || srv.hits.Load() != 0 {
		t.Fatalf("collage without fetched photos should do nothing, got %v with %d fetches", got[0].Collages, srv.hits.Load())
	}

	if err := p.FetchPhotos(context.Background(), listings); err != nil {
		t.Fatal(err)
	}
	fetched := srv.hits.Load()
	got, err = p.Collage(context.Background(), listings)
	if err != nil {
		t.Fatal(err)
	}
	if len(got[0].Collages) != 1 || srv.hits.Load() != fetched {
		t.Fatalf("collage should use stored photos only, got %v", got[0].Collages)
	}

	info, _ := os.Stat(store.Path(got[0].Collages[0]))
	time.Sleep(10 * time.Millisecond)
	p.Force = true
	if _, err := p.Collage(context.Background(), listings); err != nil {
		t.Fatal(err)
	}
	info2, _ := os.Stat(store.Path(got[0].Collages[0]))
	if info.ModTime().Equal(info2.ModTime()) {
		t.Error("Force should rebuild existing collages")
	}
}

func TestHTTPFetcherRetries(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	f := NewHTTPFetcher()
	f.Client.RetryWaitMin, f.Client.RetryWaitMax = time.Millisecond, time.Millisecond
	body, err := f.Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	body.Close()
	if calls.Load() != 3 {
		t.Errorf("got %d calls", calls.Load())
	}
}

func TestHTTPFetcherRejectsNonImage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte("<html>"))
	}))
	defer srv.Close()
	if _, err := NewHTTPFetcher().Fetch(context.Background(), srv.URL); err == nil {
		t.Fatal("expected error for non-image response")
	}
}
