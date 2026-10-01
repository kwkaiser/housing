package facebook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const itemURL = "https://www.facebook.com/marketplace/item/"

var (
	_        listing.Lookup = (*Provider)(nil)
	itemPath                = regexp.MustCompile(`^/marketplace/item/(\d+)$`)
)

func IsHost(host string) bool {
	host = strings.ToLower(host)
	return host == "facebook.com" || strings.HasSuffix(host, ".facebook.com")
}

func ListingURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !IsHost(u.Hostname()) {
		return "", fmt.Errorf("not a facebook url: %q", raw)
	}
	m := itemPath.FindStringSubmatch(strings.TrimSuffix(u.EscapedPath(), "/"))
	if m == nil {
		return "", fmt.Errorf("not a facebook marketplace item: %q; use a link like %s<id>/", raw, itemURL)
	}
	return itemURL + m[1] + "/", nil
}

func (p *Provider) Lookup(ctx context.Context, rawURL string) (listing.Listing, error) {
	u, err := ListingURL(rawURL)
	if err != nil {
		return listing.Listing{}, err
	}
	id := strings.TrimSuffix(strings.TrimPrefix(u, itemURL), "/")
	items, err := p.Runner.Run(ctx, ActorID, actorInput{
		URLs:                []string{u},
		GetListingDetails:   true,
		GetAllListingPhotos: true,
		Proxy:               proxy{UseApifyProxy: true, ApifyProxyCountry: "US"},
	})
	if err != nil {
		return listing.Listing{}, err
	}

	now := p.Now().UTC()
	for _, raw := range items {
		var it item
		if err := json.Unmarshal(raw, &it); err != nil {
			return listing.Listing{}, fmt.Errorf("decode facebook item: %w", err)
		}
		if it.ID != id {
			continue
		}
		l, ok := mapItem(it, raw, now)
		if !ok {
			return listing.Listing{}, fmt.Errorf("facebook marketplace item %s has no price", id)
		}
		if it.ListingURL == "" {
			l.URL = itemURL + it.ID
		}
		return l, nil
	}
	return listing.Listing{}, fmt.Errorf("facebook marketplace scraper returned nothing for %s", u)
}
