package dedupe

import (
	"cmp"
	"regexp"
	"slices"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const (
	nearbyMiles    = 0.03
	priceTolerance = 0.05
)

type Group struct {
	Primary listing.Listing
	Others  []listing.Listing
}

func (g Group) Members() []listing.Listing {
	return append([]listing.Listing{g.Primary}, g.Others...)
}

func Key(l listing.Listing) string {
	return string(l.Source) + "/" + l.SourceID
}

var sourceRank = []listing.Source{
	listing.SourceZillow,
	listing.SourceRedfin,
	listing.SourceStreetEasy,
	listing.SourceRealtor,
	listing.SourceFacebook,
	listing.SourceCraigslist,
}

func Groups(ls []listing.Listing) []Group {
	addrs := make([]address, len(ls))
	for i, l := range ls {
		addrs[i] = parse(l.Address)
	}
	owner := make([]int, len(ls))
	members := make([][]int, len(ls))
	for i := range ls {
		owner[i] = i
		members[i] = []int{i}
	}
	compatible := func(x, y int) bool {
		for _, i := range members[x] {
			for _, j := range members[y] {
				if conflict(ls[i], ls[j], addrs[i], addrs[j]) {
					return false
				}
			}
		}
		return true
	}
	for i := range ls {
		for j := i + 1; j < len(ls); j++ {
			x, y := owner[i], owner[j]
			if x == y || !same(ls[i], ls[j], addrs[i], addrs[j]) || !compatible(x, y) {
				continue
			}
			for _, k := range members[y] {
				owner[k] = x
			}
			members[x] = append(members[x], members[y]...)
			members[y] = nil
		}
	}

	var out []Group
	for i := range ls {
		if owner[i] != i {
			continue
		}
		group := make([]listing.Listing, len(members[i]))
		for k, m := range members[i] {
			group[k] = ls[m]
		}
		slices.SortStableFunc(group, preferred)
		out = append(out, Group{Primary: group[0], Others: group[1:]})
	}
	return out
}

func Primaries(groups []Group) []listing.Listing {
	out := make([]listing.Listing, len(groups))
	for i, g := range groups {
		out[i] = g.Primary
	}
	return out
}

func preferred(a, b listing.Listing) int {
	return cmp.Or(
		cmp.Compare(unassessed(a), unassessed(b)),
		cmp.Compare(rank(a.Source), rank(b.Source)),
		cmp.Compare(len(b.Photos), len(a.Photos)),
		strings.Compare(a.SourceID, b.SourceID),
	)
}

func unassessed(l listing.Listing) int {
	if len(l.Assessments) > 0 {
		return 0
	}
	return 1
}

func rank(s listing.Source) int {
	if i := slices.Index(sourceRank, s); i >= 0 {
		return i
	}
	return len(sourceRank)
}

func same(a, b listing.Listing, aa, ba address) bool {
	if a.Source == b.Source || a.Offer != b.Offer {
		return false
	}
	if a.Beds != nil && b.Beds != nil && *a.Beds != *b.Beds {
		return false
	}
	if !aa.sameLocality(ba) {
		return false
	}
	if aa.numbered() && ba.numbered() {
		if aa.street != ba.street {
			return false
		}
		if aa.unit != "" && ba.unit != "" {
			return aa.unit == ba.unit
		}
		if aa.unit == ba.unit {
			return true
		}
		return a.Beds != nil && b.Beds != nil && closePrice(a.Price, b.Price)
	}
	return a.Beds != nil && b.Beds != nil && closePrice(a.Price, b.Price) && near(a.Coordinates, b.Coordinates)
}

func conflict(a, b listing.Listing, aa, ba address) bool {
	if a.Source == b.Source || a.Offer != b.Offer {
		return true
	}
	if a.Beds != nil && b.Beds != nil && *a.Beds != *b.Beds {
		return true
	}
	if aa.numbered() && ba.numbered() && aa.street != ba.street {
		return true
	}
	return aa.unit != "" && ba.unit != "" && aa.unit != ba.unit
}

func near(a, b *listing.Coordinates) bool {
	return a != nil && b != nil && geo.DistanceMiles(*a, *b) <= nearbyMiles
}

func closePrice(a, b listing.Money) bool {
	if a.Currency != b.Currency || a.Cents <= 0 || b.Cents <= 0 {
		return false
	}
	hi, lo := float64(max(a.Cents, b.Cents)), float64(min(a.Cents, b.Cents))
	return (hi-lo)/hi <= priceTolerance
}

type address struct {
	number string
	street string
	unit   string
	city   string
	postal string
}

func (a address) numbered() bool {
	return a.number != ""
}

func (a address) sameLocality(b address) bool {
	if a.postal != "" && b.postal != "" {
		return a.postal == b.postal
	}
	if a.city != "" && b.city != "" {
		return a.city == b.city
	}
	return true
}

var (
	unitSuffix  = regexp.MustCompile(`\s+(?:#|(?:unit|apt|apartment|suite|ste|no)\b\.?)\s*([a-z0-9-]+)$`)
	houseNumber = regexp.MustCompile(`^(\d+[a-z]?)(?:-\d+[a-z]?)?\s+(.+)$`)
	nonWord     = regexp.MustCompile(`[^a-z0-9#\s-]+`)
	unitPrefix  = regexp.MustCompile(`^(?:#|unit|apt|apartment|suite|ste|no)\.?\s*`)
)

var abbreviations = map[string]string{
	"street": "st", "avenue": "ave", "av": "ave", "road": "rd", "drive": "dr",
	"boulevard": "blvd", "lane": "ln", "place": "pl", "court": "ct", "terrace": "ter",
	"parkway": "pkwy", "square": "sq", "highway": "hwy", "circle": "cir", "way": "wy",
	"north": "n", "south": "s", "east": "e", "west": "w",
	"northeast": "ne", "northwest": "nw", "southeast": "se", "southwest": "sw",
}

func parse(a listing.Address) address {
	street := clean(a.Street)
	unit := clean(a.Unit)
	if m := unitSuffix.FindStringSubmatchIndex(street); m != nil {
		if unit == "" {
			unit = street[m[2]:m[3]]
		}
		street = strings.TrimSpace(street[:m[0]])
	}
	out := address{
		unit:   strings.TrimLeft(unitPrefix.ReplaceAllString(unit, ""), "0"),
		city:   clean(a.City),
		postal: postal(a.PostalCode),
	}
	if m := houseNumber.FindStringSubmatch(street); m != nil {
		out.number = m[1]
		words := strings.Fields(m[2])
		for i, w := range words {
			if abbr, ok := abbreviations[w]; ok {
				words[i] = abbr
			}
		}
		out.street = m[1] + " " + strings.Join(words, " ")
	}
	return out
}

func clean(s string) string {
	return strings.Join(strings.Fields(nonWord.ReplaceAllString(strings.ToLower(s), " ")), " ")
}

func postal(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 5 && s[5] == '-' {
		s = s[:5]
	}
	return s
}
