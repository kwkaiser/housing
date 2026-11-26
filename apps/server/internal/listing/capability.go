package listing

import (
	"maps"
	"slices"
)

type Support string

const (
	SupportSearch  Support = "search"
	SupportResults Support = "results"
	SupportEnrich  Support = "enrich"
)

type Capabilities map[Amenity]Support

func NewCapabilities(search, results []Amenity) Capabilities {
	out := Capabilities{}
	for _, a := range results {
		out[a] = SupportResults
	}
	for _, a := range search {
		out[a] = SupportSearch
	}
	return out
}

func (c Capabilities) Searchable() []Amenity {
	var out []Amenity
	for a, s := range c {
		if s == SupportSearch {
			out = append(out, a)
		}
	}
	slices.Sort(out)
	return out
}

func OfferAmenities(offer OfferType) []Amenity {
	if offer == OfferRent {
		return Amenities
	}
	return slices.DeleteFunc(slices.Clone(Amenities), Amenity.RentOnly)
}

func CapabilitiesOf(p Provider, offer OfferType) Capabilities {
	out := maps.Clone(p.Capabilities(offer))
	if out == nil {
		out = Capabilities{}
	}
	if e, ok := p.(Enricher); ok {
		for _, a := range e.Enriches(offer) {
			if _, ok := out[a]; !ok {
				out[a] = SupportEnrich
			}
		}
	}
	maps.DeleteFunc(out, func(a Amenity, _ Support) bool { return offer != OfferRent && a.RentOnly() })
	return out
}
