package cli

import (
	"fmt"
	"path/filepath"
	"strings"

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
			"assessed listing for the profile and mode. Each stage saves its results before the next starts.\n\n" +
			"With --profile repeated, the first profile's saved search is used, listings are graded against every\n" +
			"profile, and each listing is ranked by its best match.\n\n" +
			"The report is printed and also written as a sortable HTML table to <data-dir>/reports/<profiles>-<mode>.html\n" +
			"unless --output is given.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if len(fetch.profileIDs) == 0 {
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
				_, _, stats, err := env.Assess(ctx, assess.options(fetch.profileIDs, fetchOpts.Mode, nil), collaged)
				if err != nil {
					if ctx.Err() != nil || stats.Updated == 0 && stats.Failed == 0 {
						return err
					}
					fmt.Fprintf(env.Out, "warning: %d listings could not be assessed; continuing to the report\n%v\n", stats.Failed, err)
				}
			}

			reportOpts := rep.options(fetch.profileIDs, fetchOpts.Mode)
			reportOpts.Report.Model = assess.model
			if reportOpts.Output != "" {
				return env.Report(ctx, reportOpts)
			}
			if err := env.Report(ctx, reportOpts); err != nil {
				return err
			}
			reportOpts.Format = "html"
			reportOpts.Output = filepath.Join(env.DataDir, "reports", fmt.Sprintf("%s-%s.html", strings.Join(fetch.profileIDs, "+"), fetchOpts.Mode))
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
