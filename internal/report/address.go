package report

import (
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

func AddressLine(l listing.Listing) string {
	a := l.Address.Formatted
	if l.Address.Unit != "" && !strings.Contains(a, l.Address.Unit) {
		a += " #" + l.Address.Unit
	}
	return a
}
