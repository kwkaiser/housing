package listing

import (
	"regexp"
	"strings"
)

var amenityPhrases = map[Amenity][]string{
	AmenityDishwasher: {"dishwasher", "dish washer"},
	AmenityInUnitLaundry: {
		"in-unit laundry", "in unit laundry", "laundry in unit", "laundry in-unit",
		"in-unit washer", "in unit washer", "washer/dryer in unit", "washer and dryer in unit",
		"washer & dryer in unit", "w/d in unit", "in-unit w/d", "in unit w/d",
	},
	AmenityAirConditioning: {"central air", "central a/c", "central ac", "air conditioning", "air-conditioning", "mini-split", "mini split", "ductless"},
	AmenityParking: {
		"off-street parking", "off street parking", "parking space", "parking spot", "deeded parking",
		"garage parking", "assigned parking", "driveway parking", "private driveway", "parking included",
	},
	AmenityFurnished: {"furnished", "furniture included"},
	AmenityShortTerm: {
		"short term", "short-term", "month-to-month", "month to month", "sublet", "sublease",
		"temporary housing", "corporate housing", "flexible lease", "flexible term",
	},
}

var negation = regexp.MustCompile(`\b(no|not|without|lacks?|none)\b[^.,;!\n]{0,20}$`)

func AmenitiesFromText(text string) map[Amenity]bool {
	lower := strings.ToLower(text)
	found := map[Amenity]bool{}
	for amenity, phrases := range amenityPhrases {
		for _, phrase := range phrases {
			if mentionedAffirmatively(lower, phrase) {
				found[amenity] = true
				break
			}
		}
	}
	return found
}

func mentionedAffirmatively(text, phrase string) bool {
	for offset := 0; ; {
		i := strings.Index(text[offset:], phrase)
		if i < 0 {
			return false
		}
		start := offset + i
		if !midWord(text, start) && !negation.MatchString(text[max(0, start-30):start]) {
			return true
		}
		offset = start + len(phrase)
	}
}

func midWord(text string, start int) bool {
	if start == 0 {
		return false
	}
	c := text[start-1]
	return c >= 'a' && c <= 'z'
}
