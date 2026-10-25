package overlays

import (
	"cmp"
	"encoding/json"
	"slices"

	"git.kwkaiser.io/kwkaiser/housing/internal/mvt"
)

type townDoc struct {
	Features []struct {
		Properties struct {
			State    string  `json:"STATE"`
			Place    string  `json:"PLACE"`
			Cousub   string  `json:"COUSUB"`
			Name     string  `json:"NAME"`
			AreaLand float64 `json:"AREALAND"`
		} `json:"properties"`
		Geometry *struct {
			Type        string          `json:"type"`
			Coordinates json.RawMessage `json:"coordinates"`
		} `json:"geometry"`
	} `json:"features"`
}

func Towns(docs ...[]byte) ([]Feature, error) {
	byID := map[string]bool{}
	var out []Feature
	for _, b := range docs {
		var doc townDoc
		if err := json.Unmarshal(b, &doc); err != nil {
			return nil, err
		}
		for _, f := range doc.Features {
			p := f.Properties
			code := cmp.Or(p.Place, p.Cousub)
			if p.State == "" || code == "" || f.Geometry == nil {
				continue
			}
			id := "town:" + p.State + code
			if byID[id] {
				continue
			}
			polys, err := polygons(f.Geometry.Type, f.Geometry.Coordinates)
			if err != nil {
				return nil, err
			}
			if len(polys) == 0 {
				continue
			}
			byID[id] = true
			out = append(out, Feature{ID: id, Label: cmp.Or(p.Name, "Town"), Area: p.AreaLand, Polygons: polys})
		}
	}
	slices.SortFunc(out, func(a, b Feature) int { return cmp.Or(cmp.Compare(b.Area, a.Area), cmp.Compare(a.ID, b.ID)) })
	return out, nil
}

func polygons(kind string, raw json.RawMessage) ([][][]mvt.LatLng, error) {
	var multi [][][][2]float64
	switch kind {
	case "Polygon":
		var poly [][][2]float64
		if err := json.Unmarshal(raw, &poly); err != nil {
			return nil, err
		}
		multi = [][][][2]float64{poly}
	case "MultiPolygon":
		if err := json.Unmarshal(raw, &multi); err != nil {
			return nil, err
		}
	default:
		return nil, nil
	}
	out := make([][][]mvt.LatLng, 0, len(multi))
	for _, poly := range multi {
		rings := make([][]mvt.LatLng, 0, len(poly))
		for _, ring := range poly {
			if len(ring) < 4 {
				continue
			}
			r := make([]mvt.LatLng, len(ring))
			for i, p := range ring {
				r[i] = mvt.LatLng{round5(p[1]), round5(p[0])}
			}
			rings = append(rings, r)
		}
		if len(rings) > 0 {
			out = append(out, rings)
		}
	}
	return out, nil
}
