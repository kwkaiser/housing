package media

import (
	"image"
	"image/color"
	"testing"
)

func solid(w, h int, c color.Color) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestGridPaging(t *testing.T) {
	g := NewGrid()
	g.CellSize = 64
	photos := make([]image.Image, 11)
	for i := range photos {
		photos[i] = solid(40, 30, color.RGBA{R: 255, A: 255})
	}

	pages, err := g.Collage(photos)
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) != 2 {
		t.Fatalf("got %d pages", len(pages))
	}
	full := 3*64 + 4*4
	if b := pages[0].Bounds(); b.Dx() != full || b.Dy() != full {
		t.Errorf("page 0 = %v", b)
	}
	if b := pages[1].Bounds(); b.Dx() != full || b.Dy() != 64+2*4 {
		t.Errorf("page 1 should shrink to one row, got %v", b)
	}
}

func TestGridFit(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	wide := solid(200, 100, red)
	cell := 100
	gap := 4

	contain := NewGrid()
	contain.CellSize, contain.Label = cell, false
	pages, _ := contain.Collage([]image.Image{wide})
	top := pages[0].At(gap+cell/2, gap+5)
	if r, _, _, _ := top.RGBA(); r>>8 == 255 {
		t.Error("contain should letterbox the top of a wide photo")
	}
	if r, _, _, _ := pages[0].At(gap+cell/2, gap+cell/2).RGBA(); r>>8 != 255 {
		t.Error("contain should draw the photo in the middle of the cell")
	}

	cover := contain
	cover.Fit = FitCover
	pages, _ = cover.Collage([]image.Image{wide})
	if r, _, _, _ := pages[0].At(gap+cell/2, gap+2).RGBA(); r>>8 != 255 {
		t.Error("cover should fill the cell")
	}
}

func TestGridLabel(t *testing.T) {
	g := NewGrid()
	g.CellSize = 128
	pages, _ := g.Collage([]image.Image{solid(128, 128, color.RGBA{R: 255, A: 255})})
	if r, _, _, _ := pages[0].At(g.Gap+2, g.Gap+2).RGBA(); r>>8 == 255 {
		t.Error("label should cover the top-left corner")
	}
}

func TestGridIDChangesWithLayout(t *testing.T) {
	a := NewGrid()
	b := NewGrid()
	b.Cols = 4
	if a.ID() == b.ID() {
		t.Error("grid ID should change with layout")
	}
}
