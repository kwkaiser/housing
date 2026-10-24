package streeteasy

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

var (
	_            listing.Lookup = (*Provider)(nil)
	listingPaths                = []*regexp.Regexp{
		regexp.MustCompile(`^/building/[^/]+/(?:rental/\d+|[^/]+)$`),
		regexp.MustCompile(`^/(?:rental|sale)/\d+$`),
	}
)

func IsHost(host string) bool {
	host = strings.ToLower(host)
	return host == "streeteasy.com" || strings.HasSuffix(host, ".streeteasy.com")
}

func ListingURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !IsHost(u.Hostname()) {
		return "", fmt.Errorf("not a streeteasy url: %q", raw)
	}
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	for _, p := range listingPaths {
		if p.MatchString(path) {
			return origin + path, nil
		}
	}
	return "", fmt.Errorf("not a streeteasy listing page: %q; use a unit link like /building/<building>/<unit> or /rental/<id>", raw)
}

func (p *Provider) Lookup(ctx context.Context, rawURL string) (listing.Listing, error) {
	u, err := ListingURL(rawURL)
	if err != nil {
		return listing.Listing{}, err
	}
	items, err := p.Runner.Run(ctx, ActorID, actorInput{StartURLs: []actorURL{{URL: u}}, MaxItems: 1})
	if err != nil {
		return listing.Listing{}, err
	}
	now := p.Now().UTC()
	for _, raw := range items {
		l, ok, err := toListing(raw, "", now)
		if err != nil {
			return listing.Listing{}, err
		}
		if !ok {
			continue
		}
		if l.Coordinates == nil {
			if region, err := p.Regions.Resolve(ctx, l.Address.Formatted); err == nil {
				l.Coordinates = &region.Center
			}
		}
		return l, nil
	}
	return listing.Listing{}, fmt.Errorf("streeteasy scraper returned no priced listing for %s", u)
}
