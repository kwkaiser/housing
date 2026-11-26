package service

import (
	"context"
	"log/slog"
	"strings"

	"go.opentelemetry.io/otel/attribute"

	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/geocode"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/telemetry"
)

const geocodeMaxMiles = 3

type Geocoder interface {
	Search(ctx context.Context, query string, near *geocode.Near, limit int) ([]geocode.Result, error)
}

func (s *Service) locate(ctx context.Context, ls []listing.Listing, log *slog.Logger) {
	ctx, span := telemetry.Start(ctx, "service.locate")
	defer span.End()
	var tried, found int
	for i := range ls {
		l := &ls[i]
		if geo.Placed(*l) {
			continue
		}
		query, number := geocodeQuery(l.Address)
		if query == "" {
			continue
		}
		tried++
		c, ok, err := Geocode(ctx, s.cfg.Clients.Geocoder, query, number, l.Address)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			log.WarnContext(ctx, "geocode failed", "source", l.Source, "id", l.SourceID, "err", err)
			continue
		}
		if ok {
			l.Coordinates = &c
			found++
		}
	}
	span.SetAttributes(attribute.Int("tried", tried), attribute.Int("found", found))
	if tried > 0 {
		log.InfoContext(ctx, "geocoded listings without coordinates", "tried", tried, "found", found)
	}
}

func Geocode(ctx context.Context, g Geocoder, query, number string, a listing.Address) (listing.Coordinates, bool, error) {
	var near *geocode.Near
	center, anchored := geo.ZipCentroid(geo.PostalCode(a))
	if anchored {
		near = &geocode.Near{Lat: center.Lat, Lng: center.Lng}
	}
	rs, err := g.Search(ctx, query, near, 3)
	if err != nil {
		return listing.Coordinates{}, false, err
	}
	for _, r := range rs {
		if !strings.HasPrefix(r.Address, number+" ") {
			continue
		}
		c := listing.Coordinates{Lat: r.Lat, Lng: r.Lng}
		if anchored && geo.DistanceMiles(c, center) > geocodeMaxMiles {
			continue
		}
		return c, true, nil
	}
	return listing.Coordinates{}, false, nil
}

func geocodeQuery(a listing.Address) (query, number string) {
	street := strings.TrimSpace(a.Street)
	if street == "" {
		street, _, _ = strings.Cut(a.Formatted, ",")
		street = strings.TrimSpace(street)
	}
	number, _, _ = strings.Cut(street, " ")
	if number == "" || number[0] < '0' || number[0] > '9' {
		return "", ""
	}
	if a.Street == "" || a.City == "" && a.PostalCode == "" {
		return strings.TrimSpace(a.Formatted), number
	}
	var parts []string
	for _, p := range []string{street, a.City, strings.TrimSpace(a.State + " " + a.PostalCode)} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", "), number
}
