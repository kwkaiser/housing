package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
)

type collageOptions struct {
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

func newCollageCmd(dataDir *string) *cobra.Command {
	defaults := media.NewGrid()
	var o collageOptions
	cmd := &cobra.Command{
		Use:   "collage",
		Short: "Build photo collages for stored listings",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runCollage(cmd, *dataDir, o)
		},
	}

	f := cmd.Flags()
	f.IntVar(&o.cols, "cols", defaults.Cols, "photos per row")
	f.IntVar(&o.rows, "rows", defaults.Rows, "rows per collage")
	f.IntVar(&o.cellSize, "cell-size", defaults.CellSize, "cell size in pixels")
	f.IntVar(&o.gap, "gap", defaults.Gap, "gap between cells in pixels")
	f.StringVar(&o.fit, "fit", string(defaults.Fit), "contain (letterbox) or cover (crop)")
	f.BoolVar(&o.noLabel, "no-label", false, "omit photo numbers")
	f.IntVar(&o.maxPhotos, "max-photos", media.DefaultMaxPhotos, "maximum photos per listing")
	f.BoolVar(&o.force, "force", false, "rebuild collages that already exist")
	f.BoolVar(&o.fetchMissing, "fetch-missing", false, "download photos that are not stored yet")
	return cmd
}

func runCollage(cmd *cobra.Command, dataDir string, o collageOptions) error {
	ctx := cmd.Context()
	fit := media.FitMode(o.fit)
	if fit != media.FitContain && fit != media.FitCover {
		return fmt.Errorf("invalid --fit %q", o.fit)
	}

	grid := media.NewGrid()
	grid.Cols, grid.Rows, grid.CellSize, grid.Gap = o.cols, o.rows, o.cellSize, o.gap
	grid.Fit, grid.Label = fit, !o.noLabel

	p := media.NewProcessor(media.NewHTTPFetcher(), media.DiskStore{Root: dataDir}, grid)
	p.MaxPhotos, p.Force = o.maxPhotos, o.force

	persister := jsonfile.Persister{}
	listings, err := persister.Load(ctx, dataDir)
	if err != nil {
		return err
	}
	if o.fetchMissing {
		if err := p.FetchPhotos(ctx, listings); err != nil {
			return err
		}
	}
	processed, err := p.Collage(ctx, listings)
	if err != nil {
		return err
	}
	if err := persister.Persist(ctx, dataDir, processed); err != nil {
		return err
	}

	distinct := map[string]bool{}
	without := 0
	for _, l := range processed {
		if len(l.Collages) == 0 {
			without++
		}
		for _, c := range l.Collages {
			distinct[c] = true
		}
	}
	fmt.Fprintf(cmd.OutOrStdout(), "%d listings, %d collages, %d listings without collages\n", len(processed), len(distinct), without)
	return nil
}
