package mvt

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"slices"
)

type GeomType int

const (
	Unknown GeomType = iota
	Point
	LineString
	Polygon
)

type LatLng [2]float64

type Feature struct {
	ID    uint64
	Type  GeomType
	Props map[string]any
	Parts [][]LatLng
}

type Layer struct {
	Name     string
	Features []Feature
}

var ErrMalformed = errors.New("malformed vector tile")

const (
	wireVarint = 0
	wire64     = 1
	wireBytes  = 2
	wire32     = 5
)

func Inflate(data []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return data, nil
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if data, err = io.ReadAll(r); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	return data, nil
}

func Decode(data []byte, z, x, y int, only ...string) ([]Layer, error) {
	data, err := Inflate(data)
	if err != nil {
		return nil, err
	}
	var layers []Layer
	err = fields(data, func(num int, wire int, v uint64, b []byte) error {
		if num != 3 || wire != wireBytes {
			return nil
		}
		if len(only) > 0 {
			name, err := layerName(b)
			if err != nil {
				return err
			}
			if !slices.Contains(only, name) {
				return nil
			}
		}
		l, err := decodeLayer(b, z, x, y)
		if err != nil {
			return err
		}
		layers = append(layers, l)
		return nil
	})
	return layers, err
}

func layerName(data []byte) (string, error) {
	var name string
	err := fields(data, func(num int, wire int, v uint64, b []byte) error {
		if num == 1 && wire == wireBytes {
			name = string(b)
		}
		return nil
	})
	return name, err
}

type rawFeature struct {
	id   uint64
	typ  GeomType
	tags []uint64
	geom []uint64
}

func decodeLayer(data []byte, z, x, y int) (Layer, error) {
	var (
		l      Layer
		keys   []string
		values []any
		raws   []rawFeature
		extent uint64 = 4096
	)
	err := fields(data, func(num int, wire int, v uint64, b []byte) error {
		switch {
		case num == 1 && wire == wireBytes:
			l.Name = string(b)
		case num == 2 && wire == wireBytes:
			f, err := decodeFeature(b)
			if err != nil {
				return err
			}
			raws = append(raws, f)
		case num == 3 && wire == wireBytes:
			keys = append(keys, string(b))
		case num == 4 && wire == wireBytes:
			val, err := decodeValue(b)
			if err != nil {
				return err
			}
			values = append(values, val)
		case num == 5 && wire == wireVarint:
			extent = v
		}
		return nil
	})
	if err != nil {
		return l, err
	}
	if extent == 0 {
		return l, fmt.Errorf("%w: zero extent", ErrMalformed)
	}
	proj := projector(z, x, y, float64(extent))
	for _, r := range raws {
		f := Feature{ID: r.id, Type: r.typ, Props: map[string]any{}}
		for i := 0; i+1 < len(r.tags); i += 2 {
			k, v := r.tags[i], r.tags[i+1]
			if k < uint64(len(keys)) && v < uint64(len(values)) {
				f.Props[keys[k]] = values[v]
			}
		}
		parts, err := decodeGeometry(r.geom, proj)
		if err != nil {
			return l, err
		}
		f.Parts = parts
		l.Features = append(l.Features, f)
	}
	return l, nil
}

func decodeFeature(data []byte) (rawFeature, error) {
	var f rawFeature
	err := fields(data, func(num int, wire int, v uint64, b []byte) error {
		switch {
		case num == 1 && wire == wireVarint:
			f.id = v
		case num == 2 && wire == wireBytes:
			var err error
			f.tags, err = packed(b)
			return err
		case num == 3 && wire == wireVarint:
			f.typ = GeomType(v)
		case num == 4 && wire == wireBytes:
			var err error
			f.geom, err = packed(b)
			return err
		}
		return nil
	})
	return f, err
}

func decodeValue(data []byte) (any, error) {
	var val any
	err := fields(data, func(num int, wire int, v uint64, b []byte) error {
		switch num {
		case 1:
			val = string(b)
		case 2:
			val = float64(math.Float32frombits(uint32(v)))
		case 3:
			val = math.Float64frombits(v)
		case 4:
			val = float64(int64(v))
		case 5:
			val = float64(v)
		case 6:
			val = float64(unzigzag(v))
		case 7:
			val = v != 0
		}
		return nil
	})
	return val, err
}

func decodeGeometry(cmds []uint64, proj func(px, py float64) LatLng) ([][]LatLng, error) {
	var (
		parts [][]LatLng
		cur   []LatLng
		cx    int64
		cy    int64
	)
	for i := 0; i < len(cmds); {
		id, count := cmds[i]&0x7, int(cmds[i]>>3)
		i++
		switch id {
		case 1, 2:
			if i+2*count > len(cmds) {
				return nil, fmt.Errorf("%w: truncated geometry", ErrMalformed)
			}
			for j := 0; j < count; j++ {
				cx += unzigzag(cmds[i])
				cy += unzigzag(cmds[i+1])
				i += 2
				if id == 1 && len(cur) > 0 {
					parts = append(parts, cur)
					cur = nil
				}
				cur = append(cur, proj(float64(cx), float64(cy)))
			}
		case 7:
			if len(cur) > 0 {
				cur = append(cur, cur[0])
			}
		default:
			return nil, fmt.Errorf("%w: unknown geometry command %d", ErrMalformed, id)
		}
	}
	if len(cur) > 0 {
		parts = append(parts, cur)
	}
	return parts, nil
}

func projector(z, x, y int, extent float64) func(px, py float64) LatLng {
	n := math.Exp2(float64(z))
	return func(px, py float64) LatLng {
		lng := (float64(x)+px/extent)/n*360 - 180
		lat := math.Atan(math.Sinh(math.Pi*(1-2*(float64(y)+py/extent)/n))) * 180 / math.Pi
		return LatLng{lat, lng}
	}
}

func unzigzag(v uint64) int64 {
	return int64(v>>1) ^ -int64(v&1)
}

func packed(b []byte) ([]uint64, error) {
	var out []uint64
	for len(b) > 0 {
		v, n := binary.Uvarint(b)
		if n <= 0 {
			return nil, fmt.Errorf("%w: bad varint", ErrMalformed)
		}
		out = append(out, v)
		b = b[n:]
	}
	return out, nil
}

func fields(b []byte, fn func(num int, wire int, v uint64, data []byte) error) error {
	for len(b) > 0 {
		key, n := binary.Uvarint(b)
		if n <= 0 {
			return fmt.Errorf("%w: bad field key", ErrMalformed)
		}
		b = b[n:]
		num, wire := int(key>>3), int(key&0x7)
		var (
			v    uint64
			data []byte
		)
		switch wire {
		case wireVarint:
			v, n = binary.Uvarint(b)
			if n <= 0 {
				return fmt.Errorf("%w: bad varint", ErrMalformed)
			}
			b = b[n:]
		case wire64:
			if len(b) < 8 {
				return fmt.Errorf("%w: truncated fixed64", ErrMalformed)
			}
			v, b = binary.LittleEndian.Uint64(b), b[8:]
		case wire32:
			if len(b) < 4 {
				return fmt.Errorf("%w: truncated fixed32", ErrMalformed)
			}
			v, b = uint64(binary.LittleEndian.Uint32(b)), b[4:]
		case wireBytes:
			l, n := binary.Uvarint(b)
			if n <= 0 || uint64(len(b)-n) < l {
				return fmt.Errorf("%w: truncated bytes", ErrMalformed)
			}
			data, b = b[n:n+int(l)], b[n+int(l):]
		default:
			return fmt.Errorf("%w: unsupported wire type %d", ErrMalformed, wire)
		}
		if err := fn(num, wire, v, data); err != nil {
			return err
		}
	}
	return nil
}
