package cli

import (
	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/pipeline"
)

func newServeCmd(dataDir, profilesDir, collectionsDir *string) *cobra.Command {
	var o pipeline.ServeOptions
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Preview the published site locally",
		Long: "Publish every collection to a temporary directory and serve it over HTTP for a local preview.\n" +
			"With --dir, serve an already-published directory instead. Caching is disabled, so reload to see a\n" +
			"new publish.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			env := newEnv(cmd, *dataDir, *profilesDir)
			env.CollectionsDir = *collectionsDir
			return env.Serve(cmd.Context(), o)
		},
	}
	cmd.Flags().StringVar(&o.Addr, "addr", "127.0.0.1:8080", "address to listen on")
	cmd.Flags().StringVar(&o.Dir, "dir", "", "serve this published directory instead of publishing to a temporary one")
	return cmd
}
