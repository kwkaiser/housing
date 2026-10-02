package media

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png"
	"log/slog"
	"net/url"
	"path"
	"strconv"
	"strings"

	_ "golang.org/x/image/webp"
	"golang.org/x/sync/errgroup"

	"git.kwkaiser.io/kwkaiser/housing/internal/digest"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

const (
	DefaultMaxPhotos   = 18
	DefaultConcurrency = 8
	jpegQuality        = 85
)

type Processor struct {
	Fetcher     Fetcher
	Store       ImageStore
	Collager    Collager
	MaxPhotos   int
	Concurrency int
	Force       bool
	Logger      *slog.Logger
}

func NewProcessor(fetcher Fetcher, store ImageStore, collager Collager) *Processor {
	return &Processor{
		Fetcher:     fetcher,
		Store:       store,
		Collager:    collager,
		MaxPhotos:   DefaultMaxPhotos,
		Concurrency: DefaultConcurrency,
		Logger:      slog.New(slog.DiscardHandler),
	}
}

func (p *Processor) Process(ctx context.Context, listings []listing.Listing) ([]listing.Listing, error) {
	if err := p.FetchPhotos(ctx, listings); err != nil {
		return nil, err
	}
	return p.Collage(ctx, listings)
}

func (p *Processor) FetchPhotos(ctx context.Context, listings []listing.Listing) error {
	var urls []string
	seen := map[string]bool{}
	for _, l := range listings {
		for _, u := range p.selectPhotos(l) {
			if !seen[u] {
				seen[u] = true
				urls = append(urls, u)
			}
		}
	}

	var g errgroup.Group
	g.SetLimit(max(p.Concurrency, 1))
	for _, u := range urls {
		g.Go(func() error {
			if ctx.Err() != nil {
				return nil
			}
			if err := p.ensurePhoto(ctx, u); err != nil {
				p.Logger.Warn("skipping photo", "url", u, "err", err)
			}
			return nil
		})
	}
	g.Wait()
	return ctx.Err()
}

func (p *Processor) Collage(ctx context.Context, listings []listing.Listing) ([]listing.Listing, error) {
	collages := map[string][]string{}
	out := make([]listing.Listing, len(listings))
	for i, l := range listings {
		photos, err := p.storedPhotos(ctx, l)
		if err != nil {
			return nil, err
		}
		if len(photos) == 0 {
			l.Collages = nil
			out[i] = l
			continue
		}

		setKey := p.setKey(photos)
		keys, ok := collages[setKey]
		if !ok {
			keys, err = p.collage(ctx, setKey, photos)
			if err != nil {
				return nil, fmt.Errorf("collage %s/%s: %w", l.Source, l.SourceID, err)
			}
			collages[setKey] = keys
		}
		l.Collages = keys
		out[i] = l
	}
	return out, nil
}

func (p *Processor) storedPhotos(ctx context.Context, l listing.Listing) ([]string, error) {
	var out []string
	for _, u := range p.selectPhotos(l) {
		ok, err := p.Store.Has(ctx, PhotoKey(u))
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, u)
		}
	}
	return out, nil
}

func (p *Processor) selectPhotos(l listing.Listing) []string {
	if p.MaxPhotos > 0 && len(l.Photos) > p.MaxPhotos {
		return l.Photos[:p.MaxPhotos]
	}
	return l.Photos
}

func (p *Processor) ensurePhoto(ctx context.Context, u string) error {
	key := PhotoKey(u)
	ok, err := p.Store.Has(ctx, key)
	if err != nil || ok {
		return err
	}
	body, err := p.Fetcher.Fetch(ctx, u)
	if err != nil {
		return err
	}
	defer body.Close()
	return p.Store.Put(ctx, key, body)
}

func (p *Processor) collage(ctx context.Context, setKey string, urls []string) ([]string, error) {
	indexKey := setKey + "/index.json"
	if !p.Force {
		if keys, ok, err := p.readIndex(ctx, indexKey); err != nil || ok {
			return keys, err
		}
	}

	var imgs []image.Image
	for _, u := range urls {
		img, err := p.decode(ctx, PhotoKey(u))
		if err != nil {
			p.Logger.Warn("skipping undecodable photo", "url", u, "err", err)
			continue
		}
		imgs = append(imgs, img)
	}
	if len(imgs) == 0 {
		return nil, nil
	}

	pages, err := p.Collager.Collage(imgs)
	if err != nil {
		return nil, err
	}

	keys := make([]string, len(pages))
	for i, page := range pages {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, page, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return nil, err
		}
		keys[i] = setKey + "/" + strconv.Itoa(i) + ".jpg"
		if err := p.Store.Put(ctx, keys[i], &buf); err != nil {
			return nil, err
		}
	}

	index, err := json.Marshal(keys)
	if err != nil {
		return nil, err
	}
	if err := p.Store.Put(ctx, indexKey, bytes.NewReader(index)); err != nil {
		return nil, err
	}
	return keys, nil
}

func (p *Processor) readIndex(ctx context.Context, key string) ([]string, bool, error) {
	ok, err := p.Store.Has(ctx, key)
	if err != nil || !ok {
		return nil, false, err
	}
	r, err := p.Store.Open(ctx, key)
	if err != nil {
		return nil, false, err
	}
	defer r.Close()
	var keys []string
	if err := json.NewDecoder(r).Decode(&keys); err != nil {
		return nil, false, nil
	}
	return keys, true, nil
}

func (p *Processor) decode(ctx context.Context, key string) (image.Image, error) {
	r, err := p.Store.Open(ctx, key)
	if err != nil {
		return nil, err
	}
	defer r.Close()
	img, _, err := image.Decode(r)
	return img, err
}

func (p *Processor) setKey(urls []string) string {
	ids := make([]string, len(urls))
	for i, u := range urls {
		ids[i] = PhotoID(u)
	}
	h := digest.String(p.Collager.ID() + "\n" + strings.Join(ids, "\n"))
	return "collages/" + h[:2] + "/" + h
}

func PhotoKey(rawURL string) string {
	h := digest.String(PhotoID(rawURL))
	return "photos/" + h[:2] + "/" + h + photoExt(rawURL)
}

func PhotoID(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if strings.HasSuffix(u.Hostname(), ".fbcdn.net") {
		return "fbcdn:" + u.Path
	}
	return rawURL
}

func photoExt(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	switch ext := strings.ToLower(path.Ext(u.Path)); ext {
	case ".jpg", ".jpeg", ".png", ".webp":
		return ext
	}
	return ""
}
