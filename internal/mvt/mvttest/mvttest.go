package mvttest

import (
	"encoding/binary"
	"math"
	"sort"
)

type Feature struct {
	ID    uint64
	Type  int
	Props map[string]any
	Lines [][][2]int
}

type Layer struct {
	Name     string
	Features []Feature
}

func Tile(layers ...Layer) []byte {
	var out []byte
	for _, l := range layers {
		out = append(out, bytesField(3, layer(l))...)
	}
	return out
}

func layer(l Layer) []byte {
	var (
		keys   []string
		values []any
		keyIdx = map[string]int{}
		valIdx = map[any]int{}
		body   []byte
	)
	body = append(body, varintField(15, 2)...)
	body = append(body, bytesField(1, []byte(l.Name))...)
	for _, f := range l.Features {
		var tags []uint64
		names := make([]string, 0, len(f.Props))
		for k := range f.Props {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			ki, ok := keyIdx[k]
			if !ok {
				ki = len(keys)
				keyIdx[k] = ki
				keys = append(keys, k)
			}
			v := f.Props[k]
			vi, ok := valIdx[v]
			if !ok {
				vi = len(values)
				valIdx[v] = vi
				values = append(values, v)
			}
			tags = append(tags, uint64(ki), uint64(vi))
		}
		var geom []uint64
		var cx, cy int
		for _, line := range f.Lines {
			for i, p := range line {
				switch i {
				case 0:
					geom = append(geom, 1<<3|1)
				case 1:
					geom = append(geom, uint64(len(line)-1)<<3|2)
				}
				geom = append(geom, zigzag(p[0]-cx), zigzag(p[1]-cy))
				cx, cy = p[0], p[1]
			}
		}
		var feat []byte
		feat = append(feat, varintField(1, f.ID)...)
		feat = append(feat, packedField(2, tags)...)
		feat = append(feat, varintField(3, uint64(f.Type))...)
		feat = append(feat, packedField(4, geom)...)
		body = append(body, bytesField(2, feat)...)
	}
	for _, k := range keys {
		body = append(body, bytesField(3, []byte(k))...)
	}
	for _, v := range values {
		body = append(body, bytesField(4, value(v))...)
	}
	return append(body, varintField(5, 4096)...)
}

func value(v any) []byte {
	switch v := v.(type) {
	case string:
		return bytesField(1, []byte(v))
	case float64:
		return append(binary.AppendUvarint(nil, 3<<3|1), binary.LittleEndian.AppendUint64(nil, math.Float64bits(v))...)
	case int:
		return varintField(6, zigzag(v))
	case bool:
		if v {
			return varintField(7, 1)
		}
		return varintField(7, 0)
	}
	panic("mvttest: unsupported value type")
}

func zigzag(v int) uint64 {
	return uint64((int64(v) << 1) ^ (int64(v) >> 63))
}

func varintField(num int, v uint64) []byte {
	return binary.AppendUvarint(binary.AppendUvarint(nil, uint64(num<<3)), v)
}

func bytesField(num int, b []byte) []byte {
	out := binary.AppendUvarint(nil, uint64(num<<3|2))
	out = binary.AppendUvarint(out, uint64(len(b)))
	return append(out, b...)
}

func packedField(num int, vals []uint64) []byte {
	var p []byte
	for _, v := range vals {
		p = binary.AppendUvarint(p, v)
	}
	return bytesField(num, p)
}
