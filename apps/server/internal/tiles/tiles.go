package tiles

import (
	"context"
	"encoding/json"
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
	"sync"
	"time"

	"github.com/google/renameio/v2"
	"golang.org/x/sync/singleflight"
)

const (
	DefaultTTL    = 30 * 24 * time.Hour
	MaxZoom       = 20
	VectorMaxZoom = 14
	positronPath  = "light_all"
	maxTileBytes  = 4 << 20
)

var (
	ErrNoKey    = errors.New("tile api key is not set")
	ErrBadTile  = errors.New("invalid tile coordinates")
	ErrUpstream = errors.New("tile upstream failed")
)

type Tile struct {
	Z, X, Y int
	Retina  bool
}

func Parse(z, x, file, ext string) (Tile, error) {
	name, ok := strings.CutSuffix(file, ext)
	if !ok {
		return Tile{}, ErrBadTile
	}
	var t Tile
	if ext == ".png" {
		name, t.Retina = strings.CutSuffix(name, "@2x")
	}
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
	Dir       string
	Ext       string
	Key       string
	KeyParam  string
	KeyHeader string
	NeedsKey  bool
	MaxZoom   int
	TTL       time.Duration
	Client    *http.Client
	Now       func() time.Time
	Upstream  func(ctx context.Context, t Tile) (string, error)
	Check     func(res *http.Response) error
	Empty     func(status int) bool

	group singleflight.Group
}

func newCache(dir, ext string) *Cache {
	return &Cache{
		Dir:     dir,
		Ext:     ext,
		MaxZoom: MaxZoom,
		TTL:     DefaultTTL,
		Client:  &http.Client{Timeout: 15 * time.Second},
		Now:     time.Now,
	}
}

func NewPositron(dir, key string) *Cache {
	c := newCache(dir, ".png")
	c.Key, c.KeyParam, c.NeedsKey = key, "key", true
	c.Upstream = func(_ context.Context, t Tile) (string, error) {
		return fmt.Sprintf("https://%c.basemaps.cartocdn.com/%s/%d/%d/%d%s.png", "abcd"[(t.X+t.Y)%4], positronPath, t.Z, t.X, t.Y, t.suffix()), nil
	}
	c.Check = func(res *http.Response) error {
		if strings.HasPrefix(strings.Trim(res.Header.Get("ETag"), `W/"`), "wm-") {
			return errors.New("placeholder tile returned, check the api key")
		}
		return contentType(res, "image/")
	}
	return c
}

func NewTransitland(dir, key string) *Cache {
	c := newCache(dir, ".pbf")
	c.Key, c.KeyHeader, c.NeedsKey = key, "apikey", true
	c.MaxZoom = VectorMaxZoom
	c.TTL = 7 * 24 * time.Hour
	c.Upstream = func(_ context.Context, t Tile) (string, error) {
		return fmt.Sprintf("https://transit.land/api/v2/tiles/routes/tiles/%d/%d/%d.pbf", t.Z, t.X, t.Y), nil
	}
	c.Check = vectorCheck
	c.Empty = vectorEmpty
	return c
}

const openFreeMapTileJSON = "https://tiles.openfreemap.org/planet"

func NewOpenFreeMap(dir string) *Cache {
	return newOpenFreeMap(dir, openFreeMapTileJSON)
}

func newOpenFreeMap(dir, tileJSONURL string) *Cache {
	c := newCache(dir, ".pbf")
	c.MaxZoom = VectorMaxZoom
	tj := &tileJSON{url: tileJSONURL, ttl: 24 * time.Hour}
	c.Upstream = func(ctx context.Context, t Tile) (string, error) {
		tmpl, err := tj.template(ctx, c.Client, c.Now())
		if err != nil {
			return "", err
		}
		return strings.NewReplacer("{z}", strconv.Itoa(t.Z), "{x}", strconv.Itoa(t.X), "{y}", strconv.Itoa(t.Y)).Replace(tmpl), nil
	}
	c.Check = vectorCheck
	c.Empty = vectorEmpty
	return c
}

func vectorCheck(res *http.Response) error {
	ct := res.Header.Get("Content-Type")
	for _, ok := range []string{"application/vnd.mapbox-vector-tile", "application/x-protobuf", "application/octet-stream"} {
		if strings.HasPrefix(ct, ok) {
			return nil
		}
	}
	return fmt.Errorf("content type %q", ct)
}

func vectorEmpty(status int) bool {
	return status == http.StatusNoContent
}

func contentType(res *http.Response, prefix string) error {
	if !strings.HasPrefix(res.Header.Get("Content-Type"), prefix) {
		return fmt.Errorf("content type %q", res.Header.Get("Content-Type"))
	}
	return nil
}

type tileJSON struct {
	url string
	ttl time.Duration

	mu      sync.Mutex
	tmpl    string
	fetched time.Time
}

func (j *tileJSON) template(ctx context.Context, client *http.Client, now time.Time) (string, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.tmpl != "" && now.Sub(j.fetched) < j.ttl {
		return j.tmpl, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "housing")
	res, err := client.Do(req)
	if err != nil {
		return j.stale(fmt.Errorf("%w: tilejson: %v", ErrUpstream, err))
	}
	defer res.Body.Close()
	var doc struct {
		Tiles []string `json:"tiles"`
	}
	if res.StatusCode != http.StatusOK {
		return j.stale(fmt.Errorf("%w: tilejson status %d", ErrUpstream, res.StatusCode))
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&doc); err != nil || len(doc.Tiles) == 0 {
		return j.stale(fmt.Errorf("%w: tilejson has no tiles", ErrUpstream))
	}
	j.tmpl, j.fetched = doc.Tiles[0], now
	return j.tmpl, nil
}

func (j *tileJSON) stale(err error) (string, error) {
	if j.tmpl != "" {
		return j.tmpl, nil
	}
	return "", err
}

func (c *Cache) Enabled() bool {
	return c != nil && (!c.NeedsKey || c.Key != "")
}

func (c *Cache) path(t Tile) string {
	return filepath.Join(c.Dir, strconv.Itoa(t.Z), strconv.Itoa(t.X), strconv.Itoa(t.Y)+t.suffix()+c.Ext)
}

func (c *Cache) Get(ctx context.Context, t Tile) ([]byte, error) {
	if !c.Enabled() {
		return nil, ErrNoKey
	}
	if t.Z > c.MaxZoom {
		return nil, ErrBadTile
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
		if info != nil {
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
	ctx = context.WithoutCancel(ctx)
	raw, err := c.Upstream(ctx, t)
	if err != nil {
		return nil, err
	}
	u, err := url.Parse(raw)
	if err != nil {
		return nil, err
	}
	if c.KeyParam != "" {
		q := u.Query()
		q.Set(c.KeyParam, c.Key)
		u.RawQuery = q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "housing")
	if c.KeyHeader != "" {
		req.Header.Set(c.KeyHeader, c.Key)
	}
	res, err := c.Client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUpstream, c.redact(err))
	}
	defer res.Body.Close()
	if c.Empty != nil && c.Empty(res.StatusCode) {
		return []byte{}, nil
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrUpstream, res.StatusCode)
	}
	if c.Check != nil {
		if err := c.Check(res); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrUpstream, err)
		}
	}
	b, err := io.ReadAll(io.LimitReader(res.Body, maxTileBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w: %s", ErrUpstream, c.redact(err))
	}
	if len(b) > maxTileBytes {
		return nil, fmt.Errorf("%w: tile larger than %d bytes", ErrUpstream, maxTileBytes)
	}
	return b, nil
}

func (c *Cache) redact(err error) string {
	if c.Key == "" {
		return err.Error()
	}
	return strings.NewReplacer(url.QueryEscape(c.Key), "REDACTED", c.Key, "REDACTED").Replace(err.Error())
}
