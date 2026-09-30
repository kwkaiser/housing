package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/pipeline"
)

type collageFlags struct {
	cols         int
	rows         int
	cellSize     int
	gap          int
	fit          string
	noLabel      bool
	maxPhotos    int
	force        bool
	fetchMissing bool
}

func (c *collageFlags) register(fs *pflag.FlagSet, forceName string, withFetch bool) {
	defaults := media.NewGrid()
	fs.IntVar(&c.cols, "cols", defaults.Cols, "photos per row")
	fs.IntVar(&c.rows, "rows", defaults.Rows, "rows per collage")
	fs.IntVar(&c.cellSize, "cell-size", defaults.CellSize, "cell size in pixels")
	fs.IntVar(&c.gap, "gap", defaults.Gap, "gap between cells in pixels")
	fs.StringVar(&c.fit, "fit", string(defaults.Fit), "contain (letterbox) or cover (crop)")
	fs.BoolVar(&c.noLabel, "no-label", false, "omit photo numbers")
	fs.IntVar(&c.maxPhotos, "max-photos", media.DefaultMaxPhotos, "maximum photos per listing")
	fs.BoolVar(&c.force, forceName, false, "rebuild collages that already exist")
	if withFetch {
		fs.BoolVar(&c.fetchMissing, "fetch-missing", false, "download photos that are not stored yet")
	}
}

func (c *collageFlags) options() (pipeline.CollageOptions, error) {
	fit := media.FitMode(c.fit)
	if fit != media.FitContain && fit != media.FitCover {
		return pipeline.CollageOptions{}, fmt.Errorf("invalid --fit %q", c.fit)
	}
	grid := media.NewGrid()
	grid.Cols, grid.Rows, grid.CellSize, grid.Gap = c.cols, c.rows, c.cellSize, c.gap
	grid.Fit, grid.Label = fit, !c.noLabel
	return pipeline.CollageOptions{Grid: grid, MaxPhotos: c.maxPhotos, Force: c.force, FetchMissing: c.fetchMissing}, nil
}

func newCollageCmd(dataDir *string) *cobra.Command {
	var c collageFlags
	cmd := &cobra.Command{
		Use:   "collage",
		Short: "Build photo collages for stored listings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := c.options()
			if err != nil {
				return err
			}
			_, err = newEnv(cmd, *dataDir, "").Collage(cmd.Context(), opts, nil)
			return err
		},
	}
	c.register(cmd.Flags(), "force", true)
	return cmd
}
