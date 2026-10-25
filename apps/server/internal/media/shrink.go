package media

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"strconv"

	"golang.org/x/image/draw"
)

var Sizes = []int{320, 640, 1280}

func SizedKey(key string, size int) string {
	return key + ".s" + strconv.Itoa(size) + ".jpg"
}

func Shrink(b []byte, maxPx int) ([]byte, error) {
	out, err := ShrinkAll(b, []int{maxPx})
	if err != nil {
		return nil, err
	}
	if small, ok := out[maxPx]; ok {
		return small, nil
	}
	return b, nil
}

func ShrinkAll(b []byte, sizes []int) (map[int][]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}
	longest := max(cfg.Width, cfg.Height)
	out := map[int][]byte{}
	var src image.Image
	for _, size := range sizes {
		if size <= 0 || longest <= size {
			continue
		}
		if src == nil {
			if src, _, err = image.Decode(bytes.NewReader(b)); err != nil {
				return nil, fmt.Errorf("decode image: %w", err)
			}
		}
		w := cfg.Width * size / longest
		h := cfg.Height * size / longest
		dst := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
		draw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Src, nil)
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return nil, err
		}
		out[size] = buf.Bytes()
	}
	return out, nil
}
