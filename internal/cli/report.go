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
	fs.StringVar(&r.format, "format", "table", "table, markdown or json")
	fs.StringVarP(&r.output, "output", "o", "", "write the report to a file instead of stdout")
	fs.BoolVar(&r.opts.IncludeStale, "include-stale", false, "include assessments made against an older profile or listing")
	fs.Float64Var(&r.opts.MinScore, "min-score", 0, "omit listings scoring below this")
	fs.IntVar(&r.opts.Top, "top", 0, "show only the top N listings (0 for all)")
	if withModel {
		fs.StringVar(&r.opts.Model, "model", "", "rank by a single grading model (default: average of all models)")
	}
}

func (r *reportFlags) options(profileID string, mode profile.Mode) pipeline.ReportOptions {
	opts := r.opts
	if mode != "" {
		opts.Offer = mode.Offer()
	}
	return pipeline.ReportOptions{ProfileID: profileID, Format: r.format, Output: r.output, Report: opts}
}

func newReportCmd(dataDir, profilesDir *string) *cobra.Command {
	var r reportFlags
	var profileID, mode string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Rank assessed listings for a profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var m profile.Mode
			if mode != "" {
				var err error
				if m, err = profile.ParseMode(mode); err != nil {
					return err
				}
			}
			return newEnv(cmd, *dataDir, *profilesDir).Report(cmd.Context(), r.options(profileID, m))
		},
	}
	f := cmd.Flags()
	r.register(f, true)
	f.StringVar(&profileID, "profile", "", "profile id")
	f.StringVar(&mode, "mode", "", "only report rent or buy listings (default: both)")
	cmd.MarkFlagRequired("profile")
	return cmd
}
