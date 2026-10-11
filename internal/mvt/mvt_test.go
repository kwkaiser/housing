package mvt_test

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/mvt"
	"git.kwkaiser.io/kwkaiser/housing/internal/mvt/mvttest"
)

func testTile() []byte {
	return mvttest.Tile(
		mvttest.Layer{Name: "routes", Features: []mvttest.Feature{{
			ID:    7,
			Type:  int(mvt.LineString),
			Props: map[string]any{"name": "Red Line", "headway": 600, "night": true},
			Lines: [][][2]int{{{0, 0}, {4096, 4096}}, {{2048, 2048}, {2048, 4096}}},
		}}},
		mvttest.Layer{Name: "buildings", Features: []mvttest.Feature{{
			ID: 1, Type: int(mvt.LineString), Lines: [][][2]int{{{0, 0}, {1, 1}}},
		}}},
	)
}

func near(a, b mvt.LatLng) bool {
	return math.Abs(a[0]-b[0]) < 1e-6 && math.Abs(a[1]-b[1]) < 1e-6
}

func TestDecode(t *testing.T) {
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	w.Write(testTile())
	w.Close()

	for name, data := range map[string][]byte{"plain": testTile(), "gzip": gz.Bytes()} {
		layers, err := mvt.Decode(data, 0, 0, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(layers) != 2 || layers[0].Name != "routes" || len(layers[0].Features) != 1 {
			t.Fatalf("%s: layers = %+v", name, layers)
		}
		f := layers[0].Features[0]
		if f.ID != 7 || f.Type != mvt.LineString {
			t.Errorf("%s: feature = %+v", name, f)
		}
		if f.Props["name"] != "Red Line" || f.Props["headway"] != 600.0 || f.Props["night"] != true {
			t.Errorf("%s: props = %v", name, f.Props)
		}
		if len(f.Parts) != 2 || len(f.Parts[0]) != 2 || len(f.Parts[1]) != 2 {
			t.Fatalf("%s: parts = %v", name, f.Parts)
		}
		top := 85.0511287798
		for i, want := range []mvt.LatLng{{top, -180}, {-top, 180}, {0, 0}, {-top, 0}} {
			if got := f.Parts[i/2][i%2]; !near(got, want) {
				t.Errorf("%s: point %d = %v, want %v", name, i, got, want)
			}
		}
	}

	layers, err := mvt.Decode(testTile(), 14, 4958, 6060, "routes")
	if err != nil {
		t.Fatal(err)
	}
	if len(layers) != 1 || layers[0].Name != "routes" {
		t.Fatalf("filtered layers = %+v", layers)
	}
	start := layers[0].Features[0].Parts[0][0]
	if math.Abs(start[0]-42.358544) > 1e-6 || math.Abs(start[1]+71.05957) > 1e-6 {
		t.Errorf("z14 tile origin = %v, want 42.358544, -71.059570", start)
	}
}

func bytesField(num int, b []byte) []byte {
	out := binary.AppendUvarint(nil, uint64(num<<3|2))
	return append(binary.AppendUvarint(out, uint64(len(b))), b...)
}

func TestDecodeMalformed(t *testing.T) {
	badCommand := bytesField(3, bytesField(2, bytesField(4, []byte{5<<3 | 1})))
	for name, data := range map[string][]byte{
		"truncated":   testTile()[:20],
		"bad gzip":    {0x1f, 0x8b, 0x00},
		"bad command": badCommand,
	} {
		if _, err := mvt.Decode(data, 0, 0, 0); !errors.Is(err, mvt.ErrMalformed) {
			t.Errorf("%s: err = %v, want ErrMalformed", name, err)
		}
	}
	if layers, err := mvt.Decode(nil, 0, 0, 0); err != nil || len(layers) != 0 {
		t.Errorf("empty tile = %v, %v", layers, err)
	}
}
