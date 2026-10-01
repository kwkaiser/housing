package cli

import (
	"errors"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/pipeline"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

type fetchFlags struct {
	search           searchFlags
	sources          []string
	mode             string
	profileIDs       []string
	enrich           bool
	photos           bool
	maxCharge        float64
	apifyConcurrency int
}

func (f *fetchFlags) register(fs *pflag.FlagSet) {
	f.search.register(fs)
	fs.StringSliceVar(&f.sources, "source", []string{string(listing.SourceZillow)}, "listing sources to search in parallel (repeatable)")
	fs.StringVar(&f.mode, "mode", string(profile.ModeRent), "rent or buy")
	fs.StringSliceVar(&f.profileIDs, "profile", nil, "use this profile's saved search for --mode (the first, if repeated)")
	fs.BoolVar(&f.enrich, "enrich", true, "fetch listing details (splits buildings into units)")
	fs.BoolVar(&f.photos, "photos", true, "download listing photos")
	fs.Float64Var(&f.maxCharge, "max-charge", 1, "maximum Apify charge in USD per actor run")
	fs.IntVar(&f.apifyConcurrency, "apify-concurrency", apify.DefaultConcurrency, "maximum concurrent Apify actor runs")
}

func (f *fetchFlags) options(cmd *cobra.Command, env *pipeline.Env, c *collection.Collection) (pipeline.FetchOptions, error) {
	if c != nil {
		return f.collectionOptions(cmd, *c)
	}
	mode, err := profile.ParseMode(f.mode)
	if err != nil {
		return pipeline.FetchOptions{}, err
	}
	base, err := env.SavedSearch(f.searchProfile(), mode)
	if err != nil && !(errors.Is(err, pipeline.ErrNoSavedSearch) && cmd.Flags().Changed("location")) {
		return pipeline.FetchOptions{}, err
	}
	search, err := f.search.apply(cmd, base)
	if err != nil {
		return pipeline.FetchOptions{}, err
	}
	sources := make([]listing.Source, len(f.sources))
	for i, s := range f.sources {
		sources[i] = listing.Source(s)
	}
	return pipeline.FetchOptions{
		Sources:          sources,
		Mode:             mode,
		Search:           search,
		Enrich:           f.enrich,
		Photos:           f.photos,
		MaxChargeUSD:     f.maxCharge,
		ApifyConcurrency: f.apifyConcurrency,
	}, nil
}

func (f *fetchFlags) collectionOptions(cmd *cobra.Command, c collection.Collection) (pipeline.FetchOptions, error) {
	o := pipeline.FetchOptions{
		Collection:       c.ID,
		Sources:          c.Sources,
		Mode:             c.Mode,
		Enrich:           f.enrich,
		Photos:           f.photos,
		MaxChargeUSD:     f.maxCharge,
		ApifyConcurrency: f.apifyConcurrency,
	}
	var err error
	if cmd.Flags().Changed("mode") {
		if o.Mode, err = profile.ParseMode(f.mode); err != nil {
			return pipeline.FetchOptions{}, err
		}
	}
	if cmd.Flags().Changed("source") {
		o.Sources = toSources(f.sources)
	}
	if o.Search, err = f.search.apply(cmd, c.Search); err != nil {
		return pipeline.FetchOptions{}, err
	}
	return o, nil
}

func (f *fetchFlags) searchProfile() string {
	if len(f.profileIDs) == 0 {
		return ""
	}
	return f.profileIDs[0]
}

func newFetchCmd(dataDir, profilesDir *string) *cobra.Command {
	var f fetchFlags
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Search listing sources and store the results",
		Long: "Search listing sources in parallel and store the results.\n\n" +
			"With --profile, the profile's saved search for --mode is used; any search flag given overrides it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env := newEnv(cmd, *dataDir, *profilesDir)
			opts, err := f.options(cmd, env, nil)
			if err != nil {
				return err
			}
			_, err = env.Fetch(cmd.Context(), opts)
			return err
		},
	}
	f.register(cmd.Flags())
	return cmd
}
