package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
	"git.kwkaiser.io/kwkaiser/housing/internal/tiles"
)

const DefaultShutdownTimeout = 10 * time.Second

type Jobs interface {
	Enqueue(ctx context.Context, p jobs.Params) (int64, error)
	Job(ctx context.Context, id int64) (jobs.Job, error)
	Jobs(ctx context.Context, f jobs.Filter) ([]jobs.Job, error)
	Events(ctx context.Context, id int64) ([]jobs.Event, error)
	ActiveForCollection(ctx context.Context, collectionID string) (jobs.Job, bool, error)
	LatestForCollection(ctx context.Context, collectionID string) (jobs.Job, bool, error)
}

type App struct {
	ShutdownTimeout time.Duration
	Version         string
	NoAuth          bool

	svc      *service.Service
	jobs     Jobs
	now      func() time.Time
	log      *slog.Logger
	positron *tiles.Cache
	terrain  *tiles.Cache
	overlays []overlay
	handler  http.Handler

	streamPoll time.Duration
}

func New(svc *service.Service, q Jobs, log *slog.Logger) (*App, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	a := &App{ShutdownTimeout: DefaultShutdownTimeout, Version: "dev", svc: svc, jobs: q, now: time.Now, log: log, streamPoll: defaultStreamPoll}
	tileDir := filepath.Join(svc.DataDir(), "tiles")
	a.positron = tiles.NewPositron(filepath.Join(tileDir, "positron"), svc.CartoAPIKey())
	a.terrain = tiles.NewTerrain(filepath.Join(tileDir, "terrain"))
	a.overlays = newOverlays(tiles.NewTransitland(filepath.Join(tileDir, "transitland"), svc.TransitlandKey()), tiles.NewOpenFreeMap(filepath.Join(tileDir, "openfreemap")))
	web := a.routes()
	api := a.etags(a.apiHandler())
	a.handler = a.logRequests(a.recoverPanics(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, apiPrefix+"/") {
			api.ServeHTTP(w, r)
			return
		}
		web.ServeHTTP(w, r)
	})))
	return a, nil
}

func (a *App) Handler() http.Handler {
	return a.handler
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.healthz)
	mux.HandleFunc("GET /health", a.healthz)
	mux.HandleFunc("GET /version", a.version)
	mux.HandleFunc("/", a.notFound)
	return mux
}

func (a *App) healthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if _, err := a.svc.Store().SchemaVersion(r.Context()); err != nil {
		a.log.Error("health check", "err", err)
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, "unavailable")
		return
	}
	fmt.Fprint(w, "ok")
}

func (a *App) version(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, a.Version)
}

func (a *App) notFound(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
}

func (a *App) Run(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	return a.Serve(ctx, ln)
}

func (a *App) Serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           a.handler,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(a.log.Handler(), slog.LevelWarn),
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()
	a.log.Info("listening", "addr", ln.Addr().String())

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}

	a.log.Info("shutting down", "timeout", a.ShutdownTimeout)
	sctx, cancel := context.WithTimeout(context.Background(), a.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(sctx); err != nil {
		srv.Close()
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	a.log.Info("stopped")
	return nil
}
