package media

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"maps"
	"slices"
	"testing"
)

func jpegOf(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := range w {
		img.Set(x, 0, color.RGBA{R: uint8(x), A: 255})
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestShrink(t *testing.T) {
	big := jpegOf(t, 1552, 1040)
	out, err := Shrink(big, 1024)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := jpeg.DecodeConfig(bytes.NewReader(out))
	if err != nil || cfg.Width != 1024 || cfg.Height != 686 {
		t.Errorf("shrunk to %dx%d %v, want 1024x686", cfg.Width, cfg.Height, err)
	}
	if len(out) >= len(big) {
		t.Errorf("shrunk image is not smaller: %d >= %d", len(out), len(big))
	}

	small := jpegOf(t, 800, 600)
	if out, _ := Shrink(small, 1024); !bytes.Equal(out, small) {
		t.Error("images already within the limit should be returned unchanged")
	}
	if out, _ := Shrink(big, 0); !bytes.Equal(out, big) {
		t.Error("a zero limit should leave images unchanged")
	}
	if _, err := Shrink([]byte("not an image"), 1024); err == nil {
		t.Error("garbage should fail to decode")
	}
}

func TestShrinkAll(t *testing.T) {
	out, err := ShrinkAll(jpegOf(t, 1000, 750), Sizes)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := out[1280]; ok || len(out) != 2 {
		t.Fatalf("got sizes %v, want only those smaller than the image", slices.Sorted(maps.Keys(out)))
	}
	for size, want := range map[int][2]int{320: {320, 240}, 640: {640, 480}} {
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(out[size]))
		if err != nil || cfg.Width != want[0] || cfg.Height != want[1] {
			t.Errorf("size %d = %dx%d %v, want %v", size, cfg.Width, cfg.Height, err, want)
		}
	}
}
