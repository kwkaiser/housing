package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/pipeline"
)

func newRunCmd(dataDir, profilesDir *string) *cobra.Command {
	var (
		fetch   fetchFlags
		collage collageFlags
		assess  assessFlags
		rep     reportFlags
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Fetch, collage, assess and report in one pass",
		Long: "Run the whole pipeline for a profile's saved search: fetch listings from every source in parallel,\n" +
			"download photos, build collages, grade the fetched listings in parallel, then report on every\n" +
			"assessed listing for the profile and mode. Each stage saves its results before the next starts.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if fetch.profileID == "" {
				return fmt.Errorf("--profile is required")
			}
			if err := pipeline.ValidFormat(rep.format); err != nil {
				return err
			}
			env := newEnv(cmd, *dataDir, *profilesDir)

			fetchOpts, err := fetch.options(cmd, env)
			if err != nil {
				return err
			}
			collageOpts, err := collage.options()
			if err != nil {
				return err
			}
			if _, err := env.Config.OpenRouter(); err != nil {
				return err
			}

			fetched, err := env.Fetch(ctx, fetchOpts)
			if err != nil {
				return err
			}
			if len(fetched) == 0 {
				fmt.Fprintln(env.Out, "no listings matched; nothing to assess")
			} else {
				collaged, err := env.Collage(ctx, collageOpts, fetched)
				if err != nil {
					return err
				}
				if _, _, _, err := env.Assess(ctx, assess.options(fetch.profileID, fetchOpts.Mode, nil), collaged); err != nil {
					return err
				}
			}

			reportOpts := rep.options(fetch.profileID, fetchOpts.Mode)
			reportOpts.Report.Model = assess.model
			return env.Report(ctx, reportOpts)
		},
	}
	f := cmd.Flags()
	fetch.register(f)
	collage.register(f, "force-collage", false)
	assess.register(f, "assess-limit")
	rep.register(f, false)
	cmd.MarkFlagRequired("profile")
	return cmd
}
