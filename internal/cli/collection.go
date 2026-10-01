package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/pipeline"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func newCollectionCmd(root, profilesDir *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collection",
		Short: "Manage collections: a search plus the profiles to grade its results against",
	}
	cmd.AddCommand(newCollectionSetCmd(root, profilesDir), newCollectionListCmd(root), newCollectionShowCmd(root))
	return cmd
}

func newCollectionSetCmd(root, profilesDir *string) *cobra.Command {
	var (
		search     searchFlags
		profiles   []string
		sources    []string
		mode       string
		model      string
		maxRunCost float64
	)
	cmd := &cobra.Command{
		Use:   "set <id>",
		Short: "Create a collection or update an existing one",
		Long: "Create a collection, or update one: flags given are merged into the existing collection.\n\n" +
			"Run a collection with `housing run --collection <id>`.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			store := collection.Store{Root: *root}
			c, err := store.Load(args[0])
			switch {
			case errors.Is(err, collection.ErrNotFound):
				c = collection.Collection{ID: args[0], Mode: profile.ModeRent, Sources: []listing.Source{listing.SourceZillow}, Search: profile.Search{Limit: 50}}
			case err != nil:
				return err
			}

			changed := cmd.Flags().Changed
			if changed("profile") {
				c.Profiles = profiles
			}
			if changed("source") {
				c.Sources = toSources(sources)
				for _, src := range c.Sources {
					if _, err := pipeline.NewProvider(src, nil); err != nil {
						return err
					}
				}
			}
			if changed("mode") {
				if c.Mode, err = profile.ParseMode(mode); err != nil {
					return err
				}
			}
			if changed("model") {
				c.Model = model
			}
			if changed("max-run-cost-usd") {
				c.MaxRunCostUSD = maxRunCost
			}
			if c.Search, err = search.apply(cmd, c.Search); err != nil {
				return err
			}
			ps := profile.Store{Root: *profilesDir}
			for _, id := range c.Profiles {
				if _, err := ps.Effective(id); err != nil {
					return err
				}
			}
			if err := store.Save(c); err != nil {
				return err
			}
			return printCollection(cmd, c)
		},
	}
	f := cmd.Flags()
	search.register(f)
	f.StringSliceVar(&profiles, "profile", nil, "profiles to grade results against (repeatable)")
	f.StringSliceVar(&sources, "source", nil, "listing sources to search (repeatable)")
	f.StringVar(&mode, "mode", string(profile.ModeRent), "rent or buy")
	f.StringVar(&model, "model", "", "OpenRouter model used to grade listings (default: the assess default)")
	f.Float64Var(&maxRunCost, "max-run-cost-usd", 0, "cap on grading spend per run (0 for no cap)")
	return cmd
}

func newCollectionListCmd(root *string) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List collections",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cs, err := collection.Store{Root: *root}.List()
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tMODE\tLOCATION\tSOURCES\tPROFILES")
			for _, c := range cs {
				names := make([]string, len(c.Sources))
				for i, s := range c.Sources {
					names[i] = string(s)
				}
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", c.ID, c.Mode, c.Search.Location, strings.Join(names, ","), strings.Join(c.Profiles, ","))
			}
			return tw.Flush()
		},
	}
}

func newCollectionShowCmd(root *string) *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show a collection",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := collection.Store{Root: *root}.Load(args[0])
			if err != nil {
				return err
			}
			return printCollection(cmd, c)
		},
	}
}

func printCollection(cmd *cobra.Command, c collection.Collection) error {
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), string(b))
	return nil
}

type scope struct {
	collection *collection.Collection
	profileIDs []string
	mode       profile.Mode
}

func resolveScope(cmd *cobra.Command, collectionsDir, collectionID string, profileIDs []string, mode string) (scope, error) {
	var sc scope
	if mode != "" {
		m, err := profile.ParseMode(mode)
		if err != nil {
			return sc, err
		}
		sc.mode = m
	}
	sc.profileIDs = profileIDs
	if collectionID != "" {
		c, err := collection.Store{Root: collectionsDir}.Load(collectionID)
		if err != nil {
			return sc, err
		}
		sc.collection = &c
		if !cmd.Flags().Changed("profile") {
			sc.profileIDs = c.Profiles
		}
		if mode == "" {
			sc.mode = c.Mode
		}
	}
	if len(sc.profileIDs) == 0 {
		return sc, fmt.Errorf("--profile or --collection is required")
	}
	return sc, nil
}

func (sc scope) collectionID() string {
	if sc.collection == nil {
		return ""
	}
	return sc.collection.ID
}

func toSources(names []string) []listing.Source {
	out := make([]listing.Source, len(names))
	for i, s := range names {
		out[i] = listing.Source(s)
	}
	return out
}
