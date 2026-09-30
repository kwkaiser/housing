package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
)

type reportOptions struct {
	profileID string
	format    string
	output    string
	mode      string
	opts      report.Options
}

func newReportCmd(dataDir, profilesDir *string) *cobra.Command {
	var o reportOptions
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Rank assessed listings for a profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runReport(cmd, *dataDir, *profilesDir, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.profileID, "profile", "", "profile id")
	f.StringVar(&o.format, "format", "table", "table, markdown or json")
	f.StringVarP(&o.output, "output", "o", "", "write the report to a file instead of stdout")
	f.StringVar(&o.mode, "mode", "", "only report rent or buy listings (default: both)")
	f.StringVar(&o.opts.Model, "model", "", "rank by a single grading model (default: average of all models)")
	f.BoolVar(&o.opts.IncludeStale, "include-stale", false, "include assessments made against an older profile or listing")
	f.Float64Var(&o.opts.MinScore, "min-score", 0, "omit listings scoring below this")
	f.IntVar(&o.opts.Top, "top", 0, "show only the top N listings (0 for all)")
	cmd.MarkFlagRequired("profile")
	return cmd
}

func runReport(cmd *cobra.Command, dataDir, profilesDir string, o reportOptions) error {
	render, ok := map[string]func(io.Writer, report.Report) error{
		"table":    report.Table,
		"markdown": report.Markdown,
		"json":     report.JSON,
	}[o.format]
	if !ok {
		return fmt.Errorf("invalid --format %q", o.format)
	}

	if o.mode != "" {
		m, err := profile.ParseMode(o.mode)
		if err != nil {
			return err
		}
		o.opts.Offer = m.Offer()
	}

	p, err := profile.Store{Root: profilesDir}.Effective(o.profileID)
	if err != nil {
		return err
	}
	listings, err := (jsonfile.Persister{}).Load(cmd.Context(), dataDir)
	if err != nil {
		return err
	}
	r := report.Build(p, listings, o.opts)
	if len(r.Rows) == 0 && r.Stale == 0 {
		return fmt.Errorf("no listings in %s have been assessed against %q; run `housing assess --profile %s` first", dataDir, p.ID, p.ID)
	}

	if o.output == "" {
		return render(cmd.OutOrStdout(), r)
	}
	if err := os.MkdirAll(filepath.Dir(o.output), 0o755); err != nil {
		return err
	}
	f, err := os.Create(o.output)
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
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %d listings to %s\n", len(r.Rows), o.output)
	return nil
}
