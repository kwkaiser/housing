package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

func newService(dataDir string) *service.Service {
	return newImportService(dataDir, profile.DefaultRoot, collection.DefaultRoot)
}

func newImportService(dataDir, profilesDir, collectionsDir string) *service.Service {
	return service.New(service.Config{
		DataDir:        dataDir,
		ProfilesDir:    profilesDir,
		CollectionsDir: collectionsDir,
		Keys:           config.Load(),
	})
}

func printProgress(cmd *cobra.Command) service.Progress {
	out := cmd.OutOrStdout()
	return func(e service.Event) {
		fmt.Fprintln(out, e.Message)
	}
}
