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
	cmd.AddCommand(newVersionCmd())
	return cmd
}

func Execute() error {
	return newRootCmd().Execute()
}
