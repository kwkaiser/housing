package tiles

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/renameio/v2"
	"golang.org/x/sync/singleflight"
)

const (
	DefaultTTL   = 30 * 24 * time.Hour
	MaxZoom      = 20
	positronPath = "light_all"
	maxTileBytes = 1 << 20
)

var (
	ErrNoKey    = errors.New("carto api key is not set")
	ErrBadTile  = errors.New("invalid tile coordinates")
	ErrUpstream = errors.New("tile upstream failed")
)

type Tile struct {
	Z, X, Y int
	Retina  bool
}

func Parse(z, x, file string) (Tile, error) {
	name, ok := strings.CutSuffix(file, ".png")
	if !ok {
		return Tile{}, ErrBadTile
	}
	var t Tile
	name, t.Retina = strings.CutSuffix(name, "@2x")
	var err error
	if t.Z, err = strconv.Atoi(z); err != nil {
		return Tile{}, ErrBadTile
	}
	if t.X, err = strconv.Atoi(x); err != nil {
		return Tile{}, ErrBadTile
	}
	if t.Y, err = strconv.Atoi(name); err != nil {
		return Tile{}, ErrBadTile
	}
	if t.Z < 0 || t.Z > MaxZoom || t.X < 0 || t.Y < 0 || t.X >= 1<<t.Z || t.Y >= 1<<t.Z {
		return Tile{}, ErrBadTile
	}
	return t, nil
}

func (t Tile) suffix() string {
	if t.Retina {
		return "@2x"
	}
	return ""
}

type Cache struct {
	Dir      string
	Key      string
	TTL      time.Duration
	Client   *http.Client
	Now      func() time.Time
	Upstream func(t Tile) string

	group singleflight.Group
}

func New(dir, key string) *Cache {
	return &Cache{
		Dir:    dir,
		Key:    key,
		TTL:    DefaultTTL,
		Client: &http.Client{Timeout: 15 * time.Second},
		Now:    time.Now,
		Upstream: func(t Tile) string {
			return fmt.Sprintf("https://%c.basemaps.cartocdn.com/%s/%d/%d/%d%s.png", "abcd"[(t.X+t.Y)%4], positronPath, t.Z, t.X, t.Y, t.suffix())
		},
	}
}

func (c *Cache) Enabled() bool {
	return c != nil && c.Key != ""
}

func (c *Cache) path(t Tile) string {
	return filepath.Join(c.Dir, strconv.Itoa(t.Z), strconv.Itoa(t.X), strconv.Itoa(t.Y)+t.suffix()+".png")
}

func (c *Cache) Get(ctx context.Context, t Tile) ([]byte, error) {
	if !c.Enabled() {
		return nil, ErrNoKey
	}
	p := c.path(t)
	cached, info, err := read(p)
	if err == nil && c.Now().Sub(info.ModTime()) < c.TTL {
		return cached, nil
	}
	v, err, _ := c.group.Do(p, func() (any, error) {
		b, err := c.fetch(ctx, t)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, err
		}
		return b, renameio.WriteFile(p, b, 0o644)
	})
	if err != nil {
		if cached != nil {
			return cached, nil
		}
		return nil, err
	}
	return v.([]byte), nil
}

func read(p string) ([]byte, fs.FileInfo, error) {
	info, err := os.Stat(p)
	if err != nil {
		return nil, nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, nil, err
	}
	return b, info, nil
}

func (c *Cache) fetch(ctx context.Context, t Tile) ([]byte, error) {
	u, err := url.Parse(c.Upstream(t))
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("key", c.Key)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(context.WithoutCancel(ctx), http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "housing")
	res, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUpstream, redact(err, c.Key))
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("%w: status %d", ErrUpstream, res.StatusCode)
	case strings.HasPrefix(strings.Trim(res.Header.Get("ETag"), `W/"`), "wm-"):
		return nil, fmt.Errorf("%w: placeholder tile returned, check the api key", ErrUpstream)
	case !strings.HasPrefix(res.Header.Get("Content-Type"), "image/"):
		return nil, fmt.Errorf("%w: content type %q", ErrUpstream, res.Header.Get("Content-Type"))
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxTileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUpstream, redact(err, c.Key))
	}
	if len(b) > maxTileBytes {
		return nil, fmt.Errorf("%w: tile larger than %d bytes", ErrUpstream, maxTileBytes)
	}
	return b, nil
}

func redact(err error, key string) string {
	return strings.NewReplacer(url.QueryEscape(key), "REDACTED", key, "REDACTED").Replace(err.Error())
}
