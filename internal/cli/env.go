package cli

import (
	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/pipeline"
)

func newEnv(cmd *cobra.Command, dataDir, profilesDir string) *pipeline.Env {
	return &pipeline.Env{
		DataDir:     dataDir,
		ProfilesDir: profilesDir,
		Config:      config.Load(),
		Out:         cmd.OutOrStdout(),
	}
}
