package media

import (
	"context"
	"image"
	"io"
)

type Fetcher interface {
	Fetch(ctx context.Context, url string) (io.ReadCloser, error)
}

type ImageStore interface {
	Has(ctx context.Context, key string) (bool, error)
	Put(ctx context.Context, key string, r io.Reader) error
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Path(key string) string
}

type Collager interface {
	ID() string
	Collage(photos []image.Image) ([]image.Image, error)
}
