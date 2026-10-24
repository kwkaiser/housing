package media

import (
	"fmt"
	"image"
	"image/color"
	stddraw "image/draw"
	"strconv"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

type FitMode string

const (
	FitContain FitMode = "contain"
	FitCover   FitMode = "cover"
)

type Grid struct {
	Cols       int
	Rows       int
	CellSize   int
	Gap        int
	Fit        FitMode
	Label      bool
	LabelScale int
	Background color.Color
}

var _ Collager = Grid{}

func NewGrid() Grid {
	return Grid{
		Cols:       3,
		Rows:       3,
		CellSize:   512,
		Gap:        4,
		Fit:        FitContain,
		Label:      true,
		LabelScale: 3,
		Background: color.RGBA{R: 24, G: 24, B: 24, A: 255},
	}
}

func (g Grid) ID() string {
	return fmt.Sprintf("grid:%dx%d:%d:%d:%s:%t:%d:%v", g.Cols, g.Rows, g.CellSize, g.Gap, g.Fit, g.Label, g.LabelScale, g.Background)
}

func (g Grid) Collage(photos []image.Image) ([]image.Image, error) {
	if g.Cols <= 0 || g.Rows <= 0 || g.CellSize <= 0 {
		return nil, fmt.Errorf("invalid grid %dx%d cell %d", g.Cols, g.Rows, g.CellSize)
	}
	perPage := g.Cols * g.Rows
	var pages []image.Image
	for start := 0; start < len(photos); start += perPage {
		end := min(start+perPage, len(photos))
		pages = append(pages, g.page(photos[start:end], start))
	}
	return pages, nil
}

func (g Grid) page(photos []image.Image, offset int) image.Image {
	rows := (len(photos) + g.Cols - 1) / g.Cols
	width := g.Cols*g.CellSize + (g.Cols+1)*g.Gap
	height := rows*g.CellSize + (rows+1)*g.Gap

	canvas := image.NewRGBA(image.Rect(0, 0, width, height))
	stddraw.Draw(canvas, canvas.Bounds(), image.NewUniform(g.Background), image.Point{}, stddraw.Src)

	for i, photo := range photos {
		col, row := i%g.Cols, i/g.Cols
		x := g.Gap + col*(g.CellSize+g.Gap)
		y := g.Gap + row*(g.CellSize+g.Gap)
		cell := image.Rect(x, y, x+g.CellSize, y+g.CellSize)
		g.drawPhoto(canvas, cell, photo)
		if g.Label {
			g.drawLabel(canvas, cell.Min, strconv.Itoa(offset+i+1))
		}
	}
	return canvas
}

func (g Grid) drawPhoto(dst *image.RGBA, cell image.Rectangle, src image.Image) {
	sb := src.Bounds()
	if sb.Empty() {
		return
	}
	sw, sh := float64(sb.Dx()), float64(sb.Dy())
	cw, ch := float64(cell.Dx()), float64(cell.Dy())

	if g.Fit == FitCover {
		scale := max(cw/sw, ch/sh)
		cropW, cropH := int(cw/scale), int(ch/scale)
		x0 := sb.Min.X + (sb.Dx()-cropW)/2
		y0 := sb.Min.Y + (sb.Dy()-cropH)/2
		draw.CatmullRom.Scale(dst, cell, src, image.Rect(x0, y0, x0+cropW, y0+cropH), draw.Src, nil)
		return
	}

	scale := min(cw/sw, ch/sh)
	w, h := int(sw*scale), int(sh*scale)
	x0 := cell.Min.X + (cell.Dx()-w)/2
	y0 := cell.Min.Y + (cell.Dy()-h)/2
	draw.CatmullRom.Scale(dst, image.Rect(x0, y0, x0+w, y0+h), src, sb, draw.Src, nil)
}

func (g Grid) drawLabel(dst *image.RGBA, at image.Point, text string) {
	face := basicfont.Face7x13
	pad := 3
	w := font.MeasureString(face, text).Ceil() + 2*pad
	h := face.Metrics().Height.Ceil() + 2*pad

	small := image.NewRGBA(image.Rect(0, 0, w, h))
	stddraw.Draw(small, small.Bounds(), image.NewUniform(color.RGBA{A: 200}), image.Point{}, stddraw.Src)
	d := font.Drawer{
		Dst:  small,
		Src:  image.White,
		Face: face,
		Dot:  fixed.P(pad, pad+face.Metrics().Ascent.Ceil()),
	}
	d.DrawString(text)

	scale := max(g.LabelScale, 1)
	target := image.Rect(at.X, at.Y, at.X+w*scale, at.Y+h*scale)
	draw.NearestNeighbor.Scale(dst, target, small, small.Bounds(), draw.Over, nil)
}
