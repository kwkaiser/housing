package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/google/renameio/v2"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/geo"
	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

const (
	apiPrefix      = "/api"
	apiV1          = apiPrefix + "/v1"
	apiAuthScheme  = "bearer"
	sessionScheme  = "session"
	apiSpecVersion = "1.1.0"
)

func OpenAPI() ([]byte, error) {
	a := &App{log: slog.New(slog.DiscardHandler)}
	_, api := a.newAPI()
	return json.MarshalIndent(api.OpenAPI(), "", "  ")
}

func (a *App) apiHandler() http.Handler {
	mux, _ := a.newAPI()
	return mux
}

func (a *App) newAPI() (*http.ServeMux, huma.API) {
	huma.DefaultArrayNullable = false
	mux := http.NewServeMux()
	cfg := huma.DefaultConfig("housing", apiSpecVersion)
	cfg.CreateHooks = nil
	cfg.OpenAPIPath = apiPrefix + "/openapi"
	cfg.DocsPath = apiPrefix + "/docs"
	cfg.SchemasPath = apiPrefix + "/schemas"
	cfg.Info.Description = "Manage collections, profiles, favorites, and places of interest, read ranked listings and map pins, and queue and follow jobs."
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		apiAuthScheme: {Type: "http", Scheme: "bearer", Description: "An access token from the server's OIDC provider."},
		sessionScheme: {Type: "apiKey", In: "cookie", Name: "__Host-" + sessionCookie, Description: "The browser session set by signing in at /auth/login."},
	}
	cfg.Security = []map[string][]string{{apiAuthScheme: {}}, {sessionScheme: {}}}
	api := humago.New(mux, cfg)
	api.UseMiddleware(a.apiAuth(api))

	huma.Get(api, apiV1+"/me", a.apiMe, op("getMe", "Get the signed-in user", "auth"))
	huma.Get(api, apiV1+"/meta", a.apiMeta, op("getMeta", "Get the server version, choices and defaults for building forms", "meta"))
	huma.Get(api, apiV1+"/collections", a.apiCollections, op("listCollections", "List collections", "collections"))
	huma.Register(api, huma.Operation{
		OperationID:   "createCollection",
		Method:        http.MethodPost,
		Path:          apiV1 + "/collections",
		Summary:       "Create a collection",
		Description:   "Does not run it; queue a run for that.",
		Tags:          []string{"collections"},
		DefaultStatus: http.StatusCreated,
	}, a.apiCreateCollection)
	huma.Get(api, apiV1+"/collections/{id}", a.apiCollection, op("getCollection", "Get a collection", "collections"))
	huma.Patch(api, apiV1+"/collections/{id}", a.apiUpdateCollection, op("updateCollection", "Change a collection's settings; only the fields given change", "collections"))
	huma.Get(api, apiV1+"/collections/{id}/days", a.apiCollectionDays, op("listCollectionDays", "List the days a collection was run", "collections"))
	huma.Get(api, apiV1+"/collections/{id}/listings", a.apiCollectionListings, op("listCollectionListings", "List a collection's ranked listings for one day", "listings"))
	huma.Register(api, huma.Operation{
		OperationID:   "runCollection",
		Method:        http.MethodPost,
		Path:          apiV1 + "/collections/{id}/runs",
		Summary:       "Queue a run of a collection",
		Tags:          []string{"jobs"},
		DefaultStatus: http.StatusAccepted,
	}, a.apiRunCollection)
	huma.Get(api, apiV1+"/listings/{source}/{source_id}", a.apiListing, op("getListing", "Get a listing with its grades", "listings"))
	huma.Get(api, apiV1+"/listings/{source}/{source_id}/gallery", a.apiListingGallery, op("getListingGallery", "Get a listing's stored photos", "listings"))
	huma.Get(api, apiV1+"/favorites", a.apiFavorites, op("listFavorites", "List favorite listings, newest first", "favorites"))
	huma.Register(api, huma.Operation{
		OperationID:   "addFavorite",
		Method:        http.MethodPut,
		Path:          apiV1 + "/listings/{source}/{source_id}/favorite",
		Summary:       "Mark a listing as a favorite",
		Tags:          []string{"favorites"},
		DefaultStatus: http.StatusNoContent,
	}, a.apiSetFavorite(true))
	huma.Register(api, huma.Operation{
		OperationID:   "removeFavorite",
		Method:        http.MethodDelete,
		Path:          apiV1 + "/listings/{source}/{source_id}/favorite",
		Summary:       "Unmark a favorite listing",
		Tags:          []string{"favorites"},
		DefaultStatus: http.StatusNoContent,
	}, a.apiSetFavorite(false))
	huma.Get(api, apiV1+"/profiles", a.apiProfiles, op("listProfiles", "List profiles", "profiles"))
	huma.Register(api, huma.Operation{
		OperationID:   "createProfile",
		Method:        http.MethodPost,
		Path:          apiV1 + "/profiles",
		Summary:       "Queue creation of a profile from a reference listing",
		Description:   "Looking up the listing spends money on scraping, and drafting criteria spends money on a model call. Follow the returned job for progress.",
		Tags:          []string{"profiles"},
		DefaultStatus: http.StatusAccepted,
	}, a.apiCreateProfile)
	huma.Get(api, apiV1+"/profiles/{id}", a.apiProfile, op("getProfile", "Get a profile", "profiles"))
	huma.Patch(api, apiV1+"/profiles/{id}", a.apiUpdateProfile, op("updateProfile", "Change a profile's name, summary, notes or own criteria; only the fields given change", "profiles"))
	huma.Register(api, huma.Operation{
		OperationID:   "redraftProfile",
		Method:        http.MethodPost,
		Path:          apiV1 + "/profiles/{id}/drafts",
		Summary:       "Queue a re-draft of a profile's criteria from its reference listings",
		Description:   "Replaces the profile's criteria and spends money on a model call. Follow the returned job for progress.",
		Tags:          []string{"profiles"},
		DefaultStatus: http.StatusAccepted,
	}, a.apiRedraftProfile)
	huma.Get(api, apiV1+"/jobs", a.apiJobs, op("listJobs", "List jobs, newest first", "jobs"))
	huma.Get(api, apiV1+"/jobs/{id}", a.apiJob, op("getJob", "Get a job", "jobs"))
	huma.Get(api, apiV1+"/jobs/{id}/events", a.apiJobEvents, op("listJobEvents", "List a job's progress log", "jobs"))
	a.registerJobStream(api)
	huma.Get(api, apiV1+"/places", a.apiPlaces, op("listPlaces", "List saved places of interest", "places"))
	huma.Register(api, huma.Operation{
		OperationID:   "addPlace",
		Method:        http.MethodPost,
		Path:          apiV1 + "/places",
		Summary:       "Save a place of interest",
		Tags:          []string{"places"},
		DefaultStatus: http.StatusCreated,
	}, a.apiAddPlace)
	huma.Register(api, huma.Operation{
		OperationID:   "removePlace",
		Method:        http.MethodDelete,
		Path:          apiV1 + "/places/{id}",
		Summary:       "Remove a saved place of interest",
		Tags:          []string{"places"},
		DefaultStatus: http.StatusNoContent,
	}, a.apiRemovePlace)
	huma.Get(api, apiV1+"/places/search", a.apiSearchPlaces, op("searchPlaces", "Look up addresses and places to save", "places"))
	huma.Get(api, apiV1+"/map/pins", a.apiMapPins, op("listMapPins", "List map pins for every collection's listings over a range of runs", "map"))
	huma.Get(api, apiV1+"/map/config", a.apiMapConfig, op("getMapConfig", "Get the tile layers and overlays the server can serve", "map"))
	huma.Get(api, apiV1+"/tiles/positron/{z}/{x}/{file}", a.apiPositronTile, op("getPositronTile", "Get a cached CARTO Positron base map tile", "map"))
	huma.Get(api, apiV1+"/tiles/cyclosm/{z}/{x}/{file}", a.apiCyclOSMTile, op("getCyclOSMTile", "Get a cached CyclOSM Lite bike infrastructure tile", "map"))
	huma.Get(api, apiV1+"/tiles/noise/{z}/{x}/{file}", a.apiNoiseTile, op("getNoiseTile", "Get a cached BTS transportation noise tile", "map"))
	huma.Get(api, apiV1+"/tiles/terrain/{z}/{x}/{file}", a.apiTerrainTile, op("getTerrainTile", "Get a cached Mapterhorn terrarium-encoded elevation tile", "map"))
	huma.Get(api, apiV1+"/overlays/{name}/{z}/{x}/{file}", a.apiOverlayTile, op("getOverlayTile", "Get a town boundary, transit or bike lane overlay tile", "map"))
	mux.Handle(mcpPath, a.mcpHandler())
	huma.Get(api, apiV1+"/media/{key...}", a.apiMedia, op("getMedia", "Get a stored image such as a listing collage", "media"))
	paths := api.OpenAPI().Paths
	paths[apiV1+"/media/{key}"] = paths[apiV1+"/media/{key...}"]
	delete(paths, apiV1+"/media/{key...}")
	return mux, api
}

func op(id, summary, tag string) func(*huma.Operation) {
	return func(o *huma.Operation) {
		o.OperationID = id
		o.Summary = summary
		o.Tags = []string{tag}
	}
}

func (a *App) apiAuth(api huma.API) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		r, _ := humago.Unwrap(ctx)
		u, err := a.authenticate(r)
		switch {
		case errors.Is(err, errUnauthenticated):
			ctx.SetHeader("WWW-Authenticate", `Bearer realm="housing"`)
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "sign in, or send an access token")
		case errors.Is(err, errCrossSite):
			huma.WriteErr(api, ctx, http.StatusForbidden, "cross-site request rejected")
		case err != nil:
			a.log.ErrorContext(ctx.Context(), "api auth", "err", err)
			huma.WriteErr(api, ctx, http.StatusInternalServerError, "internal error")
		default:
			next(huma.WithValue(ctx, userKey{}, u))
		}
	}
}

func (a *App) apiMe(ctx context.Context, _ *struct{}) (*struct{ Body APIUser }, error) {
	return &struct{ Body APIUser }{userFrom(ctx)}, nil
}

func (a *App) apiError(ctx context.Context, err error) error {
	if errors.Is(err, collection.ErrNotFound) || errors.Is(err, store.ErrListingNotFound) || errors.Is(err, store.ErrJobNotFound) || errors.Is(err, profile.ErrNotFound) || errors.Is(err, store.ErrPlaceNotFound) {
		return huma.Error404NotFound(err.Error())
	}
	a.log.ErrorContext(ctx, "api", "err", err)
	return huma.Error500InternalServerError("internal error")
}

func (a *App) apiSaveError(ctx context.Context, err error) error {
	var fe profile.FieldError
	if !errors.As(err, &fe) {
		return a.apiError(ctx, err)
	}
	fields := map[string]string{}
	var general []string
	collectErrors(err, fields, &general)
	var details []error
	for _, k := range slices.Sorted(maps.Keys(fields)) {
		details = append(details, &huma.ErrorDetail{Location: "body." + k, Message: fields[k]})
	}
	for _, g := range general {
		details = append(details, &huma.ErrorDetail{Location: "body", Message: g})
	}
	return huma.Error422UnprocessableEntity("validation failed", details...)
}

type APICollection struct {
	ID             string           `json:"id"`
	Mode           profile.Mode     `json:"mode"`
	Sources        []listing.Source `json:"sources"`
	Profiles       []string         `json:"profiles"`
	Search         profile.Search   `json:"search"`
	Model          string           `json:"model,omitempty"`
	Schedule       string           `json:"schedule,omitempty" doc:"Daily run time, HH:MM in server-local time"`
	MaxRunCostUSD  float64          `json:"max_run_cost_usd,omitempty" doc:"Spending cap per run; absent means the default"`
	NotifyURL      string           `json:"notify_url,omitempty"`
	NotifyMinScore float64          `json:"notify_min_score,omitempty" doc:"Minimum match to notify about; absent means the default"`
	LastRun        *APIJob          `json:"last_run,omitempty" doc:"The active run, or else the most recent one"`
}

func apiCollection(c collection.Collection) APICollection {
	return APICollection{
		ID: c.ID, Mode: c.Mode, Sources: nonNil(c.Sources), Profiles: nonNil(c.Profiles), Search: c.Search, Model: c.Model, Schedule: c.Schedule,
		MaxRunCostUSD: c.MaxRunCostUSD, NotifyURL: c.Notify.URL, NotifyMinScore: c.Notify.MinScore,
	}
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

type APIListing struct {
	Source      listing.Source       `json:"source"`
	SourceID    string               `json:"source_id"`
	URL         string               `json:"url" doc:"The listing on its source site"`
	Href        string               `json:"href" doc:"This listing's API path"`
	Offer       listing.OfferType    `json:"offer"`
	Price       listing.Money        `json:"price"`
	Address     listing.Address      `json:"address"`
	Coordinates *listing.Coordinates `json:"coordinates,omitempty"`
	Beds        *int                 `json:"beds,omitempty"`
	Baths       *float64             `json:"baths,omitempty"`
	SqFt        *int                 `json:"sqft,omitempty"`
	Amenities   map[string]bool      `json:"amenities,omitempty"`
	Description string               `json:"description,omitempty"`
	Photos      []string             `json:"photos" doc:"Photo URLs on the source site"`
	Collages    []string             `json:"collages,omitempty" doc:"API paths of the photo collages the listing was graded from"`
	Thumbnail   string               `json:"thumbnail,omitempty" doc:"API path of the listing's first stored photo, which takes a size like other media"`
	ListedAt    *time.Time           `json:"listed_at,omitempty"`
	ObservedAt  time.Time            `json:"observed_at"`
}

func apiListing(l listing.Listing, thumbnail string) APIListing {
	out := APIListing{
		Source: l.Source, SourceID: l.SourceID, URL: l.URL, Href: apiListingPath(l.Source, l.SourceID), Offer: l.Offer, Price: l.Price, Address: l.Address,
		Coordinates: l.Coordinates, Beds: l.Beds, Baths: l.Baths, SqFt: l.SqFt, Description: l.Description,
		Photos: l.Photos, Thumbnail: thumbnail, ListedAt: l.ListedAt, ObservedAt: l.ObservedAt,
	}
	if out.Photos == nil {
		out.Photos = []string{}
	}
	if len(l.Amenities) > 0 {
		out.Amenities = make(map[string]bool, len(l.Amenities))
		for k, v := range l.Amenities {
			out.Amenities[string(k)] = v
		}
	}
	for _, key := range l.Collages {
		out.Collages = append(out.Collages, apiV1+"/media/"+key)
	}
	return out
}

func mediaPaths(keys []string) []string {
	out := make([]string, len(keys))
	for i, k := range keys {
		out[i] = apiV1 + "/media/" + k
	}
	return out
}

func (a *App) listing(ctx context.Context, l listing.Listing) APIListing {
	thumbnail := a.thumbnail(ctx, l)
	l.Collages = a.svc.StoredCollages(ctx, l.Collages)
	return apiListing(l, thumbnail)
}

func (a *App) thumbnail(ctx context.Context, l listing.Listing) string {
	if key := a.svc.Thumbnail(ctx, l); key != "" {
		return apiV1 + "/media/" + key
	}
	return ""
}

func apiListingPath(source listing.Source, id string) string {
	return apiV1 + "/listings/" + string(source) + "/" + id
}

type APIGrade struct {
	Profile           string   `json:"profile"`
	Match             float64  `json:"match" doc:"Score relative to the profile's reference listings when calibrated, otherwise the raw score"`
	Calibrated        bool     `json:"calibrated"`
	Score             float64  `json:"score"`
	Coverage          float64  `json:"coverage"`
	MissingEssentials []string `json:"missing_essentials,omitempty"`
	Dealbreakers      []string `json:"dealbreakers,omitempty"`
	AvoidsHit         []string `json:"avoids_hit,omitempty"`
	Summary           string   `json:"summary"`
	Stale             bool     `json:"stale,omitempty" doc:"The assessment predates a change to the profile or listing"`
}

func apiGrade(g report.Grade) APIGrade {
	return APIGrade{
		Profile: g.Profile, Match: g.Match, Calibrated: g.Calibrated, Score: g.Score, Coverage: g.Coverage,
		MissingEssentials: g.MissingEssentials, Dealbreakers: g.Dealbreakers, AvoidsHit: g.AvoidsHit, Summary: g.Summary, Stale: g.Stale,
	}
}

type APIProfileSummary struct {
	ID      string       `json:"id"`
	Kind    profile.Kind `json:"kind"`
	Name    string       `json:"name"`
	Summary string       `json:"summary"`
}

type APIListingRow struct {
	Rank               int            `json:"rank"`
	Listing            APIListing     `json:"listing"`
	Best               APIGrade       `json:"best" doc:"The best grade across the collection's profiles"`
	Grades             []APIGrade     `json:"grades"`
	AlsoListed         []report.Link  `json:"also_listed,omitempty"`
	FirstSeen          string         `json:"first_seen"`
	New                bool           `json:"new"`
	Status             service.Status `json:"status" enum:"new,changed,repeat" doc:"new: first seen this day; changed: price, description or photos differ from the previous sighting; repeat: unchanged since the previous sighting, so its grade carries over"`
	PreviousPriceCents *int64         `json:"previous_price_cents,omitempty"`
	Favorite           bool           `json:"favorite"`
	Day                string         `json:"day" doc:"The run this row is from"`
}

type APIDay struct {
	Day      string `json:"day"`
	Listings int    `json:"listings"`
}

func (a *App) apiCollections(ctx context.Context, _ *struct{}) (*struct{ Body []APICollection }, error) {
	cs, err := a.svc.Collections(ctx)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := make([]APICollection, len(cs))
	for i, c := range cs {
		if out[i], err = a.apiCollectionWithRun(ctx, c); err != nil {
			return nil, a.apiError(ctx, err)
		}
	}
	return &struct{ Body []APICollection }{out}, nil
}

type collectionInput struct {
	ID string `path:"id"`
}

func (a *App) apiCollection(ctx context.Context, in *collectionInput) (*struct{ Body APICollection }, error) {
	c, err := a.svc.Collection(ctx, in.ID)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out, err := a.apiCollectionWithRun(ctx, c)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	return &struct{ Body APICollection }{out}, nil
}

func (a *App) apiCollectionDays(ctx context.Context, in *collectionInput) (*struct{ Body []APIDay }, error) {
	days, err := a.svc.CollectionDays(ctx, in.ID)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := make([]APIDay, len(days))
	for i, d := range days {
		out[i] = APIDay{Day: d.Day, Listings: d.Listings}
	}
	return &struct{ Body []APIDay }{out}, nil
}

type collectionListingsInput struct {
	ID           string  `path:"id"`
	Day          string  `query:"day" doc:"YYYY-MM-DD; the latest run on or before this day. Defaults to the latest run."`
	From         string  `query:"from" format:"date" doc:"With from or to, list each listing's latest graded sighting across the runs in this range instead of one run"`
	To           string  `query:"to" format:"date"`
	Dealbreakers bool    `query:"dealbreakers" doc:"Include listings that hit a dealbreaker"`
	MinMatch     float64 `query:"min_match" minimum:"0" doc:"Only listings whose best match is at least this"`
	NewOnly      bool    `query:"new_only" doc:"Only listings first seen on this day"`
	HideRepeats  bool    `query:"hide_repeats" doc:"Leave out listings unchanged since their previous sighting"`
	Limit        int     `query:"limit" minimum:"0" doc:"Maximum rows to return; 0 for all"`
	Offset       int     `query:"offset" minimum:"0" doc:"Rows to skip after filtering"`
}

type APICollectionListings struct {
	Collection  APICollection       `json:"collection"`
	Day         string              `json:"day,omitempty" doc:"The run shown; empty when the collection has never run or a range was asked for"`
	From        string              `json:"from,omitempty"`
	To          string              `json:"to,omitempty"`
	PreviousDay string              `json:"previous_day,omitempty"`
	NextDay     string              `json:"next_day,omitempty"`
	Listings    int                 `json:"listings" doc:"Listings observed that day, or across the range"`
	Graded      int                 `json:"graded" doc:"Distinct listings with a grade"`
	Hidden      int                 `json:"hidden" doc:"Graded listings left out for hitting a dealbreaker"`
	Matched     int                 `json:"matched" doc:"Rows that pass the filters, before offset and limit"`
	Stale       bool                `json:"stale"`
	Models      []string            `json:"models"`
	Profiles    []APIProfileSummary `json:"profiles"`
	Rows        []APIListingRow     `json:"rows"`
}

func (a *App) apiCollectionListings(ctx context.Context, in *collectionListingsInput) (*struct{ Body APICollectionListings }, error) {
	var v service.CollectionDayView
	var err error
	if in.From != "" || in.To != "" {
		if in.From != "" && in.To != "" && in.From > in.To {
			in.From, in.To = in.To, in.From
		}
		v, err = a.svc.CollectionSpan(ctx, in.ID, service.SpanOptions{From: in.From, To: in.To, Dealbreakers: in.Dealbreakers})
	} else {
		v, err = a.svc.CollectionDay(ctx, in.ID, service.DayOptions{Day: in.Day, Dealbreakers: in.Dealbreakers})
	}
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := APICollectionListings{
		Collection: apiCollection(v.Collection), Day: v.Day, From: in.From, To: in.To, PreviousDay: v.Prev, NextDay: v.Next,
		Listings: v.Listings, Graded: v.Graded, Hidden: v.Hidden, Stale: v.Stale,
		Models: v.Models, Profiles: []APIProfileSummary{}, Rows: []APIListingRow{},
	}
	if out.Models == nil {
		out.Models = []string{}
	}
	for _, p := range v.Profiles {
		out.Profiles = append(out.Profiles, APIProfileSummary{ID: p.ID, Kind: profile.KindWant, Name: p.Name, Summary: p.Summary})
	}
	for _, row := range v.Rows {
		if row.Match < in.MinMatch || in.NewOnly && !row.New() || in.HideRepeats && row.Repeat() {
			continue
		}
		out.Matched++
		if out.Matched <= in.Offset || in.Limit > 0 && len(out.Rows) == in.Limit {
			continue
		}
		r := APIListingRow{
			Rank: row.Rank, Listing: a.listing(ctx, row.Listing), Best: apiGrade(row.Grade), Grades: []APIGrade{}, AlsoListed: row.AlsoListed,
			FirstSeen: row.FirstSeen, New: row.New(), Status: row.Status, PreviousPriceCents: row.PreviousPriceCents, Favorite: row.Favorite, Day: row.Day,
		}
		for _, id := range v.Collection.Profiles {
			if g, ok := row.Grades[id]; ok {
				r.Grades = append(r.Grades, apiGrade(g))
			}
		}
		out.Rows = append(out.Rows, r)
	}
	return &struct{ Body APICollectionListings }{out}, nil
}

type runInput struct {
	ID string `path:"id"`
}

func (a *App) apiRunCollection(ctx context.Context, in *runInput) (*struct{ Body APIJob }, error) {
	if _, err := a.svc.Collection(ctx, in.ID); err != nil {
		return nil, a.apiError(ctx, err)
	}
	id, err := a.jobs.Enqueue(ctx, jobs.RunCollectionParams{CollectionID: in.ID})
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	j, err := a.jobs.Job(ctx, id)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	return &struct{ Body APIJob }{apiJob(j)}, nil
}

type listingInput struct {
	Source     listing.Source `path:"source"`
	SourceID   string         `path:"source_id"`
	Collection string         `query:"collection" doc:"Grade against this collection's profiles and show the listing as of one of its runs"`
	Day        string         `query:"day" doc:"YYYY-MM-DD; with collection, the run on or before this day"`
}

type APIListingDetail struct {
	Listing    APIListing         `json:"listing"`
	Collection string             `json:"collection,omitempty"`
	Day        string             `json:"day,omitempty"`
	AlsoListed []APIListing       `json:"also_listed"`
	History    []APIPriceSighting `json:"history"`
	Grades     []APIListingGrade  `json:"grades"`
	Favorite   bool               `json:"favorite"`
	Location   *APILocation       `json:"location,omitempty" doc:"Where to place the listing on a map"`
	Gallery    []string           `json:"gallery" doc:"API paths of the listing's stored photos in listing order, which take a size like other media"`
}

type APILocation struct {
	Lat    float64 `json:"lat"`
	Lng    float64 `json:"lng"`
	Approx bool    `json:"approx,omitempty" doc:"Placed at the zip code's center because the listing has no coordinates"`
}

type APIPriceSighting struct {
	Day        string `json:"day"`
	PriceCents int64  `json:"price_cents"`
}

type APIListingGrade struct {
	APIGrade
	ProfileName string           `json:"profile_name"`
	Models      []APIModelResult `json:"models"`
}

type APIModelResult struct {
	Model      string               `json:"model"`
	Stale      bool                 `json:"stale"`
	Score      float64              `json:"score"`
	Coverage   float64              `json:"coverage"`
	Summary    string               `json:"summary"`
	AssessedAt time.Time            `json:"assessed_at"`
	Wants      []APICriterionResult `json:"wants"`
	Avoids     []APICriterionResult `json:"avoids"`
}

type APICriterionResult struct {
	ID          string             `json:"id"`
	Label       string             `json:"label"`
	Importance  profile.Importance `json:"importance"`
	Dealbreaker bool               `json:"dealbreaker,omitempty"`
	Verdict     listing.Verdict    `json:"verdict" enum:"present,partial,absent,unknown"`
	Confidence  listing.Confidence `json:"confidence" enum:"low,medium,high"`
	Evidence    string             `json:"evidence"`
	Photos      []int              `json:"photos" doc:"Indexes into the listing's photos"`
}

func (a *App) apiListing(ctx context.Context, in *listingInput) (*struct{ Body APIListingDetail }, error) {
	v, err := a.svc.ListingDetail(ctx, in.Source, in.SourceID, service.ListingOptions{Collection: in.Collection, Day: in.Day})
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := APIListingDetail{
		Listing: a.listing(ctx, v.Listing), Collection: v.Collection, Day: v.Day,
		AlsoListed: []APIListing{}, History: []APIPriceSighting{}, Grades: []APIListingGrade{}, Favorite: v.Favorite,
		Gallery: mediaPaths(a.svc.StoredPhotos(ctx, v.Listing, 0)),
	}
	if loc, approx, ok := geo.Locate(v.Listing); ok {
		out.Location = &APILocation{Lat: loc.Lat, Lng: loc.Lng, Approx: approx}
	}
	for _, l := range v.AlsoListed {
		out.AlsoListed = append(out.AlsoListed, a.listing(ctx, l))
	}
	for _, s := range v.History {
		out.History = append(out.History, APIPriceSighting{Day: s.Day, PriceCents: s.PriceCents})
	}
	for _, g := range v.Grades {
		lg := APIListingGrade{APIGrade: apiGrade(g.Grade), ProfileName: g.Profile.Name, Models: []APIModelResult{}}
		for _, m := range g.Models {
			lg.Models = append(lg.Models, APIModelResult{
				Model: m.Model, Stale: m.Stale, Score: m.Score, Coverage: m.Coverage, Summary: m.Summary, AssessedAt: m.AssessedAt,
				Wants: apiResults(m.Wants), Avoids: apiResults(m.Avoids),
			})
		}
		out.Grades = append(out.Grades, lg)
	}
	return &struct{ Body APIListingDetail }{out}, nil
}

type listingGalleryInput struct {
	Source   listing.Source `path:"source"`
	SourceID string         `path:"source_id"`
}

type APIListingGallery struct {
	Gallery []string `json:"gallery" doc:"API paths of the listing's stored photos in listing order, which take a size like other media"`
}

func (a *App) apiListingGallery(ctx context.Context, in *listingGalleryInput) (*struct{ Body APIListingGallery }, error) {
	keys, err := a.svc.ListingGallery(ctx, in.Source, in.SourceID)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	return &struct{ Body APIListingGallery }{APIListingGallery{Gallery: mediaPaths(keys)}}, nil
}

func apiResults(rs []service.ResultView) []APICriterionResult {
	out := make([]APICriterionResult, len(rs))
	for i, r := range rs {
		out[i] = APICriterionResult{
			ID: r.CriterionView.ID, Label: r.Label, Importance: r.Importance, Dealbreaker: r.Dealbreaker,
			Verdict: r.Verdict, Confidence: r.Confidence, Evidence: r.Evidence, Photos: r.Photos,
		}
		if out[i].Photos == nil {
			out[i].Photos = []int{}
		}
	}
	return out
}

func (a *App) apiProfiles(ctx context.Context, _ *struct{}) (*struct{ Body []APIProfileListItem }, error) {
	ps, err := a.svc.Profiles(ctx)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	cs, err := a.svc.Collections(ctx)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := make([]APIProfileListItem, len(ps))
	for i, p := range ps {
		out[i] = APIProfileListItem{
			APIProfileSummary: APIProfileSummary{ID: p.ID, Kind: kindOf(p), Name: p.Name, Summary: p.Summary},
			Wants:             len(p.Want), Avoids: len(p.Avoid), References: len(p.References), Collections: []string{}, Drafted: p.Drafted,
		}
		for _, c := range cs {
			if slices.Contains(c.Profiles, p.ID) {
				out[i].Collections = append(out[i].Collections, c.ID)
			}
		}
	}
	return &struct{ Body []APIProfileListItem }{out}, nil
}

type profileInput struct {
	ID string `path:"id"`
}

type APIProfileDetail struct {
	Profile         profile.Profile         `json:"profile"`
	InheritedAvoids []APIInheritedCriterion `json:"inherited_avoids" doc:"Avoid criteria applied from avoid profiles"`
	Collections     []string                `json:"collections" doc:"Collections that use this profile"`
	References      []APIReference          `json:"references" doc:"The listings the profile's criteria were drafted from"`
}

type APIInheritedCriterion struct {
	Profile     string `json:"profile"`
	ProfileName string `json:"profile_name"`
	profile.Criterion
}

func (a *App) apiProfile(ctx context.Context, in *profileInput) (*struct{ Body APIProfileDetail }, error) {
	v, err := a.svc.ProfileDetail(ctx, in.ID)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := APIProfileDetail{Profile: v.Profile, InheritedAvoids: []APIInheritedCriterion{}, Collections: v.Collections, References: []APIReference{}}
	for _, r := range v.References {
		ref := APIReference{Source: r.Source, SourceID: r.SourceID, URL: r.URL, Address: r.Address, Collages: mediaPaths(r.Collages), Gallery: mediaPaths(r.Photos), Scores: r.Scores}
		out.References = append(out.References, ref)
	}
	out.Profile.Kind = kindOf(out.Profile)
	out.Profile.Want, out.Profile.Ignore, out.Profile.References = nonNil(out.Profile.Want), nonNil(out.Profile.Ignore), nonNil(out.Profile.References)
	for i := range out.Profile.References {
		out.Profile.References[i].Collages = nonNil(out.Profile.References[i].Collages)
	}
	if out.Collections == nil {
		out.Collections = []string{}
	}
	for _, c := range v.Inherited {
		out.InheritedAvoids = append(out.InheritedAvoids, APIInheritedCriterion{Profile: c.Profile, ProfileName: c.ProfileName, Criterion: c.Criterion})
	}
	return &struct{ Body APIProfileDetail }{out}, nil
}

type APIJob struct {
	ID           int64       `json:"id"`
	Kind         string      `json:"kind"`
	Trigger      string      `json:"trigger"`
	CollectionID string      `json:"collection_id,omitempty"`
	ProfileID    string      `json:"profile_id,omitempty"`
	Status       jobs.Status `json:"status" enum:"queued,running,succeeded,degraded,failed,cancelled"`
	CreatedAt    time.Time   `json:"created_at"`
	StartedAt    *time.Time  `json:"started_at,omitempty"`
	FinishedAt   *time.Time  `json:"finished_at,omitempty"`
	Error        string      `json:"error,omitempty"`
	CostUSD      float64     `json:"cost_usd"`
	Params       any         `json:"params,omitempty"`
	Result       any         `json:"result,omitempty"`
}

func apiJob(j jobs.Job) APIJob {
	out := APIJob{
		ID: j.ID, Kind: j.Kind, Trigger: j.Trigger, CollectionID: j.CollectionID, ProfileID: j.ProfileID,
		Status: j.Status, CreatedAt: j.CreatedAt, Error: j.Error, CostUSD: j.CostUSD,
	}
	if !j.StartedAt.IsZero() {
		out.StartedAt = &j.StartedAt
	}
	if !j.FinishedAt.IsZero() {
		out.FinishedAt = &j.FinishedAt
	}
	if len(j.Params) > 0 {
		out.Params = j.Params
	}
	if len(j.Result) > 0 {
		out.Result = j.Result
	}
	return out
}

type jobsInput struct {
	Collection string      `query:"collection" doc:"Only this collection's jobs"`
	Profile    string      `query:"profile" doc:"Only jobs that create or draft this profile"`
	Kind       jobs.Kind   `query:"kind" enum:"run_collection,create_profile,draft_profile,prune_media"`
	Status     jobs.Status `query:"status" enum:"queued,running,succeeded,degraded,failed,cancelled"`
	Limit      int         `query:"limit" minimum:"0" default:"20" doc:"Maximum jobs to return; 0 for all"`
	Before     int64       `query:"before" minimum:"0" doc:"Only jobs older than this job id; pass the last id of the previous page"`
}

type jobsOutput struct {
	Link string `header:"Link" doc:"The next page, as rel=\"next\", when there may be more jobs"`
	Body []APIJob
}

func (a *App) apiJobs(ctx context.Context, in *jobsInput) (*jobsOutput, error) {
	js, err := a.jobs.Jobs(ctx, jobs.Filter{Kind: string(in.Kind), CollectionID: in.Collection, ProfileID: in.Profile, Status: in.Status, Limit: in.Limit, BeforeID: in.Before})
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := &jobsOutput{Body: make([]APIJob, len(js))}
	for i, j := range js {
		out.Body[i] = apiJob(j)
	}
	if in.Limit > 0 && len(js) == in.Limit {
		q := url.Values{}
		for k, v := range map[string]string{"collection": in.Collection, "profile": in.Profile, "kind": string(in.Kind), "status": string(in.Status)} {
			if v != "" {
				q.Set(k, v)
			}
		}
		q.Set("limit", strconv.Itoa(in.Limit))
		q.Set("before", strconv.FormatInt(js[len(js)-1].ID, 10))
		out.Link = "<" + apiV1 + "/jobs?" + q.Encode() + `>; rel="next"`
	}
	return out, nil
}

type jobInput struct {
	ID int64 `path:"id"`
}

type jobEventsInput struct {
	ID    int64 `path:"id"`
	After int   `query:"after" minimum:"0" doc:"Only events with a higher seq"`
}

func (a *App) apiJob(ctx context.Context, in *jobInput) (*struct{ Body APIJob }, error) {
	j, err := a.jobs.Job(ctx, in.ID)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	return &struct{ Body APIJob }{apiJob(j)}, nil
}

type APIJobEvent struct {
	Seq     int       `json:"seq"`
	At      time.Time `json:"at"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
	Record  any       `json:"record,omitempty" doc:"Structured attributes logged with the message"`
}

func (a *App) apiJobEvents(ctx context.Context, in *jobEventsInput) (*struct{ Body []APIJobEvent }, error) {
	if _, err := a.jobs.Job(ctx, in.ID); err != nil {
		return nil, a.apiError(ctx, err)
	}
	out, err := a.jobEventsAfter(ctx, in.ID, in.After)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	return &struct{ Body []APIJobEvent }{out}, nil
}

func (a *App) jobEventsAfter(ctx context.Context, id int64, after int) ([]APIJobEvent, error) {
	es, err := a.jobs.Events(ctx, id)
	if err != nil {
		return nil, err
	}
	out := []APIJobEvent{}
	for _, e := range es {
		if e.Seq <= after {
			continue
		}
		ev := APIJobEvent{Seq: e.Seq, At: e.At, Level: e.Level.String(), Message: e.Message}
		if len(e.Record) > 0 {
			ev.Record = e.Record
		}
		out = append(out, ev)
	}
	return out, nil
}

type mediaInput struct {
	Key  string `path:"key" doc:"A collage path from a listing's collages, without the /api/v1/media/ prefix"`
	Size int    `query:"size" enum:"320,640,1280" doc:"Scale the image down so its longest side is at most this many pixels"`
}

type mediaOutput struct {
	ContentType  string `header:"Content-Type"`
	CacheControl string `header:"Cache-Control"`
	Body         []byte
}

const mediaCacheControl = "private, max-age=31536000, immutable"

func (a *App) apiMedia(ctx context.Context, in *mediaInput) (*mediaOutput, error) {
	p, err := a.svc.MediaPath(in.Key)
	if err != nil {
		return nil, huma.Error404NotFound("no such media")
	}
	if in.Size > 0 {
		if sp, err := a.svc.MediaPath(media.SizedKey(in.Key, in.Size)); err == nil {
			if b, err := os.ReadFile(sp); err == nil {
				return &mediaOutput{ContentType: "image/jpeg", CacheControl: mediaCacheControl, Body: b}, nil
			}
		}
	}
	ctype := mime.TypeByExtension(path.Ext(in.Key))
	b, err := os.ReadFile(p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, huma.Error404NotFound("no such media")
	}
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	if ctype == "" {
		ctype = http.DetectContentType(b)
	}
	if !strings.HasPrefix(ctype, "image/") {
		return nil, huma.Error404NotFound("no such media")
	}
	if in.Size > 0 {
		if small, err := a.resized(p, b, in.Size); err != nil {
			a.log.WarnContext(ctx, "resize media", "key", in.Key, "size", in.Size, "err", err)
		} else if small != nil {
			b, ctype = small, "image/jpeg"
		}
	}
	return &mediaOutput{ContentType: ctype, CacheControl: mediaCacheControl, Body: b}, nil
}

func (a *App) resized(p string, original []byte, size int) ([]byte, error) {
	cached := media.SizedKey(p, size)
	b, err := media.Shrink(original, size)
	if err != nil || bytes.Equal(b, original) {
		return nil, err
	}
	if err := renameio.WriteFile(cached, b, 0o644); err != nil {
		a.log.Warn("cache resized media", "path", cached, "err", err)
	}
	return b, nil
}

type APIMeta struct {
	Version      string               `json:"version"`
	Modes        []profile.Mode       `json:"modes"`
	Sources      []listing.Source     `json:"sources" doc:"Listing sources this server can search"`
	Amenities    []listing.Amenity    `json:"amenities"`
	Importances  []profile.Importance `json:"importances"`
	ProfileKinds []profile.Kind       `json:"profile_kinds"`
	Defaults     APIDefaults          `json:"defaults"`
}

type APIDefaults struct {
	AssessModel         string  `json:"assess_model"`
	DraftModel          string  `json:"draft_model"`
	CollectionBudgetUSD float64 `json:"collection_budget_usd"`
	LookupMaxChargeUSD  float64 `json:"lookup_max_charge_usd"`
	NotifyMinScore      float64 `json:"notify_min_score"`
}

func (a *App) apiMeta(_ context.Context, _ *struct{}) (*struct{ Body APIMeta }, error) {
	return &struct{ Body APIMeta }{APIMeta{
		Version:      a.Version,
		Modes:        []profile.Mode{profile.ModeRent, profile.ModeBuy},
		Sources:      service.SupportedSources(),
		Amenities:    listing.Amenities,
		Importances:  profile.Importances,
		ProfileKinds: []profile.Kind{profile.KindWant, profile.KindAvoid},
		Defaults: APIDefaults{
			AssessModel:         profile.DefaultAssessModel,
			DraftModel:          profile.DefaultDraftModel,
			CollectionBudgetUSD: service.DefaultCollectionBudgetUSD,
			LookupMaxChargeUSD:  service.DefaultLookupMaxChargeUSD,
			NotifyMinScore:      collection.DefaultNotifyMinScore,
		},
	}}, nil
}

func collectErrors(err error, fields map[string]string, general *[]string) {
	if err == nil {
		return
	}
	if fe, ok := err.(profile.FieldError); ok {
		if prev, dup := fields[fe.Field]; dup {
			fields[fe.Field] = prev + "; " + fe.Error()
		} else {
			fields[fe.Field] = fe.Error()
		}
		return
	}
	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		for _, e := range multi.Unwrap() {
			collectErrors(e, fields, general)
		}
		return
	}
	var fe profile.FieldError
	if errors.As(err, &fe) {
		collectErrors(errors.Unwrap(err), fields, general)
		return
	}
	*general = append(*general, err.Error())
}
