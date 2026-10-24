package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/app"
)

func newOpenAPICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "openapi",
		Short: "Print the API's OpenAPI spec as JSON",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := app.OpenAPI()
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), string(b))
			return err
		},
	}
}
