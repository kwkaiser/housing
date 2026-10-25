package app

import (
	"context"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"git.kwkaiser.io/kwkaiser/housing/internal/geocode"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

type APIPlace struct {
	ID      int64     `json:"id"`
	Name    string    `json:"name"`
	Address string    `json:"address"`
	Lat     float64   `json:"lat"`
	Lng     float64   `json:"lng"`
	AddedAt time.Time `json:"added_at"`
}

type APIPlaceMatch struct {
	Name    string  `json:"name,omitempty" doc:"The place's own name, when it is more than a street address"`
	Address string  `json:"address"`
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
}

func apiPlace(p store.Place) APIPlace {
	return APIPlace{ID: p.ID, Name: p.Name, Address: p.Address, Lat: p.Lat, Lng: p.Lng, AddedAt: p.CreatedAt}
}

func (a *App) apiPlaces(ctx context.Context, _ *struct{}) (*struct{ Body []APIPlace }, error) {
	ps, err := a.svc.Places(ctx)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := make([]APIPlace, len(ps))
	for i, p := range ps {
		out[i] = apiPlace(p)
	}
	return &struct{ Body []APIPlace }{out}, nil
}

type addPlaceInput struct {
	Body struct {
		Name    string  `json:"name" minLength:"1" maxLength:"100"`
		Address string  `json:"address" maxLength:"300"`
		Lat     float64 `json:"lat" minimum:"-90" maximum:"90"`
		Lng     float64 `json:"lng" minimum:"-180" maximum:"180"`
	}
}

func (a *App) apiAddPlace(ctx context.Context, in *addPlaceInput) (*struct{ Body APIPlace }, error) {
	name := strings.TrimSpace(in.Body.Name)
	if name == "" {
		return nil, huma.Error422UnprocessableEntity("validation failed", &huma.ErrorDetail{Location: "body.name", Message: "name is required", Value: in.Body.Name})
	}
	p, err := a.svc.AddPlace(ctx, store.Place{Name: name, Address: strings.TrimSpace(in.Body.Address), Lat: in.Body.Lat, Lng: in.Body.Lng})
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	return &struct{ Body APIPlace }{apiPlace(p)}, nil
}

type placeInput struct {
	ID int64 `path:"id"`
}

func (a *App) apiRemovePlace(ctx context.Context, in *placeInput) (*struct{}, error) {
	if err := a.svc.RemovePlace(ctx, in.ID); err != nil {
		return nil, a.apiError(ctx, err)
	}
	return nil, nil
}

type searchPlacesInput struct {
	Q   string  `query:"q" maxLength:"200" doc:"An address or place name"`
	Lat float64 `query:"lat" minimum:"-90" maximum:"90" doc:"Prefer results near this latitude; give with lng"`
	Lng float64 `query:"lng" minimum:"-180" maximum:"180" doc:"Prefer results near this longitude; give with lat"`
}

func (a *App) apiSearchPlaces(ctx context.Context, in *searchPlacesInput) (*struct{ Body []APIPlaceMatch }, error) {
	var near *geocode.Near
	if in.Lat != 0 || in.Lng != 0 {
		near = &geocode.Near{Lat: in.Lat, Lng: in.Lng}
	}
	rs, err := a.geocoder.Search(ctx, in.Q, near, 6)
	if err != nil {
		a.log.WarnContext(ctx, "geocode", "err", err)
		return nil, huma.Error502BadGateway("address search unavailable")
	}
	out := make([]APIPlaceMatch, len(rs))
	for i, r := range rs {
		out[i] = APIPlaceMatch{Name: r.Name, Address: r.Address, Lat: r.Lat, Lng: r.Lng}
	}
	return &struct{ Body []APIPlaceMatch }{out}, nil
}
