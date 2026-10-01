package app

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"html/template"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

//go:embed templates static
var assets embed.FS

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

	svc     *service.Service
	jobs    Jobs
	now     func() time.Time
	log     *slog.Logger
	static  fs.FS
	hashes  map[string]string
	pages   map[string]*template.Template
	handler http.Handler
}

func New(svc *service.Service, q Jobs, log *slog.Logger) (*App, error) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	templates, err := fs.Sub(assets, "templates")
	if err != nil {
		return nil, err
	}
	static, err := fs.Sub(assets, "static")
	if err != nil {
		return nil, err
	}
	a := &App{ShutdownTimeout: DefaultShutdownTimeout, svc: svc, jobs: q, now: time.Now, log: log, static: static}
	if a.hashes, err = fingerprint(static); err != nil {
		return nil, fmt.Errorf("static assets: %w", err)
	}
	if a.pages, err = parsePages(templates, a.funcs()); err != nil {
		return nil, fmt.Errorf("templates: %w", err)
	}
	a.handler = a.logRequests(a.recoverPanics(a.sameOrigin(a.routes())))
	return a, nil
}

func (a *App) Handler() http.Handler {
	return a.handler
}

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.home)
	mux.HandleFunc("GET /healthz", a.healthz)
	mux.HandleFunc("GET /static/{path...}", a.serveStatic)
	mux.HandleFunc("GET /c/{collection}", a.collectionPage)
	mux.HandleFunc("GET /listing/{source}/{id}", a.listingPage)
	mux.HandleFunc("GET /media/{key...}", a.media)
	mux.HandleFunc("GET /collections", a.collectionsPage)
	mux.HandleFunc("GET /collections/new", a.newCollectionPage)
	mux.HandleFunc("GET /collections/{id}/edit", a.editCollectionPage)
	mux.HandleFunc("POST /collections", a.createCollection)
	mux.HandleFunc("POST /collections/{id}", a.updateCollection)
	mux.HandleFunc("GET /profiles", a.profilesPage)
	mux.HandleFunc("GET /profiles/new", a.newProfilePage)
	mux.HandleFunc("GET /profiles/{id}", a.profilePage)
	mux.HandleFunc("GET /profiles/{id}/edit", a.editProfilePage)
	mux.HandleFunc("POST /profiles", a.createProfile)
	mux.HandleFunc("POST /profiles/{id}", a.updateProfile)
	mux.HandleFunc("POST /profiles/{id}/draft", a.draftProfile)
	mux.HandleFunc("GET /jobs", a.jobsPage)
	mux.HandleFunc("GET /jobs/{id}", a.jobPage)
	mux.HandleFunc("POST /jobs/run", a.runCollection)
	mux.HandleFunc("/", a.notFound)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/media/") && path.Clean(r.URL.Path) != r.URL.Path {
			a.notFound(w, r)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

func (a *App) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	fmt.Fprint(w, "ok")
}

func (a *App) notFound(w http.ResponseWriter, r *http.Request) {
	a.renderError(w, r, http.StatusNotFound)
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
