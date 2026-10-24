package geo

import (
	"math"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const earthRadiusMiles = 3958.8

type Bounds struct {
	South float64
	West  float64
	North float64
	East  float64
}

func BoundsAround(c listing.Coordinates, radiusMiles float64) Bounds {
	dLat := radiusMiles / earthRadiusMiles * 180 / math.Pi
	dLng := dLat / math.Cos(c.Lat*math.Pi/180)
	return Bounds{South: c.Lat - dLat, West: c.Lng - dLng, North: c.Lat + dLat, East: c.Lng + dLng}
}

func DistanceMiles(a, b listing.Coordinates) float64 {
	rad := func(d float64) float64 { return d * math.Pi / 180 }
	dLat := rad(b.Lat - a.Lat)
	dLng := rad(b.Lng - a.Lng)
	h := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(rad(a.Lat))*math.Cos(rad(b.Lat))*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 2 * earthRadiusMiles * math.Asin(math.Sqrt(h))
}
