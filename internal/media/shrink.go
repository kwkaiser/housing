package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"golang.org/x/image/draw"
)

func Shrink(b []byte, maxPx int) ([]byte, error) {
	if maxPx <= 0 {
		return b, nil
	}
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	longest := max(cfg.Width, cfg.Height)
	if longest <= maxPx {
		return b, nil
	}
	src, _, err := image.Decode(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	w := cfg.Width * maxPx / longest
	h := cfg.Height * maxPx / longest
	dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
