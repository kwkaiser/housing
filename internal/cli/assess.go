package cli

import (
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"git.kwkaiser.io/kwkaiser/housing/internal/pipeline"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
)

type assessFlags struct {
	model       string
	limit       int
	concurrency int
	maxCost     float64
	force       bool
}

func (a *assessFlags) register(fs *pflag.FlagSet, limitName string) {
	fs.StringVar(&a.model, "model", profile.DefaultAssessModel, "OpenRouter model used to grade listings")
	fs.IntVar(&a.limit, limitName, 20, "maximum model calls per profile (0 for no limit)")
	fs.IntVar(&a.concurrency, "concurrency", pipeline.DefaultAssessConcurrency, "concurrent model calls")
	fs.Float64Var(&a.maxCost, "max-cost-usd", 1, "stop starting model calls for a profile once this much has been spent on it (0 for no cap)")
	fs.BoolVar(&a.force, "force", false, "reassess listings that already have a current assessment")
}

func (a *assessFlags) options(profileIDs []string, mode profile.Mode, ids []string) pipeline.AssessOptions {
	return pipeline.AssessOptions{
		ProfileIDs:  profileIDs,
		Model:       a.model,
		Mode:        mode,
		IDs:         ids,
		Limit:       a.limit,
		Concurrency: a.concurrency,
		MaxCostUSD:  a.maxCost,
		Force:       a.force,
	}
}

func newAssessCmd(dataDir, profilesDir *string) *cobra.Command {
	var a assessFlags
	var profileIDs, ids []string
	var mode string
	cmd := &cobra.Command{
		Use:   "assess",
		Short: "Grade stored listings against one or more profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			var m profile.Mode
			if mode != "" {
				var err error
				if m, err = profile.ParseMode(mode); err != nil {
					return err
				}
			}
			env := newEnv(cmd, *dataDir, *profilesDir)
			templates, assessed, _, err := env.Assess(cmd.Context(), a.options(profileIDs, m, ids), nil)
			if assessed != nil {
				cmd.Println()
				if terr := report.Table(cmd.OutOrStdout(), report.Build(templates, assessed, report.Options{Model: a.model})); terr != nil && err == nil {
					err = terr
				}
			}
			return err
		},
	}
	f := cmd.Flags()
	a.register(f, "limit")
	f.StringSliceVar(&profileIDs, "profile", nil, "profile ids to grade against (repeatable)")
	f.StringSliceVar(&ids, "id", nil, "only assess listings with these source ids (repeatable)")
	f.StringVar(&mode, "mode", "", "only assess rent or buy listings (default: both)")
	cmd.MarkFlagRequired("profile")
	return cmd
}
