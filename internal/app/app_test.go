package app

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

func newApp(t *testing.T) (*App, *bytes.Buffer) {
	t.Helper()
	var logs bytes.Buffer
	dir := t.TempDir()
	svc := openService(t, service.Config{DataDir: dir, ProfilesDir: dir, CollectionsDir: dir})
	a, err := New(svc, jobs.New(svc.Store(), nil), slog.New(slog.NewTextHandler(&logs, nil)))
	if err != nil {
		t.Fatal(err)
	}
	return a, &logs
}

func openService(t *testing.T, cfg service.Config) *service.Service {
	t.Helper()
	svc, err := service.Open(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { svc.Close() })
	return svc
}

func get(t *testing.T, h http.Handler, target string) (*http.Response, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, target, nil))
	res := rec.Result()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func TestRoutes(t *testing.T) {
	a, logs := newApp(t)
	h := a.Handler()

	for _, tc := range []struct {
		target, contentType string
		status              int
		contains            []string
	}{
		{"/healthz", "text/plain; charset=utf-8", http.StatusOK, nil},
		{"/", "text/html; charset=utf-8", http.StatusOK, []string{
			"<!doctype html>", "<title>Listings · housing</title>", `<a href="/" aria-current="page">Listings</a>`,
			`<a href="/collections">Collections</a>`, `<a href="/profiles">Profiles</a>`, `<a href="/jobs">Jobs</a>`,
			`<a href="/collections/new">Create one</a>, then run it.`, `<link rel="stylesheet" href="/static/app.css?v=` + a.hashes["app.css"] + `">`,
		}},
		{"/nope", "text/html; charset=utf-8", http.StatusNotFound, []string{"<title>Page not found · housing</title>", "There is nothing at /nope.", `<nav>`}},
		{"/collections", "text/html; charset=utf-8", http.StatusOK, []string{`<a href="/collections" aria-current="page">`, "There are no collections yet", `href="/collections/new"`}},
		{"/collections/new", "text/html; charset=utf-8", http.StatusOK, []string{`<a href="/collections" aria-current="page">`, "There are no want profiles yet"}},
		{"/static/", "text/html; charset=utf-8", http.StatusNotFound, []string{"Page not found"}},
		{"/static/missing.css", "text/html; charset=utf-8", http.StatusNotFound, []string{"Page not found"}},
	} {
		res, body := get(t, h, tc.target)
		if res.StatusCode != tc.status || res.Header.Get("Content-Type") != tc.contentType {
			t.Errorf("GET %s = %d %q, want %d %q", tc.target, res.StatusCode, res.Header.Get("Content-Type"), tc.status, tc.contentType)
		}
		if tc.target == "/healthz" && body != "ok" {
			t.Errorf("healthz body = %q", body)
		}
		for _, s := range tc.contains {
			if !strings.Contains(body, s) {
				t.Errorf("GET %s: body missing %q:\n%s", tc.target, s, body)
			}
		}
	}
	if !strings.Contains(logs.String(), "method=GET path=/nope status=404") {
		t.Errorf("request not logged:\n%s", logs)
	}
}

func TestStatic(t *testing.T) {
	a, _ := newApp(t)
	h := a.Handler()

	res, body := get(t, h, a.staticURL("app.css"))
	if res.StatusCode != http.StatusOK || !strings.HasPrefix(res.Header.Get("Content-Type"), "text/css") || !strings.Contains(body, "--accent") {
		t.Fatalf("GET app.css = %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}
	if cc := res.Header.Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Errorf("fingerprinted asset Cache-Control = %q", cc)
	}
	if res, _ := get(t, h, "/static/app.css"); res.Header.Get("Cache-Control") != "no-cache" || res.Header.Get("ETag") == "" {
		t.Errorf("unversioned asset headers = %v", res.Header)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/static/app.css", nil)
	req.Header.Set("If-None-Match", `"`+a.hashes["app.css"]+`"`)
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Errorf("conditional GET = %d", rec.Code)
	}
}

func TestPanicRecovery(t *testing.T) {
	a, logs := newApp(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) { panic("boom") })
	mux.Handle("/", a.routes())
	srv := httptest.NewServer(a.logRequests(a.recoverPanics(mux)))
	defer srv.Close()

	res, err := http.Get(srv.URL + "/boom")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != http.StatusInternalServerError || !strings.Contains(string(body), "Something went wrong") {
		t.Errorf("GET /boom = %d\n%s", res.StatusCode, body)
	}
	if !strings.Contains(logs.String(), "panic=boom") || !strings.Contains(logs.String(), "path=/boom status=500") {
		t.Errorf("panic not logged:\n%s", logs)
	}

	res, err = http.Get(srv.URL + "/healthz")
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("server stopped serving after panic: %v", err)
	}
	res.Body.Close()
}

func TestRenderError(t *testing.T) {
	a, logs := newApp(t)
	templates, _ := fs.Sub(assets, "templates")
	fsys := fstest.MapFS{
		"pages/broken.tmpl": {Data: []byte(`{{define "title"}}Broken{{end}}{{define "content"}}partial output{{.Data.Nope}}{{end}}`)},
	}
	for _, name := range []string{"layout.tmpl", "pages/error.tmpl"} {
		b, err := fs.ReadFile(templates, name)
		if err != nil {
			t.Fatal(err)
		}
		fsys[name] = &fstest.MapFile{Data: b}
	}
	pages, err := parsePages(fsys, a.funcs())
	if err != nil {
		t.Fatal(err)
	}
	a.pages = pages

	rec := httptest.NewRecorder()
	a.render(rec, httptest.NewRequest(http.MethodGet, "/broken", nil), http.StatusOK, "broken", struct{}{})
	body := rec.Body.String()
	if rec.Code != http.StatusInternalServerError || strings.Contains(body, "partial output") || strings.Contains(body, "Broken") || !strings.Contains(body, "Something went wrong") {
		t.Errorf("render = %d\n%s", rec.Code, body)
	}
	if !strings.Contains(logs.String(), "page=broken") {
		t.Errorf("render error not logged:\n%s", logs)
	}
}

func TestServe(t *testing.T) {
	a, logs := newApp(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- a.Serve(ctx, ln) }()

	res, err := http.Get("http://" + ln.Addr().String() + "/healthz")
	if err != nil || res.StatusCode != http.StatusOK {
		t.Fatalf("healthz: %v", err)
	}
	res.Body.Close()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Serve = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after cancel")
	}
	if !strings.Contains(logs.String(), "addr="+ln.Addr().String()) || !strings.Contains(logs.String(), "msg=stopped") {
		t.Errorf("lifecycle not logged:\n%s", logs)
	}
}

func TestRunBadAddr(t *testing.T) {
	a, _ := newApp(t)
	if err := a.Run(t.Context(), "127.0.0.1:-1"); err == nil {
		t.Fatal("expected listen error")
	}
}
