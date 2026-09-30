package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"git.kwkaiser.io/kwkaiser/housing/internal/report"
)

type ReportOptions struct {
	ProfileID string
	Format    string
	Output    string
	Report    report.Options
}

var renderers = map[string]func(io.Writer, report.Report) error{
	"table":    report.Table,
	"markdown": report.Markdown,
	"json":     report.JSON,
}

func ValidFormat(format string) error {
	if _, ok := renderers[format]; !ok {
		return fmt.Errorf("invalid format %q: use table, markdown or json", format)
	}
	return nil
}

func (e *Env) Report(ctx context.Context, o ReportOptions) error {
	if err := ValidFormat(o.Format); err != nil {
		return err
	}
	p, err := e.profiles().Effective(o.ProfileID)
	if err != nil {
		return err
	}
	listings, err := e.persister().Load(ctx, e.DataDir)
	if err != nil {
		return err
	}
	r := report.Build(p, listings, o.Report)
	if len(r.Rows) == 0 && r.Stale == 0 {
		return fmt.Errorf("no listings in %s have been assessed against %q; run `housing assess --profile %s` first", e.DataDir, p.ID, p.ID)
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
