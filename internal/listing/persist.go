package listing

import "context"

type ListingPersister interface {
	Persist(ctx context.Context, dir string, listings []Listing) error
	Load(ctx context.Context, dir string) ([]Listing, error)
}
