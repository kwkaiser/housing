package overlays

import (
	"cmp"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/mvt"
)

type Feature struct {
	ID       string           `json:"id"`
	Label    string           `json:"label"`
	Detail   string           `json:"detail,omitempty"`
	Color    string           `json:"color,omitempty"`
	Headway  int              `json:"headway,omitempty"`
	Area     float64          `json:"area,omitempty"`
	Lines    [][]mvt.LatLng   `json:"lines,omitempty"`
	Polygons [][][]mvt.LatLng `json:"polygons,omitempty"`
}

type Kind struct {
	Layer string
	pick  func(props map[string]any) (Feature, bool)
}

var (
	Bus       = Kind{Layer: "routes", pick: transit(modeBus)}
	Rail      = Kind{Layer: "routes", pick: transit(modeRail)}
	Bike      = Kind{Layer: "transportation_name", pick: bike}
	BikePaths = Kind{Layer: "transportation", pick: bikePath}
)

func (k Kind) Collect(layers []mvt.Layer) []Feature {
	byID := map[string]*Feature{}
	for _, l := range layers {
		if l.Name != k.Layer {
			continue
		}
		for _, f := range l.Features {
			if f.Type != mvt.LineString {
				continue
			}
			of, ok := k.pick(f.Props)
			if !ok {
				continue
			}
			lines := roundLines(f.Parts)
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
	out := make([]Feature, 0, len(byID))
	for _, f := range byID {
		out = append(out, *f)
	}
	slices.SortFunc(out, func(a, b Feature) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

func roundLines(parts [][]mvt.LatLng) [][]mvt.LatLng {
	lines := make([][]mvt.LatLng, 0, len(parts))
	for _, part := range parts {
		if len(part) < 2 {
			continue
		}
		line := make([]mvt.LatLng, len(part))
		for i, p := range part {
			line[i] = mvt.LatLng{round5(p[0]), round5(p[1])}
		}
		lines = append(lines, line)
	}
	return lines
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
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		return f, err == nil
	}
	return 0, false
}

type mode int

const (
	modeOther mode = iota
	modeBus
	modeRail
)

func transitMode(routeType float64) mode {
	rt := int(routeType)
	switch {
	case rt == 3 || rt == 11 || rt >= 200 && rt < 300 || rt >= 700 && rt < 800:
		return modeBus
	case rt == 0 || rt == 1 || rt == 2 || rt == 5 || rt == 7 || rt == 12 || rt >= 100 && rt < 200 || rt >= 400 && rt < 500 || rt >= 900 && rt < 1000:
		return modeRail
	}
	return modeOther
}

const irregularHeadway = 24 * 60 * 60

var hexColor = regexp.MustCompile(`^#?([0-9a-fA-F]{6})$`)

func routeColor(raw string) string {
	m := hexColor.FindStringSubmatch(raw)
	if m == nil {
		return ""
	}
	v, _ := strconv.ParseUint(m[1], 16, 32)
	r, g, b := float64(v>>16&0xff), float64(v>>8&0xff), float64(v&0xff)
	if (0.2126*r+0.7152*g+0.0722*b)/255 > 0.8 {
		return ""
	}
	return "#" + strings.ToLower(m[1])
}

func routeLabel(short, long string) string {
	switch {
	case short != "" && long != "" && !strings.Contains(long, short):
		return short + " · " + long
	case long != "":
		return long
	}
	return cmp.Or(short, "Route")
}

func transit(want mode) func(map[string]any) (Feature, bool) {
	return func(p map[string]any) (Feature, bool) {
		rt, ok := num(p, "route_type")
		if !ok || transitMode(rt) != want {
			return Feature{}, false
		}
		id := cmp.Or(str(p, "onestop_id"), str(p, "route_id"))
		if id == "" {
			return Feature{}, false
		}
		f := Feature{
			ID:     id,
			Label:  routeLabel(str(p, "route_short_name"), str(p, "route_long_name")),
			Detail: str(p, "agency_name"),
			Color:  routeColor(str(p, "route_color")),
		}
		if h, ok := num(p, "headway_secs"); ok && h > 0 {
			switch {
			case h < irregularHeadway:
				f.Headway = int(h)
			case want == modeBus:
				return Feature{}, false
			}
		}
		return f, true
	}
}

var trailName = regexp.MustCompile(`(?i)\b(path|trail|bikeway|greenway|rail ?trail|bike)\b`)

func bike(p map[string]any) (Feature, bool) {
	name := str(p, "name")
	if str(p, "class") != "path" || name == "" {
		return Feature{}, false
	}
	switch str(p, "subclass") {
	case "cycleway":
	case "path", "footway", "track":
		if !trailName.MatchString(name) {
			return Feature{}, false
		}
	default:
		return Feature{}, false
	}
	return Feature{ID: "bike:" + name, Label: name, Detail: "Bike path"}, true
}

func bikePath(p map[string]any) (Feature, bool) {
	if str(p, "class") != "path" {
		return Feature{}, false
	}
	if str(p, "subclass") != "cycleway" && str(p, "bicycle") != "designated" {
		return Feature{}, false
	}
	return Feature{ID: "bikeway", Label: "Bike path"}, true
}
