package cli

import (
	"github.com/spf13/cobra"
)

var version = "dev"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "housing",
		Short:        "housing CLI",
		Version:      version,
		SilenceUsage: true,
	}
	dataDir := cmd.PersistentFlags().String("data-dir", "data", "directory for stored listings and images")
	cmd.AddCommand(newVersionCmd(), newFetchCmd(dataDir), newCollageCmd(dataDir))
	return cmd
}

func Execute() error {
	return newRootCmd().Execute()
}
