package pipeline

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"time"
)

type ServeOptions struct {
	Addr  string
	Dir   string
	Ready func(url string)
}

func (e *Env) Serve(ctx context.Context, o ServeOptions) error {
	dir := o.Dir
	if dir == "" {
		tmp, err := os.MkdirTemp("", "housing-site-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(tmp)
		if _, err := e.Publish(ctx, tmp); err != nil {
			return err
		}
		dir = tmp
	} else if _, err := os.Stat(dir); err != nil {
		return err
	}

	ln, err := net.Listen("tcp", o.Addr)
	if err != nil {
		return err
	}
	files := http.FileServer(http.Dir(dir))
	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-store")
			files.ServeHTTP(w, r)
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()

	url := "http://" + ln.Addr().String() + "/"
	e.printf("serving %s at %s (Ctrl-C to stop)\n", dir, url)
	if o.Ready != nil {
		o.Ready(url)
	}

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdown); err != nil {
		return err
	}
	if err := <-done; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
