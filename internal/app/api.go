package app

import (
	"context"
	"errors"
	"mime"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

const (
	apiPrefix      = "/api"
	apiV1          = apiPrefix + "/v1"
	apiAuthScheme  = "bearer"
	apiSpecVersion = "1.0.0"
)

func (a *App) apiHandler() http.Handler {
	mux := http.NewServeMux()
	cfg := huma.DefaultConfig("housing", apiSpecVersion)
	cfg.CreateHooks = nil
	cfg.OpenAPIPath = apiPrefix + "/openapi"
	cfg.DocsPath = apiPrefix + "/docs"
	cfg.SchemasPath = apiPrefix + "/schemas"
	cfg.Info.Description = "Read collections, ranked listings, profiles and jobs, and queue collection runs."
	cfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		apiAuthScheme: {Type: "http", Scheme: "bearer", Description: "An API key created on the server's API keys page or with `housing apikeys create`."},
	}
	cfg.Security = []map[string][]string{{apiAuthScheme: {}}}
	api := humago.New(mux, cfg)
	api.UseMiddleware(a.apiAuth(api))

	huma.Get(api, apiV1+"/collections", a.apiCollections, op("listCollections", "List collections", "collections"))
	huma.Get(api, apiV1+"/collections/{id}", a.apiCollection, op("getCollection", "Get a collection", "collections"))
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
	huma.Get(api, apiV1+"/profiles", a.apiProfiles, op("listProfiles", "List profiles", "profiles"))
	huma.Get(api, apiV1+"/profiles/{id}", a.apiProfile, op("getProfile", "Get a profile", "profiles"))
	huma.Get(api, apiV1+"/jobs", a.apiJobs, op("listJobs", "List jobs, newest first", "jobs"))
	huma.Get(api, apiV1+"/jobs/{id}", a.apiJob, op("getJob", "Get a job", "jobs"))
	huma.Get(api, apiV1+"/jobs/{id}/events", a.apiJobEvents, op("listJobEvents", "List a job's progress log", "jobs"))
	mux.Handle(mcpPath, a.mcpHandler())
	huma.Get(api, apiV1+"/media/{key...}", a.apiMedia, op("getMedia", "Get a stored image such as a listing collage", "media"))
	paths := api.OpenAPI().Paths
	paths[apiV1+"/media/{key}"] = paths[apiV1+"/media/{key...}"]
	delete(paths, apiV1+"/media/{key...}")
	return mux
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
		ok, err := a.authenticate(ctx.Context(), ctx.Header("Authorization"))
		if err != nil {
			a.log.ErrorContext(ctx.Context(), "api auth", "err", err)
			huma.WriteErr(api, ctx, http.StatusInternalServerError, "internal error")
			return
		}
		if !ok {
			ctx.SetHeader("WWW-Authenticate", `Bearer realm="housing"`)
			huma.WriteErr(api, ctx, http.StatusUnauthorized, "a valid API key is required")
			return
		}
		next(ctx)
	}
}

func (a *App) authenticate(ctx context.Context, header string) (bool, error) {
	token, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return false, nil
	}
	_, ok, err := a.svc.AuthenticateAPIKey(ctx, token)
	return ok, err
}

func (a *App) apiError(ctx context.Context, err error) error {
	if errors.Is(err, collection.ErrNotFound) || errors.Is(err, store.ErrListingNotFound) || errors.Is(err, store.ErrJobNotFound) || errors.Is(err, profile.ErrNotFound) {
		return huma.Error404NotFound(err.Error())
	}
	a.log.ErrorContext(ctx, "api", "err", err)
	return huma.Error500InternalServerError("internal error")
}

type APICollection struct {
	ID       string           `json:"id"`
	Mode     profile.Mode     `json:"mode"`
	Sources  []listing.Source `json:"sources"`
	Profiles []string         `json:"profiles"`
	Search   profile.Search   `json:"search"`
	Model    string           `json:"model,omitempty"`
	Schedule string           `json:"schedule,omitempty" doc:"Daily run time, HH:MM in server-local time"`
}

func apiCollection(c collection.Collection) APICollection {
	return APICollection{ID: c.ID, Mode: c.Mode, Sources: c.Sources, Profiles: c.Profiles, Search: c.Search, Model: c.Model, Schedule: c.Schedule}
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
	ListedAt    *time.Time           `json:"listed_at,omitempty"`
	ObservedAt  time.Time            `json:"observed_at"`
}

func apiListing(l listing.Listing) APIListing {
	out := APIListing{
		Source: l.Source, SourceID: l.SourceID, URL: l.URL, Href: apiListingPath(l.Source, l.SourceID), Offer: l.Offer, Price: l.Price, Address: l.Address,
		Coordinates: l.Coordinates, Beds: l.Beds, Baths: l.Baths, SqFt: l.SqFt, Description: l.Description,
		Photos: l.Photos, ListedAt: l.ListedAt, ObservedAt: l.ObservedAt,
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
		out[i] = apiCollection(c)
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
	return &struct{ Body APICollection }{apiCollection(c)}, nil
}

func (a *App) apiCollectionDays(ctx context.Context, in *collectionInput) (*struct{ Body []APIDay }, error) {
	v, err := a.svc.CollectionDay(ctx, in.ID, service.DayOptions{})
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := make([]APIDay, len(v.Days))
	for i, d := range v.Days {
		out[i] = APIDay{Day: d.Day, Listings: d.Listings}
	}
	return &struct{ Body []APIDay }{out}, nil
}

type collectionListingsInput struct {
	ID           string  `path:"id"`
	Day          string  `query:"day" doc:"YYYY-MM-DD; the latest run on or before this day. Defaults to the latest run."`
	Dealbreakers bool    `query:"dealbreakers" doc:"Include listings that hit a dealbreaker"`
	MinMatch     float64 `query:"min_match" minimum:"0" doc:"Only listings whose best match is at least this"`
	NewOnly      bool    `query:"new_only" doc:"Only listings first seen on this day"`
	HideRepeats  bool    `query:"hide_repeats" doc:"Leave out listings unchanged since their previous sighting"`
	Limit        int     `query:"limit" minimum:"0" doc:"Maximum rows to return; 0 for all"`
}

type APICollectionListings struct {
	Collection  APICollection       `json:"collection"`
	Day         string              `json:"day,omitempty" doc:"The run shown; empty when the collection has never run"`
	PreviousDay string              `json:"previous_day,omitempty"`
	NextDay     string              `json:"next_day,omitempty"`
	Listings    int                 `json:"listings" doc:"Listings observed that day"`
	Graded      int                 `json:"graded" doc:"Distinct listings with a grade"`
	Hidden      int                 `json:"hidden" doc:"Graded listings left out for hitting a dealbreaker"`
	Stale       bool                `json:"stale"`
	Models      []string            `json:"models"`
	Profiles    []APIProfileSummary `json:"profiles"`
	Rows        []APIListingRow     `json:"rows"`
}

func (a *App) apiCollectionListings(ctx context.Context, in *collectionListingsInput) (*struct{ Body APICollectionListings }, error) {
	v, err := a.svc.CollectionDay(ctx, in.ID, service.DayOptions{Day: in.Day, Dealbreakers: in.Dealbreakers})
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := APICollectionListings{
		Collection: apiCollection(v.Collection), Day: v.Day, PreviousDay: v.Prev, NextDay: v.Next,
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
		r := APIListingRow{
			Rank: row.Rank, Listing: apiListing(row.Listing), Best: apiGrade(row.Grade), Grades: []APIGrade{}, AlsoListed: row.AlsoListed,
			FirstSeen: row.FirstSeen, New: row.New(), Status: row.Status, PreviousPriceCents: row.PreviousPriceCents,
		}
		for _, id := range v.Collection.Profiles {
			if g, ok := row.Grades[id]; ok {
				r.Grades = append(r.Grades, apiGrade(g))
			}
		}
		out.Rows = append(out.Rows, r)
		if in.Limit > 0 && len(out.Rows) == in.Limit {
			break
		}
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
		Listing: apiListing(v.Listing), Collection: v.Collection, Day: v.Day,
		AlsoListed: []APIListing{}, History: []APIPriceSighting{}, Grades: []APIListingGrade{},
	}
	for _, l := range v.AlsoListed {
		out.AlsoListed = append(out.AlsoListed, apiListing(l))
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

func (a *App) apiProfiles(ctx context.Context, _ *struct{}) (*struct{ Body []APIProfileSummary }, error) {
	ps, err := a.svc.Profiles(ctx)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := make([]APIProfileSummary, len(ps))
	for i, p := range ps {
		out[i] = APIProfileSummary{ID: p.ID, Kind: kindOf(p), Name: p.Name, Summary: p.Summary}
	}
	return &struct{ Body []APIProfileSummary }{out}, nil
}

type profileInput struct {
	ID string `path:"id"`
}

type APIProfileDetail struct {
	Profile         profile.Profile         `json:"profile"`
	InheritedAvoids []APIInheritedCriterion `json:"inherited_avoids" doc:"Avoid criteria applied from avoid profiles"`
	Collections     []string                `json:"collections" doc:"Collections that use this profile"`
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
	out := APIProfileDetail{Profile: v.Profile, InheritedAvoids: []APIInheritedCriterion{}, Collections: v.Collections}
	out.Profile.Kind = kindOf(out.Profile)
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
	Status       jobs.Status `json:"status" enum:"queued,running,succeeded,failed,cancelled"`
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
	Status     jobs.Status `query:"status" enum:"queued,running,succeeded,failed,cancelled"`
	Limit      int         `query:"limit" minimum:"0" default:"20" doc:"Maximum jobs to return; 0 for all"`
}

func (a *App) apiJobs(ctx context.Context, in *jobsInput) (*struct{ Body []APIJob }, error) {
	js, err := a.jobs.Jobs(ctx, jobs.Filter{CollectionID: in.Collection, Status: in.Status, Limit: in.Limit})
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := make([]APIJob, len(js))
	for i, j := range js {
		out[i] = apiJob(j)
	}
	return &struct{ Body []APIJob }{out}, nil
}

type jobInput struct {
	ID int64 `path:"id"`
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

func (a *App) apiJobEvents(ctx context.Context, in *jobInput) (*struct{ Body []APIJobEvent }, error) {
	if _, err := a.jobs.Job(ctx, in.ID); err != nil {
		return nil, a.apiError(ctx, err)
	}
	es, err := a.jobs.Events(ctx, in.ID)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	out := make([]APIJobEvent, len(es))
	for i, e := range es {
		out[i] = APIJobEvent{Seq: e.Seq, At: e.At, Level: e.Level.String(), Message: e.Message}
		if len(e.Record) > 0 {
			out[i].Record = e.Record
		}
	}
	return &struct{ Body []APIJobEvent }{out}, nil
}

type mediaInput struct {
	Key string `path:"key" doc:"A collage path from a listing's collages, without the /api/v1/media/ prefix"`
}

type mediaOutput struct {
	ContentType  string `header:"Content-Type"`
	CacheControl string `header:"Cache-Control"`
	Body         []byte
}

func (a *App) apiMedia(ctx context.Context, in *mediaInput) (*mediaOutput, error) {
	p, err := a.svc.MediaPath(in.Key)
	if err != nil {
		return nil, huma.Error404NotFound("no such media")
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
	return &mediaOutput{ContentType: ctype, CacheControl: "private, max-age=31536000, immutable", Body: b}, nil
}
