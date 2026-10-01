package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/report"
)

type ReportOptions struct {
	Collection string
	ProfileIDs []string
	Day        string
	Format     string
	Output     string
	Report     report.Options
}

var renderers = map[string]func(io.Writer, report.Report) error{
	"table": report.Table,
	"html":  report.HTML,
	"json":  report.JSON,
}

func ValidFormat(format string) error {
	if _, ok := renderers[format]; !ok {
		return fmt.Errorf("invalid format %q: use table, html or json", format)
	}
	return nil
}

func (e *Env) Report(ctx context.Context, o ReportOptions) error {
	if err := ValidFormat(o.Format); err != nil {
		return err
	}
	templates, err := e.templates(ctx, o.ProfileIDs)
	if err != nil {
		return err
	}
	db, err := e.openStore(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	day := o.Day
	if day == "" {
		if day, err = db.LatestDay(ctx, o.Collection); err != nil {
			return err
		}
		if day == "" {
			return noListings(o.Collection)
		}
	}
	listings, err := db.LoadDay(ctx, day, o.Collection)
	if err != nil {
		return err
	}
	r := report.Build(templates, listings, o.Report)
	r.Day = day
	if len(r.Rows) == 0 && r.Stale == 0 && r.Dealbreakers == 0 {
		ids := strings.Join(o.ProfileIDs, ",")
		return fmt.Errorf("no listings observed on %s have been assessed against %s; run `housing assess --profile %s` first", day, ids, ids)
	}

	render := renderers[o.Format]
	if o.Output == "" {
		e.mu.Lock()
		defer e.mu.Unlock()
		return render(e.Out, r)
	}
	if err := os.MkdirAll(filepath.Dir(o.Output), 0o755); err != nil {
		return err
	}
	f, err := os.Create(o.Output)
	if err != nil {
		return err
	}
	if err := render(f, r); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	e.printf("report: wrote %d listings to %s\n", len(r.Rows), o.Output)
	return nil
}
