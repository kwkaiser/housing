package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

func newEmbedCmd(dataDir *string) *cobra.Command {
	o := service.DefaultEmbedOptions()
	cmd := &cobra.Command{
		Use:   "embed",
		Short: "Embed listings that have collages but no embedding yet",
		Long: "Send each listing's collages and description to an OpenRouter embedding model and store the vector,\n" +
			"newest listings first, stopping once --max-cost is spent. Use --dry-run to count what would be embedded.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, err := openService(cmd, *dataDir, exclusive)
			if err != nil {
				return err
			}
			defer svc.Close()
			res, err := svc.Embed(cmd.Context(), o, nil)
			out := cmd.OutOrStdout()
			if o.DryRun {
				fmt.Fprintf(out, "embed: %d listings (%d collages) without %s embeddings\n", res.Pending, res.Collages, o.Model)
				return err
			}
			fmt.Fprintf(out, "embed: %d of %d embedded, %d failed, %d tokens, $%.4f", res.Embedded, res.Pending, res.Failed, res.Tokens, res.CostUSD)
			if res.BudgetReached {
				fmt.Fprint(out, " (budget reached)")
			}
			fmt.Fprintln(out)
			return err
		},
	}
	f := cmd.Flags()
	f.StringVar(&o.Collection, "collection", "", "only embed listings seen in this collection")
	f.StringVar(&o.Model, "model", o.Model, "OpenRouter embedding model")
	f.IntVar(&o.Dimensions, "dimensions", o.Dimensions, "vector dimensions to request; every model must share one size")
	f.IntVar(&o.Limit, "limit", 0, "maximum listings to embed (0 for all)")
	f.IntVar(&o.Concurrency, "concurrency", o.Concurrency, "embedding requests in flight")
	f.Float64Var(&o.MaxCostUSD, "max-cost", o.MaxCostUSD, "stop starting new requests after spending this many USD (0 for no limit)")
	f.BoolVar(&o.DryRun, "dry-run", false, "count listings that need embeddings without calling OpenRouter")
	return cmd
}
