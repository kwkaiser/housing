package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newMigrateCmd(dataDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "migrate",
		Short: "Apply pending database migrations and print the schema version",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			db, err := newService(*dataDir).OpenStore(cmd.Context())
			if err != nil {
				return err
			}
			defer db.Close()
			v, err := db.SchemaVersion(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "schema version %d\n", v)
			return nil
		},
	}
}
