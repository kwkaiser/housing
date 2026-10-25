package tiles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
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

	"git.kwkaiser.io/kwkaiser/housing/internal/telemetry"
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
	Valid     func(body []byte) error
	Empty     func(status int) bool

	group singleflight.Group
}

func newCache(dir, ext string) *Cache {
	return &Cache{
		Dir:     dir,
		Ext:     ext,
		MaxZoom: MaxZoom,
		TTL:     DefaultTTL,
		Client:  telemetry.Client(15 * time.Second),
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

const TerrainMaxZoom = 12

func NewTerrain(dir string) *Cache {
	c := newCache(dir, ".webp")
	c.MaxZoom = TerrainMaxZoom
	c.Upstream = func(_ context.Context, t Tile) (string, error) {
		return fmt.Sprintf("https://tiles.mapterhorn.com/%d/%d/%d.webp", t.Z, t.X, t.Y), nil
	}
	c.Check = func(res *http.Response) error {
		return contentType(res, "image/")
	}
	return c
}

func NewCyclOSM(dir string) *Cache {
	c := newCache(dir, ".png")
	c.Upstream = func(_ context.Context, t Tile) (string, error) {
		return fmt.Sprintf("https://%c.tile-cyclosm.openstreetmap.fr/cyclosm-lite/%d/%d/%d.png", "abc"[(t.X+t.Y)%3], t.Z, t.X, t.Y), nil
	}
	c.Check = func(res *http.Response) error {
		return contentType(res, "image/")
	}
	return c
}

const NoiseMaxZoom = 12

func NewNoise(dir string) *Cache {
	c := newCache(dir, ".png")
	c.MaxZoom = NoiseMaxZoom
	c.TTL = 180 * 24 * time.Hour
	c.Upstream = func(_ context.Context, t Tile) (string, error) {
		return fmt.Sprintf("https://geo.dot.gov/server/rest/services/Hosted/NTAD_Noise_2020_CONUS_Aviation_Road_Rail/MapServer/tile/%d/%d/%d", t.Z, t.Y, t.X), nil
	}
	c.Check = func(res *http.Response) error {
		return contentType(res, "image/")
	}
	c.Empty = func(status int) bool {
		return status == http.StatusNotFound
	}
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

const (
	TIGERwebMaxZoom = 12
	tigerwebURL     = "https://tigerweb.geo.census.gov/arcgis/rest/services/TIGERweb/tigerWMS_Current/MapServer"
	tigerwebPlaces  = 28
	tigerwebCousubs = 22
)

func NewPlaces(dir string) *Cache {
	return newTIGERweb(dir, tigerwebURL, tigerwebPlaces, "1=1", "STATE,PLACE,NAME,AREALAND")
}

func NewTownships(dir string) *Cache {
	return newTIGERweb(dir, tigerwebURL, tigerwebCousubs, "FUNCSTAT NOT IN ('S','N') AND COUSUB <> '00000'", "STATE,COUSUB,NAME,AREALAND")
}

func newTIGERweb(dir, base string, layer int, where, fields string) *Cache {
	c := newCache(dir, ".geojson")
	c.MaxZoom = TIGERwebMaxZoom
	c.TTL = 90 * 24 * time.Hour
	c.Upstream = func(_ context.Context, t Tile) (string, error) {
		w, s, e, n := t.Bounds()
		q := url.Values{
			"where":              {where},
			"geometry":           {fmt.Sprintf("%.6f,%.6f,%.6f,%.6f", w, s, e, n)},
			"geometryType":       {"esriGeometryEnvelope"},
			"inSR":               {"4326"},
			"outSR":              {"4326"},
			"spatialRel":         {"esriSpatialRelIntersects"},
			"outFields":          {fields},
			"maxAllowableOffset": {strconv.FormatFloat(360/math.Exp2(float64(t.Z))/1024, 'g', 4, 64)},
			"geometryPrecision":  {"5"},
			"f":                  {"geojson"},
		}
		return fmt.Sprintf("%s/%d/query?%s", base, layer, q.Encode()), nil
	}
	c.Check = func(res *http.Response) error {
		return contentType(res, "application/geo+json")
	}
	c.Valid = func(body []byte) error {
		var doc struct {
			Type  string `json:"type"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal(body, &doc); err != nil {
			return err
		}
		if doc.Error != nil {
			return fmt.Errorf("arcgis: %s", doc.Error.Message)
		}
		if doc.Type != "FeatureCollection" {
			return fmt.Errorf("geojson type %q", doc.Type)
		}
		return nil
	}
	return c
}

func (t Tile) Bounds() (west, south, east, north float64) {
	n := math.Exp2(float64(t.Z))
	lng := func(x int) float64 { return float64(x)/n*360 - 180 }
	lat := func(y int) float64 { return math.Atan(math.Sinh(math.Pi*(1-2*float64(y)/n))) * 180 / math.Pi }
	return lng(t.X), lat(t.Y + 1), lng(t.X + 1), lat(t.Y)
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
	if c.Valid != nil {
		if err := c.Valid(b); err != nil {
			return nil, fmt.Errorf("%w: %s", ErrUpstream, err)
		}
	}
	return b, nil
}

func (c *Cache) redact(err error) string {
	if c.Key == "" {
		return err.Error()
	}
	return strings.NewReplacer(url.QueryEscape(c.Key), "REDACTED", c.Key, "REDACTED").Replace(err.Error())
}
