package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
)

var version = "dev"

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "housing",
		Short:        "housing admin CLI",
		Version:      version,
		SilenceUsage: true,
	}
	dataDir := cmd.PersistentFlags().String("data-dir", "data", "directory for the database and media")
	cmd.AddCommand(
		newVersionCmd(),
		newServerCmd(dataDir),
		newMigrateCmd(dataDir),
		newImportCmd(dataDir),
		newJobsCmd(dataDir),
		newRunCmd(dataDir),
		newAPIKeysCmd(dataDir),
	)
	return cmd
}

func Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		stop()
	}()
	return newRootCmd().ExecuteContext(ctx)
}
