package app

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRoutes(t *testing.T) {
	a, logs := newApp(t)
	h := a.Handler()

	for _, tc := range []struct {
		target, contentType string
		status              int
		body                string
	}{
		{"/healthz", "text/plain; charset=utf-8", http.StatusOK, "ok"},
		{"/health", "text/plain; charset=utf-8", http.StatusOK, "ok"},
		{"/version", "text/plain; charset=utf-8", http.StatusOK, "dev"},
		{"/", "text/plain; charset=utf-8", http.StatusNotFound, "Not Found\n"},
		{"/map", "text/plain; charset=utf-8", http.StatusNotFound, "Not Found\n"},
		{"/static/app.css", "text/plain; charset=utf-8", http.StatusNotFound, "Not Found\n"},
	} {
		res, body := get(t, h, tc.target)
		if res.StatusCode != tc.status || res.Header.Get("Content-Type") != tc.contentType || body != tc.body {
			t.Errorf("GET %s = %d %q %q, want %d %q %q", tc.target, res.StatusCode, res.Header.Get("Content-Type"), body, tc.status, tc.contentType, tc.body)
		}
	}
	if !strings.Contains(logs.String(), "method=GET path=/map status=404") {
		t.Errorf("request not logged:\n%s", logs)
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
	if res.StatusCode != http.StatusInternalServerError || string(body) != "Internal Server Error\n" {
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
