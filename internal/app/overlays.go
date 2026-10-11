package app

import (
	"cmp"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/mvt"
	"git.kwkaiser.io/kwkaiser/housing/internal/tiles"
)

type overlay struct {
	source  *tiles.Cache
	minZoom int
	pick    func(layer string, f mvt.Feature) (overlayFeature, bool)
}

type overlayFeature struct {
	ID      string         `json:"id"`
	Label   string         `json:"label"`
	Detail  string         `json:"detail,omitempty"`
	Color   string         `json:"color,omitempty"`
	Headway int            `json:"headway,omitempty"`
	Lines   [][]mvt.LatLng `json:"lines"`
}

func newOverlays(transit, openFreeMap *tiles.Cache) map[string]overlay {
	return map[string]overlay{
		"bus":  {source: transit, minZoom: 12, pick: pickTransit("bus")},
		"rail": {source: transit, minZoom: 9, pick: pickTransit("rail")},
		"bike": {source: openFreeMap, minZoom: 13, pick: pickBike},
	}
}

func (a *App) overlayTile(w http.ResponseWriter, r *http.Request) {
	ov, ok := a.overlays[r.PathValue("name")]
	t, err := tiles.Parse(r.PathValue("z"), r.PathValue("x"), r.PathValue("file"), ".json")
	if !ok || err != nil || !ov.source.Enabled() || t.Z < ov.minZoom || t.Z > ov.source.MaxZoom {
		a.notFound(w, r)
		return
	}
	b, err := ov.source.Get(r.Context(), t)
	if err != nil {
		a.log.Warn("overlay tile", "overlay", r.PathValue("name"), "z", t.Z, "x", t.X, "y", t.Y, "err", err)
		http.Error(w, "overlay unavailable", http.StatusBadGateway)
		return
	}
	layers, err := mvt.Decode(b, t.Z, t.X, t.Y)
	if err != nil {
		a.log.Warn("overlay decode", "overlay", r.PathValue("name"), "z", t.Z, "x", t.X, "y", t.Y, "err", err)
		http.Error(w, "overlay unavailable", http.StatusBadGateway)
		return
	}
	body, err := json.Marshal(struct {
		Features []overlayFeature `json:"features"`
	}{collect(layers, ov.pick)})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(body)
}

func collect(layers []mvt.Layer, pick func(string, mvt.Feature) (overlayFeature, bool)) []overlayFeature {
	byID := map[string]*overlayFeature{}
	for _, l := range layers {
		for _, f := range l.Features {
			if f.Type != mvt.LineString {
				continue
			}
			of, ok := pick(l.Name, f)
			if !ok {
				continue
			}
			lines := make([][]mvt.LatLng, 0, len(f.Parts))
			for _, part := range f.Parts {
				if len(part) < 2 {
					continue
				}
				line := make([]mvt.LatLng, len(part))
				for i, p := range part {
					line[i] = mvt.LatLng{round5(p[0]), round5(p[1])}
				}
				lines = append(lines, line)
			}
			if len(lines) == 0 {
				continue
			}
			if prev, ok := byID[of.ID]; ok {
				prev.Lines = append(prev.Lines, lines...)
				continue
			}
			of.Lines = lines
			byID[of.ID] = &of
		}
	}
	out := make([]overlayFeature, 0, len(byID))
	for _, f := range byID {
		out = append(out, *f)
	}
	slices.SortFunc(out, func(a, b overlayFeature) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

func round5(f float64) float64 {
	return math.Round(f*1e5) / 1e5
}

func str(p map[string]any, key string) string {
	s, _ := p[key].(string)
	return strings.TrimSpace(s)
}

func num(p map[string]any, key string) (float64, bool) {
	switch v := p[key].(type) {
	case float64:
		return v, true
	case string:
		var f float64
		_, err := fmt.Sscan(v, &f)
		return f, err == nil
	}
	return 0, false
}

func transitMode(routeType float64) string {
	rt := int(routeType)
	switch {
	case rt == 3 || rt == 11 || rt >= 200 && rt < 300 || rt >= 700 && rt < 800:
		return "bus"
	case rt == 0 || rt == 1 || rt == 2 || rt == 5 || rt == 7 || rt == 12 || rt >= 100 && rt < 200 || rt >= 400 && rt < 500 || rt >= 900 && rt < 1000:
		return "rail"
	}
	return ""
}

const irregularHeadway = 24 * 60 * 60

var hexColor = regexp.MustCompile(`^#?([0-9a-fA-F]{6})$`)

func pale(hex string) bool {
	v, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return true
	}
	r, g, b := float64(v>>16&0xff), float64(v>>8&0xff), float64(v&0xff)
	return (0.2126*r+0.7152*g+0.0722*b)/255 > 0.8
}

func pickTransit(mode string) func(string, mvt.Feature) (overlayFeature, bool) {
	return func(layer string, f mvt.Feature) (overlayFeature, bool) {
		rt, ok := num(f.Props, "route_type")
		if layer != "routes" || !ok || transitMode(rt) != mode {
			return overlayFeature{}, false
		}
		id := cmp.Or(str(f.Props, "onestop_id"), str(f.Props, "route_id"))
		if id == "" {
			return overlayFeature{}, false
		}
		short, long := str(f.Props, "route_short_name"), str(f.Props, "route_long_name")
		label := cmp.Or(short, long, "Route")
		if short != "" && long != "" && !strings.Contains(long, short) {
			label = short + " · " + long
		} else if long != "" {
			label = long
		}
		of := overlayFeature{ID: id, Label: label, Detail: str(f.Props, "agency_name")}
		if m := hexColor.FindStringSubmatch(str(f.Props, "route_color")); m != nil && !pale(m[1]) {
			of.Color = "#" + strings.ToLower(m[1])
		}
		if h, ok := num(f.Props, "headway_secs"); ok && h > 0 {
			if h >= irregularHeadway {
				if mode == "bus" {
					return overlayFeature{}, false
				}
			} else {
				of.Headway = int(h)
			}
		}
		return of, true
	}
}

var trailName = regexp.MustCompile(`(?i)\b(path|trail|bikeway|greenway|rail ?trail|bike)\b`)

func pickBike(layer string, f mvt.Feature) (overlayFeature, bool) {
	name := str(f.Props, "name")
	if layer != "transportation_name" || str(f.Props, "class") != "path" || name == "" {
		return overlayFeature{}, false
	}
	switch str(f.Props, "subclass") {
	case "cycleway":
	case "path", "footway", "track":
		if !trailName.MatchString(name) {
			return overlayFeature{}, false
		}
	default:
		return overlayFeature{}, false
	}
	return overlayFeature{ID: "bike:" + name, Label: name, Detail: "Bike path"}, true
}
