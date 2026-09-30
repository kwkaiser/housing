package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

const defaultFetchLimit = 10

type fetchOptions struct {
	search    searchFlags
	source    string
	mode      string
	profileID string
	enrich    bool
	photos    bool
	maxCharge float64
}

func newFetchCmd(dataDir, profilesDir *string) *cobra.Command {
	var o fetchOptions
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Search a listing source and store the results",
		Long: "Search a listing source and store the results.\n\n" +
			"With --profile, the profile's saved search for --mode is used; any search flag given overrides it.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFetch(cmd, *dataDir, *profilesDir, o)
		},
	}

	f := cmd.Flags()
	o.search.register(f)
	f.StringVar(&o.source, "source", string(listing.SourceZillow), "listing source")
	f.StringVar(&o.mode, "mode", string(profile.ModeRent), "rent or buy")
	f.StringVar(&o.profileID, "profile", "", "use this profile's saved search for --mode")
	f.BoolVar(&o.enrich, "enrich", true, "fetch listing details (splits buildings into units)")
	f.BoolVar(&o.photos, "photos", true, "download listing photos")
	f.Float64Var(&o.maxCharge, "max-charge", 1, "maximum Apify charge in USD per actor run")
	return cmd
}

func runFetch(cmd *cobra.Command, dataDir, profilesDir string, o fetchOptions) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()

	mode, err := profile.ParseMode(o.mode)
	if err != nil {
		return err
	}
	base := profile.Search{Limit: defaultFetchLimit}
	if o.profileID != "" {
		p, err := profile.Store{Root: profilesDir}.Load(o.profileID)
		if err != nil {
			return err
		}
		saved, ok := p.Searches[mode]
		if !ok {
			return fmt.Errorf("profile %q has no %s search; set one with `housing profile search %s --mode %s ...`", p.ID, mode, p.ID, mode)
		}
		base = saved
		if base.Limit == 0 {
			base.Limit = defaultFetchLimit
		}
	}
	search, err := o.search.apply(cmd, base)
	if err != nil {
		return err
	}
	if search.Location == "" {
		return fmt.Errorf("--location is required")
	}
	if search.Limit <= 0 {
		return fmt.Errorf("--limit must be positive")
	}
	q := search.Query(mode)

	token, err := config.Load().Apify()
	if err != nil {
		return err
	}
	client := apify.NewClient(token)
	client.MaxTotalChargeUSD = o.maxCharge

	provider, err := newProvider(listing.Source(o.source), client)
	if err != nil {
		return err
	}
	enricher, canEnrich := provider.(listing.Enricher)

	sourceQuery := q
	var deferred []listing.Amenity
	sourceQuery.Amenities, deferred = splitAmenities(q.Amenities, provider.SupportedAmenities(q.Offer))
	if len(deferred) > 0 {
		if !o.enrich || !canEnrich {
			return fmt.Errorf("%s cannot filter %s listings by %s; enable --enrich to filter on listing details instead", o.source, mode, joinAmenities(deferred))
		}
		fmt.Fprintf(out, "amenities checked after enrichment: %s\n", joinAmenities(deferred))
	}

	listings, err := provider.Search(ctx, sourceQuery)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "search (%s): %d listings\n", mode, len(listings))

	if canEnrich && o.enrich && len(listings) > 0 {
		enriched, err := enricher.Enrich(ctx, listings)
		if err != nil {
			return err
		}
		listings = slices.DeleteFunc(enriched, func(l listing.Listing) bool { return !q.Matches(l) })
		fmt.Fprintf(out, "enrich: %d of %d listings match\n", len(listings), len(enriched))
	}

	if o.photos {
		p := media.NewProcessor(media.NewHTTPFetcher(), media.DiskStore{Root: dataDir}, media.NewGrid())
		if err := p.FetchPhotos(ctx, listings); err != nil {
			return err
		}
		fmt.Fprintln(out, "photos: downloaded")
	}

	if err := (jsonfile.Persister{}).Persist(ctx, dataDir, listings); err != nil {
		return err
	}
	fmt.Fprintf(out, "stored: %d listings in %s\n", len(listings), dataDir)
	return nil
}

func splitAmenities(want, supported []listing.Amenity) (filterable, deferred []listing.Amenity) {
	for _, a := range want {
		if slices.Contains(supported, a) {
			filterable = append(filterable, a)
		} else {
			deferred = append(deferred, a)
		}
	}
	return filterable, deferred
}

func joinAmenities(as []listing.Amenity) string {
	names := make([]string, len(as))
	for i, a := range as {
		names[i] = string(a)
	}
	return strings.Join(names, ", ")
}

func newProvider(source listing.Source, runner apify.Runner) (listing.Provider, error) {
	switch source {
	case listing.SourceZillow:
		return zillow.New(runner, zillow.NewAutocomplete()), nil
	}
	return nil, fmt.Errorf("unsupported source %q", source)
}
