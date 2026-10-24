package app

import (
	"cmp"
	"context"
	"errors"

	"github.com/danielgtaylor/huma/v2"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
)

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
	Preview     string                 `json:"preview,omitempty" doc:"API path of the listing's first collage"`
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
	d, err := a.mapData(ctx, cs, from, to)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := APIMapPins{From: from, To: to, FirstDay: d.RunFirst, LastDay: d.RunLast, Collections: []APIMapCollection{}, Pins: []APIMapPin{}, Unplaced: d.Unplaced}
	for _, c := range d.Collections {
		mc := APIMapCollection{ID: c.ID, Color: c.Color, LatestDay: c.Day, Graded: c.Count}
		if e, ok := d.Extent[c.ID]; ok {
			mc.Area = &APIBounds{South: e[0][0], West: e[0][1], North: e[1][0], East: e[1][1]}
		}
		out.Collections = append(out.Collections, mc)
	}
	for _, p := range d.Pins {
		l := p.listing
		pin := APIMapPin{
			Source: l.Source, SourceID: l.SourceID, Href: apiListingPath(l.Source, l.SourceID), Lat: p.Lat, Lng: p.Lng, Approx: p.Approx,
			Address: cmp.Or(report.AddressLine(l), l.SourceID), Offer: l.Offer, PriceCents: l.Price.Cents, Beds: l.Beds,
			FirstSeen: p.FirstSeen, LastSeen: p.LastSeen, Collections: p.Collections, Grades: p.grades, Ungraded: p.Ungraded,
		}
		if len(l.Collages) > 0 {
			pin.Preview = apiV1 + "/media/" + l.Collages[0]
		}
		out.Pins = append(out.Pins, pin)
	}
	return &struct{ Body APIMapPins }{out}, nil
}

type APIMapConfig struct {
	Positron *APITileLayer `json:"positron,omitempty" doc:"A cached CARTO Positron base layer, when the server has a CARTO key"`
	Overlays []APIOverlay  `json:"overlays"`
}

type APITileLayer struct {
	URL     string `json:"url" doc:"Tile URL template; {r} is empty or @2x for retina tiles"`
	MaxZoom int    `json:"max_zoom"`
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

func (a *App) apiOverlayTile(ctx context.Context, in *overlayInput) (*tileOutput, error) {
	b, ctype, err := a.overlayData(ctx, in.Name, in.Z, in.X, in.File, true)
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
