package cli

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/apify"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/providers/zillow"
)

func newProfileCmd() *cobra.Command {
	var root string
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Create and draft search profiles from reference listings",
	}
	cmd.PersistentFlags().StringVar(&root, "profiles-dir", profile.DefaultRoot, "directory for profiles")
	cmd.AddCommand(newProfileCreateCmd(&root), newProfileDraftCmd(&root))
	return cmd
}

type profileCreateOptions struct {
	id        string
	name      string
	notes     []string
	model     string
	noDraft   bool
	force     bool
	maxCharge float64
}

func newProfileCreateCmd(root *string) *cobra.Command {
	var o profileCreateOptions
	cmd := &cobra.Command{
		Use:   "create <listing-url>",
		Short: "Create a profile from a listing you like",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileCreate(cmd, profile.Store{Root: *root}, args[0], o)
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.id, "id", "", "profile id (lowercase letters, digits, dashes)")
	f.StringVar(&o.name, "name", "", "profile name (drafted if empty)")
	f.StringArrayVar(&o.notes, "note", nil, "something you like about the listing (repeatable)")
	f.StringVar(&o.model, "model", profile.DefaultDraftModel, "OpenRouter model used to draft criteria")
	f.BoolVar(&o.noDraft, "no-draft", false, "store the reference without drafting criteria")
	f.BoolVar(&o.force, "force", false, "overwrite an existing profile")
	f.Float64Var(&o.maxCharge, "max-charge", 0.1, "maximum Apify charge in USD")
	cmd.MarkFlagRequired("id")
	return cmd
}

func runProfileCreate(cmd *cobra.Command, store profile.Store, rawURL string, o profileCreateOptions) error {
	ctx := cmd.Context()
	out := cmd.OutOrStdout()
	if err := profile.ValidID(o.id); err != nil {
		return err
	}
	exists, err := store.Exists(o.id)
	if err != nil {
		return err
	}
	if exists && !o.force {
		return fmt.Errorf("profile %q already exists (use --force to overwrite)", o.id)
	}

	cfg := config.Load()
	if !o.noDraft {
		if _, err := cfg.OpenRouter(); err != nil {
			return err
		}
	}
	token, err := cfg.Apify()
	if err != nil {
		return err
	}
	client := apify.NewClient(token)
	client.MaxTotalChargeUSD = o.maxCharge

	lookup, err := lookupFor(rawURL, client)
	if err != nil {
		return err
	}
	ref, err := lookup.Lookup(ctx, rawURL)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "reference: %s (%d photos)\n", ref.Address.Formatted, len(ref.Photos))

	processor := media.NewProcessor(media.NewHTTPFetcher(), store.Media(o.id), media.NewGrid())
	processed, err := processor.Process(ctx, []listing.Listing{ref})
	if err != nil {
		return err
	}
	ref = processed[0]
	if len(ref.Collages) == 0 {
		return fmt.Errorf("no photos could be downloaded for %s", ref.URL)
	}
	if err := (jsonfile.Persister{}).Persist(ctx, store.ListingsDir(o.id), []listing.Listing{ref}); err != nil {
		return err
	}
	fmt.Fprintf(out, "collages: %s\n", strings.Join(ref.Collages, ", "))

	p := profile.Profile{
		ID:     o.id,
		Name:   o.name,
		Notes:  o.notes,
		Ignore: profile.DefaultIgnore,
		References: []profile.Reference{{
			Source:   ref.Source,
			SourceID: ref.SourceID,
			URL:      ref.URL,
			Collages: ref.Collages,
		}},
	}
	if !o.noDraft {
		if err := draftProfile(ctx, out, cfg, store, &p, o.model); err != nil {
			if saveErr := store.Save(p); saveErr != nil {
				return fmt.Errorf("%w (and saving undrafted profile failed: %w)", err, saveErr)
			}
			return fmt.Errorf("%w; saved undrafted profile, retry with `housing profile draft %s`", err, p.ID)
		}
	}
	if err := store.Save(p); err != nil {
		return err
	}
	fmt.Fprintf(out, "saved: %s\n", store.Dir(p.ID))
	return nil
}

func newProfileDraftCmd(root *string) *cobra.Command {
	var model string
	var notes []string
	cmd := &cobra.Command{
		Use:   "draft <id>",
		Short: "Draft or re-draft a profile's criteria from its stored references",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store := profile.Store{Root: *root}
			p, err := store.Load(args[0])
			if err != nil {
				return err
			}
			if cmd.Flags().Changed("note") {
				p.Notes = notes
			}
			if err := draftProfile(cmd.Context(), cmd.OutOrStdout(), config.Load(), store, &p, model); err != nil {
				return err
			}
			if err := store.Save(p); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "saved: %s\n", store.Dir(p.ID))
			return nil
		},
	}
	cmd.Flags().StringVar(&model, "model", profile.DefaultDraftModel, "OpenRouter model used to draft criteria")
	cmd.Flags().StringArrayVar(&notes, "note", nil, "replace the profile's notes (repeatable)")
	return cmd
}

func draftProfile(ctx context.Context, out io.Writer, cfg config.Config, store profile.Store, p *profile.Profile, model string) error {
	key, err := cfg.OpenRouter()
	if err != nil {
		return err
	}
	refs, err := store.References(ctx, *p)
	if err != nil {
		return err
	}

	drafter := profile.Drafter{Client: openrouter.NewClient(key), Model: model}
	draft, meta, err := drafter.Draft(ctx, p.Notes, refs)
	if err != nil {
		return err
	}
	if err := p.Apply(draft, meta); err != nil {
		return err
	}
	fmt.Fprintf(out, "drafted: %d want, %d avoid with %s ($%.4f)\n", len(p.Want), len(p.Avoid), meta.Model, meta.CostUSD)
	return nil
}

func lookupFor(rawURL string, runner apify.Runner) (listing.Lookup, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	switch host := u.Hostname(); {
	case host == "zillow.com" || strings.HasSuffix(host, ".zillow.com"):
		return zillow.New(runner, zillow.NewAutocomplete()), nil
	}
	return nil, fmt.Errorf("no lookup available for %q", u.Hostname())
}
