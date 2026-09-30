package cli

import (
	"cmp"
	"fmt"
	"io"
	"slices"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

type assessOptions struct {
	profileID   string
	profilesDir string
	ids         []string
	model       string
	limit       int
	force       bool
}

func newAssessCmd(dataDir *string) *cobra.Command {
	var o assessOptions
	cmd := &cobra.Command{
		Use:   "assess",
		Short: "Grade stored listings against a profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAssess(cmd, *dataDir, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.profileID, "profile", "", "profile id")
	f.StringVar(&o.profilesDir, "profiles-dir", profile.DefaultRoot, "directory for profiles")
	f.StringSliceVar(&o.ids, "id", nil, "only assess listings with these source ids (repeatable)")
	f.StringVar(&o.model, "model", profile.DefaultAssessModel, "OpenRouter model used to grade listings")
	f.IntVar(&o.limit, "limit", 5, "maximum model calls (0 for no limit)")
	f.BoolVar(&o.force, "force", false, "reassess listings that already have a current assessment")
	cmd.MarkFlagRequired("profile")
	return cmd
}

func runAssess(cmd *cobra.Command, dataDir string, o assessOptions) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	key, err := config.Load().OpenRouter()
	if err != nil {
		return err
	}
	store := profile.Store{Root: o.profilesDir}
	p, err := store.Load(o.profileID)
	if err != nil {
		return err
	}
	refs, err := store.References(ctx, p)
	if err != nil {
		return err
	}

	persister := jsonfile.Persister{}
	all, err := persister.Load(ctx, dataDir)
	if err != nil {
		return err
	}
	selected := all
	if len(o.ids) > 0 {
		selected = slices.DeleteFunc(slices.Clone(all), func(l listing.Listing) bool {
			return !slices.Contains(o.ids, l.SourceID)
		})
		if len(selected) == 0 {
			return fmt.Errorf("no stored listings match --id %s", strings.Join(o.ids, ","))
		}
	}

	images := media.DiskStore{Root: dataDir}
	assessor := profile.Assessor{Client: openrouter.NewClient(key), Model: o.model}
	assessed, stats, assessErr := assessor.AssessListings(ctx, p, refs, selected,
		func(keys []string) ([][]byte, error) { return profile.ReadCollages(images, keys) },
		profile.BatchOptions{Force: o.force, Limit: o.limit},
	)
	if stats.Updated > 0 {
		if err := persister.Persist(ctx, dataDir, assessed); err != nil {
			return err
		}
	}

	fmt.Fprintf(out, "model: %s\nmodel calls: %d ($%.4f), listings updated: %d, already current: %d, without collages: %d, over --limit: %d\n\n",
		o.model, stats.Calls, stats.CostUSD, stats.Updated, stats.Cached, stats.NoCollages, stats.OverLimit)
	printAssessments(out, p, o.model, assessed)
	return assessErr
}

func printAssessments(out io.Writer, p profile.Profile, model string, listings []listing.Listing) {
	type row struct {
		l listing.Listing
		a listing.Assessment
	}
	var rows []row
	for _, l := range listings {
		if a, ok := l.Assessment(p.ID, model); ok {
			rows = append(rows, row{l, a})
		}
	}
	slices.SortFunc(rows, func(x, y row) int {
		return cmp.Or(cmp.Compare(y.a.Score, x.a.Score), cmp.Compare(x.l.SourceID, y.l.SourceID))
	})

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SCORE\tCOVERAGE\tVIBE\tMISSING ESSENTIALS\tAVOIDS HIT\tID\tADDRESS")
	for _, r := range rows {
		address := r.l.Address.Formatted
		if r.l.Address.Unit != "" {
			address += " #" + r.l.Address.Unit
		}
		fmt.Fprintf(tw, "%.1f\t%.0f%%\t%d/5\t%s\t%s\t%s\t%s\n",
			r.a.Score, r.a.Coverage, r.a.Vibe, dashIfEmpty(r.a.MissingEssentials), dashIfEmpty(r.a.AvoidsHit), r.l.SourceID, address)
	}
	tw.Flush()
}

func dashIfEmpty(ss []string) string {
	if len(ss) == 0 {
		return "-"
	}
	return strings.Join(ss, ",")
}
