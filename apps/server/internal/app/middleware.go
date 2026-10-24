package app

import (
	"net/http"
	"net/url"
	"runtime/debug"
	"strings"
	"time"
)

type recorder struct {
	http.ResponseWriter
	status int
}

func (r *recorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func (r *recorder) Write(b []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.Write(b)
}

func (r *recorder) Unwrap() http.ResponseWriter {
	return r.ResponseWriter
}

func (a *App) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &recorder{ResponseWriter: w}
		defer func() {
			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			a.log.Info("request", "method", r.Method, "path", r.URL.Path, "status", status, "duration", time.Since(start))
		}()
		next.ServeHTTP(rec, r)
	})
}

func (a *App) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rec := &recorder{ResponseWriter: w}
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if v == http.ErrAbortHandler {
				panic(v)
			}
			a.log.Error("panic", "method", r.Method, "path", r.URL.Path, "panic", v, "stack", string(debug.Stack()))
			if rec.status == 0 {
				a.renderError(rec, r, http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

func (a *App) sameOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !safeMethod(r.Method) && !sameOriginRequest(r) {
			a.log.Warn("cross-site request rejected", "method", r.Method, "path", r.URL.Path,
				"sec_fetch_site", r.Header.Get("Sec-Fetch-Site"), "origin", r.Header.Get("Origin"), "referer", r.Header.Get("Referer"))
			a.renderMessage(w, r, http.StatusForbidden, "Request blocked", "This request did not come from a page on this site, so it was not carried out.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func safeMethod(m string) bool {
	return m == http.MethodGet || m == http.MethodHead || m == http.MethodOptions
}

func sameOriginRequest(r *http.Request) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" {
		return site == "same-origin" || site == "none"
	}
	source := r.Header.Get("Origin")
	if source == "" || source == "null" {
		source = r.Header.Get("Referer")
	}
	if source == "" {
		return false
	}
	u, err := url.Parse(source)
	return err == nil && u.Host != "" && strings.EqualFold(u.Host, r.Host)
}
