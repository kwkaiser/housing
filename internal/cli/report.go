package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"git.kwkaiser.io/kwkaiser/housing/internal/pipeline"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
)

type reportFlags struct {
	format string
	output string
	opts   report.Options
}

func (r *reportFlags) register(fs *pflag.FlagSet, withModel bool) {
	fs.StringVar(&r.format, "format", "table", "table, html or json")
	fs.StringVarP(&r.output, "output", "o", "", "write the report to a file instead of stdout")
	fs.BoolVar(&r.opts.IncludeStale, "include-stale", false, "include assessments made against an older profile or listing")
	fs.BoolVar(&r.opts.Dealbreakers, "include-dealbreakers", false, "include listings that hit a dealbreaker")
	fs.Float64Var(&r.opts.MinScore, "min-score", 0, "omit listings whose best match is below this")
	fs.IntVar(&r.opts.Top, "top", 0, "show only the top N listings (0 for all)")
	if withModel {
		fs.StringVar(&r.opts.Model, "model", "", "rank by a single grading model (default: average of all models)")
	}
}

func (r *reportFlags) options(profileIDs []string, mode profile.Mode) pipeline.ReportOptions {
	opts := r.opts
	if mode != "" {
		opts.Offer = mode.Offer()
	}
	return pipeline.ReportOptions{ProfileIDs: profileIDs, Format: r.format, Output: r.output, Report: opts}
}

func newReportCmd(dataDir, profilesDir, collectionsDir *string) *cobra.Command {
	var r reportFlags
	var profileIDs []string
	var mode, day, collectionID string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Rank assessed listings against one or more profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sc, err := resolveScope(cmd, *collectionsDir, collectionID, profileIDs, mode)
			if err != nil {
				return err
			}
			if c := sc.collection; c != nil && !cmd.Flags().Changed("model") && c.Model != "" {
				r.opts.Model = c.Model
			}
			opts := r.options(sc.profileIDs, sc.mode)
			opts.Day = day
			opts.Collection = sc.collectionID()
			return newEnv(cmd, *dataDir, *profilesDir).Report(cmd.Context(), opts)
		},
	}
	f := cmd.Flags()
	r.register(f, true)
	f.StringSliceVar(&profileIDs, "profile", nil, "profile ids to rank against; listings rank by their best match (repeatable)")
	f.StringVar(&mode, "mode", "", "only report rent or buy listings (default: both)")
	f.StringVar(&day, "day", "", "report on listings observed on this day, YYYY-MM-DD (default: the latest day fetched)")
	f.StringVar(&collectionID, "collection", "", "report on this collection's listings against its profiles")
	return cmd
}
