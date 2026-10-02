package cli

import (
	"io"
	"log/slog"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

func openService(cmd *cobra.Command, dataDir string, edit ...func(*service.Config)) (*service.Service, error) {
	cfg := service.Config{
		DataDir:        dataDir,
		ProfilesDir:    profile.DefaultRoot,
		CollectionsDir: collection.DefaultRoot,
		Keys:           config.Load(),
		Log:            cliLogger(cmd.OutOrStdout()),
	}
	for _, e := range edit {
		e(&cfg)
	}
	return service.Open(cmd.Context(), cfg)
}

func exclusive(cfg *service.Config) {
	cfg.Exclusive = true
}

func cliLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if len(groups) == 0 && a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	}))
}
