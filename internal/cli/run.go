package cli

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"
)

func newRunCmd(dataDir *string) *cobra.Command {
	return &cobra.Command{
		Use:   "run <collection>",
		Short: "Run a collection in this process, for debugging",
		Long: "Fetch, collage and grade a collection's listings in this process with the collection's settings,\n" +
			"printing progress as it goes. Only one process may use a data directory at a time, so this fails\n" +
			"fast while `housing server` is up; use `housing jobs run` to queue a run on the server instead.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService(cmd, *dataDir, exclusive)
			if err != nil {
				return err
			}
			defer svc.Close()
			res, err := svc.RunCollection(cmd.Context(), args[0], nil)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "run: %s on %s: %d fetched, %d distinct, %d graded, %d failed, %d notified, $%.4f\n",
				res.Collection, res.Day, res.Fetched, res.Distinct, res.Stats.Updated, res.Stats.Failed, res.Notified, res.Stats.CostUSD)
			return errors.Join(res.AssessErr, res.NotifyErr)
		},
	}
}
