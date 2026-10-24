package cli

import (
	"fmt"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
)

func newJobsCmd(dataDir *string) *cobra.Command {
	var (
		collectionID string
		limit        int
	)
	queue := func(cmd *cobra.Command) (*jobs.Queue, func(), error) {
		svc, err := openService(cmd, *dataDir)
		if err != nil {
			return nil, nil, err
		}
		return jobs.New(svc.Store(), nil), func() { svc.Close() }, nil
	}
	cmd := &cobra.Command{
		Use:   "jobs",
		Short: "List recent jobs run by `housing server`",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			q, done, err := queue(cmd)
			if err != nil {
				return err
			}
			defer done()
			js, err := q.Jobs(cmd.Context(), jobs.Filter{CollectionID: collectionID, Limit: limit})
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tKIND\tTARGET\tTRIGGER\tSTATUS\tCREATED\tDURATION\tCOST\tERROR")
			for _, j := range js {
				fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%s\t%s\t%s\t$%.4f\t%s\n", j.ID, j.Kind, target(j), j.Trigger, j.Status,
					j.CreatedAt.Local().Format(time.DateTime), duration(j), j.CostUSD, j.Error)
			}
			return tw.Flush()
		},
	}
	cmd.Flags().StringVar(&collectionID, "collection", "", "only list this collection's jobs")
	cmd.Flags().IntVar(&limit, "limit", 20, "maximum jobs to list (0 for all)")

	show := &cobra.Command{
		Use:   "show <id>",
		Short: "Show a job and its progress log",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid job id %q", args[0])
			}
			q, done, err := queue(cmd)
			if err != nil {
				return err
			}
			defer done()
			j, err := q.Job(cmd.Context(), id)
			if err != nil {
				return err
			}
			events, err := q.Events(cmd.Context(), id)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "job %d: %s %s (%s), %s\n", j.ID, j.Kind, target(j), j.Trigger, j.Status)
			fmt.Fprintf(out, "params: %s\n", j.Params)
			fmt.Fprintf(out, "created %s, duration %s, cost $%.4f\n", j.CreatedAt.Local().Format(time.DateTime), duration(j), j.CostUSD)
			if j.Error != "" {
				fmt.Fprintf(out, "error: %s\n", j.Error)
			}
			if len(j.Result) > 0 {
				fmt.Fprintf(out, "result: %s\n", j.Result)
			}
			for _, e := range events {
				attrs, err := jobs.EventAttrs(e)
				if err != nil {
					return err
				}
				fmt.Fprintf(out, "%s %-5s %s", e.At.Local().Format(time.TimeOnly), e.Level, e.Message)
				for _, a := range attrs {
					fmt.Fprintf(out, " %s=%q", a.Key, a.Value)
				}
				fmt.Fprintln(out)
			}
			return nil
		},
	}

	enqueue := &cobra.Command{
		Use:   "run <collection>",
		Short: "Queue a run of a collection for `housing server` to pick up",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService(cmd, *dataDir)
			if err != nil {
				return err
			}
			defer svc.Close()
			if _, err := svc.Collection(cmd.Context(), args[0]); err != nil {
				return err
			}
			id, err := jobs.New(svc.Store(), nil).Enqueue(cmd.Context(), jobs.RunCollectionParams{CollectionID: args[0]})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "job %d queued\n", id)
			return nil
		},
	}
	cmd.AddCommand(show, enqueue)
	return cmd
}

func target(j jobs.Job) string {
	if j.CollectionID != "" {
		return j.CollectionID
	}
	return j.ProfileID
}

func duration(j jobs.Job) string {
	if j.StartedAt.IsZero() {
		return "-"
	}
	end := j.FinishedAt
	if end.IsZero() {
		end = time.Now()
	}
	return end.Sub(j.StartedAt).Round(time.Second).String()
}
