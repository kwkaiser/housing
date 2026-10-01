package cli

import (
	"github.com/spf13/cobra"
)

func newPublishCmd(dataDir, profilesDir, collectionsDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "publish <dir>",
		Short: "Publish every collection's daily results as a static site",
		Long: "Write a static site to <dir>: index.html, a read-only SQLite database of every collection's ranked\n" +
			"listings for each day, and current.json naming that database. The page loads the database in the\n" +
			"browser, so <dir> can be served by any static file server.\n\n" +
			"The database file name contains a hash of its contents, so it can be cached forever; index.html and\n" +
			"current.json should not be cached. The previous database is kept so pages already open keep working.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			env := newEnv(cmd, *dataDir, *profilesDir)
			env.CollectionsDir = *collectionsDir
			_, err := env.Publish(cmd.Context(), args[0])
			return err
		},
	}
}
