package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
)

type assessOptions struct {
	profileID string
	ids       []string
	model     string
	limit     int
	force     bool
	mode      string
}

func newAssessCmd(dataDir, profilesDir *string) *cobra.Command {
	var o assessOptions
	cmd := &cobra.Command{
		Use:   "assess",
		Short: "Grade stored listings against a profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAssess(cmd, *dataDir, *profilesDir, o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.profileID, "profile", "", "profile id")
	f.StringSliceVar(&o.ids, "id", nil, "only assess listings with these source ids (repeatable)")
	f.StringVar(&o.model, "model", profile.DefaultAssessModel, "OpenRouter model used to grade listings")
	f.IntVar(&o.limit, "limit", 5, "maximum model calls (0 for no limit)")
	f.BoolVar(&o.force, "force", false, "reassess listings that already have a current assessment")
	f.StringVar(&o.mode, "mode", "", "only assess rent or buy listings (default: both)")
	cmd.MarkFlagRequired("profile")
	return cmd
}

func runAssess(cmd *cobra.Command, dataDir, profilesDir string, o assessOptions) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	key, err := config.Load().OpenRouter()
	if err != nil {
		return err
	}
	store := profile.Store{Root: profilesDir}
	p, err := store.Effective(o.profileID)
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
	if o.mode != "" {
		m, err := profile.ParseMode(o.mode)
		if err != nil {
			return err
		}
		selected = slices.DeleteFunc(slices.Clone(selected), func(l listing.Listing) bool { return l.Offer != m.Offer() })
	}
	if len(o.ids) > 0 {
		selected = slices.DeleteFunc(slices.Clone(selected), func(l listing.Listing) bool {
			return !slices.Contains(o.ids, l.SourceID)
		})
		if len(selected) == 0 {
			return fmt.Errorf("no stored listings match --id %s", strings.Join(o.ids, ","))
		}
	}
	if len(selected) == 0 {
		return fmt.Errorf("no stored listings to assess")
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
	if err := report.Table(out, report.Build(p, assessed, report.Options{Model: o.model})); err != nil {
		return err
	}
	return assessErr
}
