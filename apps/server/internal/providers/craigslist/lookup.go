package craigslist

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"path"
	"regexp"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

var (
	_         listing.Lookup = (*Provider)(nil)
	postPaths                = []*regexp.Regexp{
		regexp.MustCompile(`^(?:/[a-z]+){0,2}/d/[^/]+/\d+\.html$`),
		regexp.MustCompile(`^/view/d/[^/]+/[A-Za-z0-9]+$`),
	}
)

func IsHost(host string) bool {
	host = strings.ToLower(host)
	return host == "craigslist.org" || strings.HasSuffix(host, ".craigslist.org")
}

func ListingURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !IsHost(u.Hostname()) {
		return "", fmt.Errorf("not a craigslist url: %q", raw)
	}
	path := strings.TrimSuffix(u.EscapedPath(), "/")
	for _, p := range postPaths {
		if p.MatchString(path) {
			return "https://" + strings.ToLower(u.Hostname()) + path, nil
		}
	}
	return "", fmt.Errorf("not a craigslist posting: %q; use a link to a single post (…/d/<title>/<id>.html)", raw)
}

func (p *Provider) Lookup(ctx context.Context, rawURL string) (listing.Listing, error) {
	u, err := ListingURL(rawURL)
	if err != nil {
		return listing.Listing{}, err
	}
	items, err := p.Runner.Run(ctx, ActorID, actorInput{
		StartURLs:      []actorURL{{URL: u}},
		IncludeDetails: true,
		MaxItems:       1,
	})
	if err != nil {
		return listing.Listing{}, err
	}
	now := p.Now().UTC()
	for _, raw := range items {
		raw, err := withID(raw, postID(u))
		if err != nil {
			return listing.Listing{}, err
		}
		l, ok, err := toListing(raw, now)
		if err != nil {
			return listing.Listing{}, err
		}
		if ok {
			return l, nil
		}
	}
	return listing.Listing{}, fmt.Errorf("craigslist scraper returned no priced rental for %s", u)
}

func postID(listingURL string) string {
	return strings.TrimSuffix(path.Base(listingURL), ".html")
}

func withID(raw json.RawMessage, id string) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return nil, fmt.Errorf("decode craigslist item: %w", err)
	}
	var current string
	json.Unmarshal(fields["id"], &current)
	if current != "" {
		return raw, nil
	}
	b, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}
	fields["id"] = b
	return json.Marshal(fields)
}
