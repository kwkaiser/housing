package mvt

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"math"
	"testing"
)

func varint(v uint64) []byte {
	return binary.AppendUvarint(nil, v)
}

func field(num int, wire int, payload []byte) []byte {
	b := varint(uint64(num<<3 | wire))
	if wire == wireBytes {
		b = append(b, varint(uint64(len(payload)))...)
	}
	return append(b, payload...)
}

func packedField(num int, vals ...uint64) []byte {
	var p []byte
	for _, v := range vals {
		p = append(p, varint(v)...)
	}
	return field(num, wireBytes, p)
}

func zigzag(v int64) uint64 {
	return uint64((v << 1) ^ (v >> 63))
}

func cmd(id, count int) uint64 {
	return uint64(count<<3 | id)
}

func testTile() []byte {
	var feature []byte
	feature = append(feature, field(1, wireVarint, varint(7))...)
	feature = append(feature, packedField(2, 0, 0, 1, 1, 2, 2)...)
	feature = append(feature, field(3, wireVarint, varint(uint64(LineString)))...)
	feature = append(feature, packedField(4,
		cmd(1, 1), zigzag(0), zigzag(0),
		cmd(2, 1), zigzag(4096), zigzag(4096),
		cmd(1, 1), zigzag(-2048), zigzag(-2048),
		cmd(2, 1), zigzag(0), zigzag(2048),
	)...)

	var layer []byte
	layer = append(layer, field(15, wireVarint, varint(2))...)
	layer = append(layer, field(1, wireBytes, []byte("routes"))...)
	layer = append(layer, field(2, wireBytes, feature)...)
	layer = append(layer, field(3, wireBytes, []byte("name"))...)
	layer = append(layer, field(3, wireBytes, []byte("headway"))...)
	layer = append(layer, field(3, wireBytes, []byte("night"))...)
	layer = append(layer, field(4, wireBytes, field(1, wireBytes, []byte("Red Line")))...)
	layer = append(layer, field(4, wireBytes, field(6, wireVarint, varint(zigzag(600))))...)
	layer = append(layer, field(4, wireBytes, field(7, wireVarint, varint(1)))...)
	layer = append(layer, field(5, wireVarint, varint(4096))...)

	return field(3, wireBytes, layer)
}

func near(a, b LatLng) bool {
	return math.Abs(a[0]-b[0]) < 1e-6 && math.Abs(a[1]-b[1]) < 1e-6
}

func TestDecode(t *testing.T) {
	var gz bytes.Buffer
	w := gzip.NewWriter(&gz)
	w.Write(testTile())
	w.Close()

	for name, data := range map[string][]byte{"plain": testTile(), "gzip": gz.Bytes()} {
		layers, err := Decode(data, 0, 0, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(layers) != 1 || layers[0].Name != "routes" || len(layers[0].Features) != 1 {
			t.Fatalf("%s: layers = %+v", name, layers)
		}
		f := layers[0].Features[0]
		if f.ID != 7 || f.Type != LineString {
			t.Errorf("%s: feature = %+v", name, f)
		}
		if f.Props["name"] != "Red Line" || f.Props["headway"] != 600.0 || f.Props["night"] != true {
			t.Errorf("%s: props = %v", name, f.Props)
		}
		if len(f.Parts) != 2 || len(f.Parts[0]) != 2 || len(f.Parts[1]) != 2 {
			t.Fatalf("%s: parts = %v", name, f.Parts)
		}
		top := 85.0511287798
		for i, want := range []LatLng{{top, -180}, {-top, 180}, {0, 0}, {-top, 0}} {
			if got := f.Parts[i/2][i%2]; !near(got, want) {
				t.Errorf("%s: point %d = %v, want %v", name, i, got, want)
			}
		}
	}

	layers, err := Decode(testTile(), 14, 4958, 6060)
	if err != nil {
		t.Fatal(err)
	}
	start := layers[0].Features[0].Parts[0][0]
	if math.Abs(start[0]-42.358544) > 1e-6 || math.Abs(start[1]+71.05957) > 1e-6 {
		t.Errorf("z14 tile origin = %v, want 42.358544, -71.059570", start)
	}
}

func TestDecodeMalformed(t *testing.T) {
	for name, data := range map[string][]byte{
		"truncated":   testTile()[:20],
		"bad gzip":    {0x1f, 0x8b, 0x00},
		"bad command": field(3, wireBytes, append(field(2, wireBytes, packedField(4, cmd(5, 1))), field(5, wireVarint, varint(4096))...)),
	} {
		if _, err := Decode(data, 0, 0, 0); !errors.Is(err, ErrMalformed) {
			t.Errorf("%s: err = %v, want ErrMalformed", name, err)
		}
	}
	if layers, err := Decode(nil, 0, 0, 0); err != nil || len(layers) != 0 {
		t.Errorf("empty tile = %v, %v", layers, err)
	}
}
