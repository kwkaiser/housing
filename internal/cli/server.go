package cli

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"

	"git.kwkaiser.io/kwkaiser/housing/internal/app"
	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

func newServerCmd(dataDir *string) *cobra.Command {
	var addr string
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Run the housing web app",
		Long: "Run the long-lived web app on --addr until interrupted. It has no login of its own and is meant to\n" +
			"sit behind a reverse proxy that handles authentication.\n\n" +
			"The server also runs queued jobs one at a time and enqueues a run of each collection with a\n" +
			"--schedule once a day after its scheduled time. It holds the data directory's lock while up, so\n" +
			"`housing run` cannot write to the same data directory at the same time.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			log := slog.New(slog.NewJSONHandler(os.Stderr, nil))
			slog.SetDefault(log)
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
	return cmd
}
