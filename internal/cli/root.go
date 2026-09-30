package cli

import (
	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
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
	profilesDir := cmd.PersistentFlags().String("profiles-dir", profile.DefaultRoot, "directory for profiles")
	cmd.AddCommand(
		newVersionCmd(),
		newFetchCmd(dataDir, profilesDir),
		newCollageCmd(dataDir),
		newProfileCmd(profilesDir),
		newAssessCmd(dataDir, profilesDir),
		newReportCmd(dataDir, profilesDir),
	)
	return cmd
}

func Execute() error {
	return newRootCmd().Execute()
}
