package cli

import (
	"fmt"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
)

func newAPIKeysCmd(dataDir *string) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "apikeys",
		Short: "List keys for the JSON API",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, err := openService(cmd, *dataDir)
			if err != nil {
				return err
			}
			defer svc.Close()
			ks, err := svc.APIKeys(cmd.Context())
			if err != nil {
				return err
			}
			tw := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "ID\tNAME\tKEY\tCREATED\tLAST USED")
			for _, k := range ks {
				used := "never"
				if !k.LastUsedAt.IsZero() {
					used = k.LastUsedAt.Local().Format(time.DateTime)
				}
				fmt.Fprintf(tw, "%d\t%s\t%s…\t%s\t%s\n", k.ID, k.Name, k.Hint, k.CreatedAt.Local().Format(time.DateTime), used)
			}
			return tw.Flush()
		},
	}

	create := &cobra.Command{
		Use:   "create <name>",
		Short: "Create an API key and print it; it is not shown again",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			svc, err := openService(cmd, *dataDir)
			if err != nil {
				return err
			}
			defer svc.Close()
			_, token, err := svc.CreateAPIKey(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), token)
			return nil
		},
	}

	del := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an API key so it stops working",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id, err := strconv.ParseInt(args[0], 10, 64)
			if err != nil {
				return fmt.Errorf("invalid key id %q", args[0])
			}
			svc, err := openService(cmd, *dataDir)
			if err != nil {
				return err
			}
			defer svc.Close()
			if err := svc.DeleteAPIKey(cmd.Context(), id); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "key %d deleted\n", id)
			return nil
		},
	}
	cmd.AddCommand(create, del)
	return cmd
}
