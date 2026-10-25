package app

import (
	"cmp"
	"context"
	"errors"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
)

var mapColors = []string{"#2563eb", "#dc2626", "#16a34a", "#9333ea", "#ea580c", "#0891b2", "#db2777", "#65a30d"}

const defaultAreaMiles = 2

type mapPinsInput struct {
	From string `query:"from" format:"date" doc:"Only runs on or after this day; empty for no lower bound"`
	To   string `query:"to" format:"date" doc:"Only runs on or before this day; empty for no upper bound"`
}

type APIMapPins struct {
	From        string             `json:"from,omitempty"`
	To          string             `json:"to,omitempty"`
	FirstDay    string             `json:"first_day,omitempty" doc:"The earliest run in range across all collections"`
	LastDay     string             `json:"last_day,omitempty" doc:"The latest run in range across all collections"`
	Collections []APIMapCollection `json:"collections"`
	Pins        []APIMapPin        `json:"pins" doc:"One pin per distinct listing; listings without a location are left out"`
	Unplaced    int                `json:"unplaced" doc:"Listings left out for having no location"`
}

type APIMapCollection struct {
	ID        string     `json:"id"`
	Color     string     `json:"color" doc:"A suggested color for the collection's pins"`
	LatestDay string     `json:"latest_day,omitempty" doc:"The collection's latest run in range"`
	Graded    int        `json:"graded" doc:"Pins graded by this collection without hitting a dealbreaker"`
	Area      *APIBounds `json:"area,omitempty" doc:"The collection's search area, when its location is a known zip code"`
}

type APIBounds struct {
	South float64 `json:"south"`
	West  float64 `json:"west"`
	North float64 `json:"north"`
	East  float64 `json:"east"`
}

type APIMapPin struct {
	Source      listing.Source         `json:"source"`
	SourceID    string                 `json:"source_id"`
	Href        string                 `json:"href" doc:"This listing's API path"`
	Lat         float64                `json:"lat"`
	Lng         float64                `json:"lng"`
	Approx      bool                   `json:"approx,omitempty" doc:"Placed at the zip code's center"`
	Address     string                 `json:"address"`
	Offer       listing.OfferType      `json:"offer"`
	PriceCents  int64                  `json:"price_cents"`
	Beds        *int                   `json:"beds,omitempty"`
	Preview     string                 `json:"preview,omitempty" doc:"API path of the listing's first stored photo, or else its first collage"`
	FirstSeen   string                 `json:"first_seen"`
	LastSeen    string                 `json:"last_seen"`
	Collections []string               `json:"collections" doc:"Collections that saw the listing, in collection order"`
	Grades      map[string]APIPinGrade `json:"grades" doc:"The listing's best grade in each collection that graded it"`
	Ungraded    bool                   `json:"ungraded,omitempty" doc:"No collection has graded the listing yet"`
}

type APIPinGrade struct {
	Match        float64  `json:"match"`
	Calibrated   bool     `json:"calibrated"`
	Stale        bool     `json:"stale,omitempty"`
	Dealbreakers []string `json:"dealbreakers,omitempty"`
}

func (a *App) apiMapPins(ctx context.Context, in *mapPinsInput) (*struct{ Body APIMapPins }, error) {
	cs, err := a.svc.Collections(ctx)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	from, to := in.From, in.To
	if from != "" && to != "" && from > to {
		from, to = to, from
	}
	out, err := a.mapPins(ctx, cs, from, to)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	return &struct{ Body APIMapPins }{out}, nil
}

func (a *App) mapPins(ctx context.Context, cs []collection.Collection, from, to string) (APIMapPins, error) {
	out := APIMapPins{From: from, To: to, Collections: []APIMapCollection{}, Pins: []APIMapPin{}}
	seen := map[string]int{}
	for i, c := range cs {
		v, err := a.svc.CollectionRange(ctx, c.ID, from, to)
		if err != nil {
			return out, err
		}
		mc := APIMapCollection{ID: c.ID, Color: mapColors[i%len(mapColors)]}
		if n := len(v.Days); n > 0 {
			mc.LatestDay = v.Days[n-1].Day
			if out.FirstDay == "" || v.Days[0].Day < out.FirstDay {
				out.FirstDay = v.Days[0].Day
			}
			out.LastDay = max(out.LastDay, mc.LatestDay)
		}
		mc.Area = searchArea(c)
		for _, row := range v.Rows {
			l := row.Listing
			key := string(l.Source) + "/" + l.SourceID
			j, ok := seen[key]
			if !ok {
				loc, approx, placed := geo.Locate(l)
				if !placed {
					seen[key] = -1
					out.Unplaced++
					continue
				}
				j = len(out.Pins)
				seen[key] = j
				pin := APIMapPin{
					Source: l.Source, SourceID: l.SourceID, Href: apiListingPath(l.Source, l.SourceID), Lat: loc.Lat, Lng: loc.Lng, Approx: approx,
					Address: cmp.Or(report.AddressLine(l), l.SourceID), Offer: l.Offer, PriceCents: l.Price.Cents, Beds: l.Beds,
					FirstSeen: row.FirstSeen, LastSeen: row.Day, Grades: map[string]APIPinGrade{}, Ungraded: row.Ungraded,
				}
				if pin.Preview = a.thumbnail(ctx, l); pin.Preview == "" && len(l.Collages) > 0 {
					pin.Preview = apiV1 + "/media/" + l.Collages[0]
				}
				out.Pins = append(out.Pins, pin)
			} else if j < 0 {
				continue
			}
			p := &out.Pins[j]
			p.Collections = append(p.Collections, c.ID)
			p.FirstSeen = min(p.FirstSeen, row.FirstSeen)
			p.LastSeen = max(p.LastSeen, row.Day)
			if !row.Ungraded {
				p.Ungraded = false
				p.Grades[c.ID] = APIPinGrade{Match: row.Match, Calibrated: row.Calibrated, Stale: row.Stale, Dealbreakers: row.Dealbreakers}
				if len(row.Dealbreakers) == 0 {
					mc.Graded++
				}
			}
		}
		out.Collections = append(out.Collections, mc)
	}
	return out, nil
}

func searchArea(c collection.Collection) *APIBounds {
	center, ok := geo.ZipCentroid(strings.TrimSpace(c.Search.Location))
	if !ok {
		return nil
	}
	b := geo.BoundsAround(center, cmp.Or(c.Search.RadiusMiles, defaultAreaMiles))
	return &APIBounds{South: b.South, West: b.West, North: b.North, East: b.East}
}

type APIMapConfig struct {
	Positron *APITileLayer `json:"positron,omitempty" doc:"A cached CARTO Positron base layer, when the server has a CARTO key"`
	Terrain  *APIDEMLayer  `json:"terrain,omitempty" doc:"A cached elevation layer for hillshading"`
	Overlays []APIOverlay  `json:"overlays"`
}

type APITileLayer struct {
	URL     string `json:"url" doc:"Tile URL template; {r} is empty or @2x for retina tiles"`
	MaxZoom int    `json:"max_zoom"`
}

type APIDEMLayer struct {
	URL      string `json:"url" doc:"Tile URL template"`
	MaxZoom  int    `json:"max_zoom" doc:"Tiles stop here; clients overzoom beyond it"`
	TileSize int    `json:"tile_size"`
	Encoding string `json:"encoding" enum:"terrarium"`
}

type APIOverlay struct {
	Name      string `json:"name" enum:"bike,rail,bus"`
	MinZoom   int    `json:"min_zoom"`
	MaxZoom   int    `json:"max_zoom" doc:"Tiles stop here; clients overzoom beyond it"`
	URL       string `json:"url" doc:"Template for tiles of simplified line features as JSON"`
	VectorURL string `json:"vector_url" doc:"Template for the source's raw Mapbox vector tiles"`
}

func (a *App) apiMapConfig(_ context.Context, _ *struct{}) (*struct{ Body APIMapConfig }, error) {
	c := a.mapConfig()
	out := APIMapConfig{Overlays: []APIOverlay{}}
	if c.Positron {
		out.Positron = &APITileLayer{URL: apiV1 + "/tiles/positron/{z}/{x}/{y}{r}.png", MaxZoom: 20}
	}
	if a.terrain.Enabled() {
		out.Terrain = &APIDEMLayer{URL: apiV1 + "/tiles/terrain/{z}/{x}/{y}.webp", MaxZoom: a.terrain.MaxZoom, TileSize: 512, Encoding: "terrarium"}
	}
	for _, o := range c.Overlays {
		base := apiV1 + "/overlays/" + o.Name + "/{z}/{x}/{y}"
		out.Overlays = append(out.Overlays, APIOverlay{Name: o.Name, MinZoom: o.MinZoom, MaxZoom: o.MaxZoom, URL: base + ".json", VectorURL: base + ".pbf"})
	}
	return &struct{ Body APIMapConfig }{out}, nil
}

type tileInput struct {
	Z    string `path:"z"`
	X    string `path:"x"`
	File string `path:"file" doc:"{y}.png, or {y}@2x.png for retina"`
}

type overlayInput struct {
	Name string `path:"name" enum:"bike,rail,bus"`
	Z    string `path:"z"`
	X    string `path:"x"`
	File string `path:"file" doc:"{y}.json for simplified line features, or {y}.pbf for the raw vector tile"`
}

type tileOutput struct {
	ContentType  string `header:"Content-Type"`
	CacheControl string `header:"Cache-Control"`
	Body         []byte
}

func (a *App) apiPositronTile(ctx context.Context, in *tileInput) (*tileOutput, error) {
	b, err := a.positronPNG(ctx, in.Z, in.X, in.File)
	if err != nil {
		return nil, tileError(err)
	}
	return &tileOutput{ContentType: "image/png", CacheControl: "private, max-age=86400", Body: b}, nil
}

type terrainTileInput struct {
	Z    string `path:"z"`
	X    string `path:"x"`
	File string `path:"file" doc:"{y}.webp"`
}

func (a *App) apiTerrainTile(ctx context.Context, in *terrainTileInput) (*tileOutput, error) {
	b, err := a.terrainWebP(ctx, in.Z, in.X, in.File)
	if err != nil {
		return nil, tileError(err)
	}
	return &tileOutput{ContentType: "image/webp", CacheControl: "private, max-age=86400", Body: b}, nil
}

func (a *App) apiOverlayTile(ctx context.Context, in *overlayInput) (*tileOutput, error) {
	b, ctype, err := a.overlayData(ctx, in.Name, in.Z, in.X, in.File)
	if err != nil {
		return nil, tileError(err)
	}
	return &tileOutput{ContentType: ctype, CacheControl: "private, max-age=86400", Body: b}, nil
}

func tileError(err error) error {
	if errors.Is(err, errNoTile) {
		return huma.Error404NotFound("no such tile")
	}
	return huma.Error502BadGateway("tile unavailable")
}
