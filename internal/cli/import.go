package cli

import (
	"github.com/spf13/cobra"
)

func newImportJSONCmd(dataDir, profilesDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "import-json",
		Short: "Import listings from the old per-source JSON files into the database",
		Long: "Import data/<source>.json into the SQLite database, recording each listing as observed on the\n" +
			"day it was last fetched. Safe to run more than once.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return newEnv(cmd, *dataDir, *profilesDir).ImportJSON(cmd.Context())
		},
	}
}
