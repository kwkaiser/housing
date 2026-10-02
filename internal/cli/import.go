package cli

import (
	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

func newImportCmd(dataDir *string) *cobra.Command {
	var profilesDir, collectionsDir string
	cmd := &cobra.Command{
		Use:   "import",
		Short: "Import profiles and collections from their old files into the database",
		Long: "Copy --profiles-dir (profile.json, reference listings, and their photos and collages) and\n" +
			"--collections-dir into the SQLite database and data directory. Safe to run more than once:\n" +
			"re-running updates what was imported before.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, err := openService(cmd, *dataDir, func(c *service.Config) { c.ProfilesDir, c.CollectionsDir = profilesDir, collectionsDir })
			if err != nil {
				return err
			}
			defer svc.Close()
			return svc.Import(cmd.Context(), nil)
		},
	}
	cmd.Flags().StringVar(&profilesDir, "profiles-dir", profile.DefaultRoot, "directory of profile files to import")
	cmd.Flags().StringVar(&collectionsDir, "collections-dir", collection.DefaultRoot, "directory of collection files to import")
	return cmd
}
