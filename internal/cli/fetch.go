package cli

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

type fetchOptions struct {
	source    string
	offer     string
	location  string
	radius    float64
	minPrice  int
	maxPrice  int
	minBeds   int
	maxBeds   int
	maxAge    string
	amenities []string
	limit     int
	enrich    bool
	photos    bool
	maxCharge float64
}

func newFetchCmd(dataDir *string) *cobra.Command {
	var o fetchOptions
	cmd := &cobra.Command{
		Use:   "fetch",
		Short: "Search a listing source and store the results",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFetch(cmd, *dataDir, o)
		},
	}

	f := cmd.Flags()
	f.StringVar(&o.source, "source", string(listing.SourceZillow), "listing source")
	f.StringVar(&o.offer, "offer", string(listing.OfferRent), "rent or sale")
	f.StringVar(&o.location, "location", "", "city, ZIP, neighborhood, county or state")
	f.Float64Var(&o.radius, "radius", 0, "search radius in miles around the location")
	f.IntVar(&o.minPrice, "min-price", 0, "minimum price in dollars")
	f.IntVar(&o.maxPrice, "max-price", 0, "maximum price in dollars")
	f.IntVar(&o.minBeds, "min-beds", 0, "minimum bedrooms")
	f.IntVar(&o.maxBeds, "max-beds", 0, "maximum bedrooms")
	f.StringVar(&o.maxAge, "max-age", "", "maximum listing age, e.g. 7d or 36h")
	f.StringSliceVar(&o.amenities, "amenity", nil, "required amenity (repeatable)")
	f.IntVar(&o.limit, "limit", 10, "maximum search results")
	f.BoolVar(&o.enrich, "enrich", true, "fetch listing details (splits buildings into units)")
	f.BoolVar(&o.photos, "photos", true, "download listing photos")
	f.Float64Var(&o.maxCharge, "max-charge", 1, "maximum Apify charge in USD per actor run")
	cmd.MarkFlagRequired("location")
	return cmd
}

func runFetch(cmd *cobra.Command, dataDir string, o fetchOptions) error {
	ctx := cmd.Context()
	q, err := o.query(cmd)
	if err != nil {
		return err
	}

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

	listings, err := provider.Search(ctx, q)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "search: %d listings\n", len(listings))

	if enricher, ok := provider.(listing.Enricher); ok && o.enrich && len(listings) > 0 {
		enriched, err := enricher.Enrich(ctx, listings)
		if err != nil {
			return err
		}
		listings = slices.DeleteFunc(enriched, func(l listing.Listing) bool { return !q.Matches(l) })
		fmt.Fprintf(out, "enrich: %d listings\n", len(listings))
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

func newProvider(source listing.Source, runner apify.Runner) (listing.Provider, error) {
	switch source {
	case listing.SourceZillow:
		return zillow.New(runner, zillow.NewAutocomplete()), nil
	}
	return nil, fmt.Errorf("unsupported source %q", source)
}

func (o fetchOptions) query(cmd *cobra.Command) (listing.Query, error) {
	if o.limit <= 0 {
		return listing.Query{}, fmt.Errorf("--limit must be positive")
	}
	q := listing.Query{
		Offer: listing.OfferType(o.offer),
		Area:  listing.Area{Location: o.location, RadiusMiles: o.radius},
		Limit: o.limit,
	}
	flags := cmd.Flags()
	if flags.Changed("min-price") {
		q.MinPrice = &listing.Money{Cents: int64(o.minPrice) * 100, Currency: "USD"}
	}
	if flags.Changed("max-price") {
		q.MaxPrice = &listing.Money{Cents: int64(o.maxPrice) * 100, Currency: "USD"}
	}
	if flags.Changed("min-beds") {
		q.MinBeds = &o.minBeds
	}
	if flags.Changed("max-beds") {
		q.MaxBeds = &o.maxBeds
	}
	if o.maxAge != "" {
		d, err := parseAge(o.maxAge)
		if err != nil {
			return listing.Query{}, err
		}
		q.MaxAge = d
	}
	for _, a := range o.amenities {
		q.Amenities = append(q.Amenities, listing.Amenity(a))
	}
	return q, nil
}

func parseAge(s string) (time.Duration, error) {
	if days, ok := strings.CutSuffix(s, "d"); ok {
		n, err := strconv.Atoi(days)
		if err != nil {
			return 0, fmt.Errorf("invalid --max-age %q", s)
		}
		return time.Duration(n) * 24 * time.Hour, nil
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return 0, fmt.Errorf("invalid --max-age %q", s)
	}
	return d, nil
}
