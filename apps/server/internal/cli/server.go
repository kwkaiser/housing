package cli

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"git.kwkaiser.io/kwkaiser/housing/internal/app"
	"git.kwkaiser.io/kwkaiser/housing/internal/auth"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

func newServerCmd(dataDir *string) *cobra.Command {
	var (
		addr   string
		noAuth bool
	)
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run the housing API server",
		Long: "Run the long-lived server on --addr until interrupted. It serves a JSON API under /api/v1, with its\n" +
			"OpenAPI spec at /api/openapi.json. The UI is the separate housing-web app.\n\n" +
			"Browsers sign in through the OIDC provider at /auth/login, configured with " + config.EnvOIDCIssuer + ",\n" +
			config.EnvOIDCClientID + ", " + config.EnvOIDCClientSecret + " and " + config.EnvPublicURL + ". API clients send\n" +
			"either a key from `housing apikeys create` or an access token from that provider as a bearer token.\n" +
			"--no-auth serves everything without signing in, for local development.\n\n" +
			"The server also runs queued jobs one at a time and enqueues a run of each collection with a\n" +
			"--schedule once a day after its scheduled time. It holds the data directory's lock while up, so\n" +
			"`housing run` cannot write to the same data directory at the same time.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
			slog.SetDefault(log)
			var provider *auth.OIDC
			if !noAuth {
				cfg := config.LoadOIDC()
				if missing := cfg.Missing(); len(missing) > 0 {
					return fmt.Errorf("%s not set; pass --no-auth to run without signing in", strings.Join(missing, ", "))
				}
				var err error
				if provider, err = auth.New(cmd.Context(), cfg); err != nil {
					return err
				}
			}
			svc, err := openService(cmd, *dataDir, exclusive, func(c *service.Config) { c.Log = log.With("component", "service") })
			if err != nil {
				return err
			}
			defer svc.Close()
			queue := jobs.New(svc.Store(), nil)
			a, err := app.New(svc, queue, log)
			if err != nil {
				return err
			}
			a.Version, a.NoAuth, a.OIDC = version, noAuth, provider
			runner := &jobs.Runner{Queue: queue, Exec: svc, Log: log.With("component", "jobs")}
			scheduler := &jobs.Scheduler{Queue: queue, Log: log.With("component", "scheduler")}

			g, ctx := errgroup.WithContext(cmd.Context())
			g.Go(func() error { return a.Run(ctx, addr) })
			g.Go(func() error { return runner.Run(ctx) })
			g.Go(func() error { return scheduler.Run(ctx) })
			return g.Wait()
		},
	}
	cmd.Flags().StringVar(&addr, "addr", "127.0.0.1:8080", "address to listen on")
	cmd.Flags().BoolVar(&noAuth, "no-auth", false, "serve everything without signing in, for local development")
	return cmd
}
