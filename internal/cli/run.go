package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/pipeline"
)

func newRunCmd(dataDir, profilesDir, collectionsDir *string) *cobra.Command {
	var (
		fetch        fetchFlags
		collage      collageFlags
		assess       assessFlags
		rep          reportFlags
		collectionID string
	)
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Fetch, collage, assess and report in one pass",
		Long: "Run the whole pipeline: fetch listings from every source in parallel, download photos, build\n" +
			"collages, grade the fetched listings, then report on the day's graded listings. Each stage saves\n" +
			"its results before the next starts, and listings already graded are not graded again.\n\n" +
			"With --collection, the collection's search, sources, mode, profiles, model and run budget are used;\n" +
			"flags given explicitly override them. Without it, --profile is required and the first profile's\n" +
			"saved search is used. Each listing is ranked by its best match across the profiles.\n\n" +
			"Only one run may use a data directory at a time, so overlapping cron jobs fail fast.\n\n" +
			"The report is printed and also written as a sortable HTML table to <data-dir>/reports/<name>-<mode>.html\n" +
			"unless --output is given.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			changed := cmd.Flags().Changed
			if err := pipeline.ValidFormat(rep.format); err != nil {
				return err
			}
			env := newEnv(cmd, *dataDir, *profilesDir)

			var c *collection.Collection
			profileIDs := fetch.profileIDs
			name := strings.Join(profileIDs, "+")
			if collectionID != "" {
				loaded, err := collection.Store{Root: *collectionsDir}.Load(collectionID)
				if err != nil {
					return err
				}
				c, name = &loaded, loaded.ID
				if !changed("profile") {
					profileIDs = c.Profiles
				}
				if !changed("model") && c.Model != "" {
					assess.model = c.Model
				}
				if !changed("max-run-cost-usd") {
					assess.maxRunCost = c.MaxRunCostUSD
				}
			}
			if len(profileIDs) == 0 {
				return fmt.Errorf("--profile or --collection is required")
			}

			fetchOpts, err := fetch.options(cmd, env, c)
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

			unlock, err := env.Lock()
			if err != nil {
				return err
			}
			defer unlock()

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
				assessOpts := assess.options(profileIDs, fetchOpts.Mode, nil)
				assessOpts.Collection = fetchOpts.Collection
				_, _, stats, err := env.Assess(ctx, assessOpts, collaged)
				if err != nil {
					if ctx.Err() != nil || stats.Updated == 0 && stats.Failed == 0 {
						return err
					}
					fmt.Fprintf(env.Out, "warning: %d listings could not be assessed; continuing to the report\n%v\n", stats.Failed, err)
				}
			}

			reportOpts := rep.options(profileIDs, fetchOpts.Mode)
			reportOpts.Collection = fetchOpts.Collection
			reportOpts.Report.Model = assess.model
			if reportOpts.Output != "" {
				return env.Report(ctx, reportOpts)
			}
			if err := env.Report(ctx, reportOpts); err != nil {
				return err
			}
			reportOpts.Format = "html"
			reportOpts.Output = filepath.Join(env.DataDir, "reports", fmt.Sprintf("%s-%s.html", name, fetchOpts.Mode))
			return env.Report(ctx, reportOpts)
		},
	}
	f := cmd.Flags()
	fetch.register(f)
	collage.register(f, "force-collage", false)
	assess.register(f, "assess-limit")
	rep.register(f, false)
	f.StringVar(&collectionID, "collection", "", "run this collection")
	return cmd
}
