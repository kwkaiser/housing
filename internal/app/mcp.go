package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"git.kwkaiser.io/kwkaiser/housing/internal/app/view"
	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

const (
	mcpPath              = apiPrefix + "/mcp"
	mcpDefaultListings   = 25
	mcpMaxCollages       = 4
	mcpRecentJobEvents   = 20
	mcpServerInstruction = "Tools for browsing apartment and house listings that housing has fetched and graded against the user's profiles. " +
		"A collection is a saved search graded against one or more want profiles; it runs at most once a day. " +
		"Start with list_collections, then list_listings for ranked results, then get_listing for one listing's details and photos. " +
		"match is a 0-100+ score relative to the profile's reference listings; around 100 means as good as the references."
)

func (a *App) mcpHandler() http.Handler {
	server := sync.OnceValue(a.mcpServer)
	h := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server() }, &mcp.StreamableHTTPOptions{
		Stateless:                  true,
		JSONResponse:               true,
		DisableLocalhostProtection: true,
	})
	return a.bearerAuth(h)
}

func (a *App) mcpServer() *mcp.Server {
	srv := mcp.NewServer(&mcp.Implementation{Name: "housing", Version: a.Version}, &mcp.ServerOptions{Instructions: mcpServerInstruction})
	readOnly := &mcp.ToolAnnotations{ReadOnlyHint: true}

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_collections",
		Description: "List saved collections with their search area, profiles and most recent run day.",
		Annotations: readOnly,
	}, a.mcpListCollections)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_listings",
		Description: "List a collection's listings for one run day, best match first. Listings that hit a dealbreaker are left out unless include_dealbreakers is set.",
		Annotations: readOnly,
	}, a.mcpListListings)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_listing",
		Description: "Get one listing: its details, price history, and how it scored against each profile criterion, with the photo collages it was graded from.",
		Annotations: readOnly,
	}, a.mcpGetListing)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "list_profiles",
		Description: "List profiles. Want profiles describe what the user is looking for; avoid profiles list things to steer clear of and apply to every want profile.",
		Annotations: readOnly,
	}, a.mcpListProfiles)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_profile",
		Description: "Get a profile's criteria and the collections that use it.",
		Annotations: readOnly,
	}, a.mcpGetProfile)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "run_collection",
		Description: "Queue a run of a collection: fetch fresh listings and grade them. This spends money on scraping and model calls and takes several minutes; only call it when the user asks. Poll get_job for progress.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false), OpenWorldHint: new(true)},
	}, a.mcpRunCollection)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "get_job",
		Description: "Get a job's status, result and most recent progress messages.",
		Annotations: readOnly,
	}, a.mcpGetJob)
	return srv
}

func (a *App) bearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ok, err := a.authenticate(r.Context(), r.Header.Get("Authorization"))
		if err != nil {
			a.log.Error("mcp auth", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		if !ok {
			w.Header().Set("WWW-Authenticate", `Bearer realm="housing"`)
			http.Error(w, "a valid API key is required", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type mcpNoInput struct{}

type mcpCollection struct {
	ID       string   `json:"id"`
	Mode     string   `json:"mode" jsonschema:"rent or sale"`
	Location string   `json:"location"`
	Profiles []string `json:"profiles"`
	Schedule string   `json:"schedule,omitempty" jsonschema:"daily run time, HH:MM server-local"`
	LastDay  string   `json:"last_day,omitempty" jsonschema:"most recent run day, YYYY-MM-DD"`
	Days     int      `json:"days" jsonschema:"number of days the collection has run"`
}

type mcpCollections struct {
	Collections []mcpCollection `json:"collections"`
}

func (a *App) mcpListCollections(ctx context.Context, _ *mcp.CallToolRequest, _ mcpNoInput) (*mcp.CallToolResult, mcpCollections, error) {
	cs, err := a.svc.Collections(ctx)
	if err != nil {
		return nil, mcpCollections{}, a.apiError(ctx, err)
	}
	out := mcpCollections{Collections: []mcpCollection{}}
	for _, c := range cs {
		days, err := a.svc.Store().CollectionDays(ctx, c.ID)
		if err != nil {
			return nil, mcpCollections{}, a.apiError(ctx, err)
		}
		mc := mcpCollection{ID: c.ID, Mode: string(c.Mode), Location: c.Search.Location, Profiles: c.Profiles, Schedule: c.Schedule, Days: len(days)}
		if len(days) > 0 {
			mc.LastDay = days[len(days)-1].Day
		}
		out.Collections = append(out.Collections, mc)
	}
	return nil, out, nil
}

type mcpListListingsInput struct {
	Collection          string  `json:"collection" jsonschema:"collection id"`
	Day                 string  `json:"day,omitempty" jsonschema:"YYYY-MM-DD; the latest run on or before this day. Defaults to the latest run."`
	MinMatch            float64 `json:"min_match,omitempty" jsonschema:"only listings whose best match is at least this"`
	NewOnly             bool    `json:"new_only,omitempty" jsonschema:"only listings first seen on this day"`
	IncludeDealbreakers bool    `json:"include_dealbreakers,omitempty"`
	Limit               int     `json:"limit,omitempty" jsonschema:"maximum listings to return, default 25"`
}

type mcpListingRow struct {
	Rank          int      `json:"rank"`
	Source        string   `json:"source"`
	SourceID      string   `json:"source_id"`
	Address       string   `json:"address"`
	Price         string   `json:"price"`
	PreviousPrice string   `json:"previous_price,omitempty" jsonschema:"price on the previous sighting, when it changed"`
	Beds          *int     `json:"beds,omitempty"`
	Baths         *float64 `json:"baths,omitempty"`
	SqFt          *int     `json:"sqft,omitempty"`
	Match         float64  `json:"match"`
	Profile       string   `json:"profile" jsonschema:"the profile this listing matched best"`
	Summary       string   `json:"summary"`
	Dealbreakers  []string `json:"dealbreakers,omitempty"`
	New           bool     `json:"new" jsonschema:"first seen on this day"`
	FirstSeen     string   `json:"first_seen"`
	URL           string   `json:"url"`
	AlsoListedOn  []string `json:"also_listed_on,omitempty"`
}

type mcpListings struct {
	Collection  string          `json:"collection"`
	Day         string          `json:"day,omitempty" jsonschema:"the run shown; empty when the collection has never run"`
	PreviousDay string          `json:"previous_day,omitempty"`
	NextDay     string          `json:"next_day,omitempty"`
	Graded      int             `json:"graded" jsonschema:"distinct listings graded that day"`
	Hidden      int             `json:"hidden" jsonschema:"listings left out for hitting a dealbreaker"`
	Matching    int             `json:"matching" jsonschema:"listings matching the filters, before the limit"`
	Listings    []mcpListingRow `json:"listings"`
}

func (a *App) mcpListListings(ctx context.Context, _ *mcp.CallToolRequest, in mcpListListingsInput) (*mcp.CallToolResult, mcpListings, error) {
	res, err := a.apiCollectionListings(ctx, &collectionListingsInput{
		ID: in.Collection, Day: in.Day, Dealbreakers: in.IncludeDealbreakers, MinMatch: in.MinMatch, NewOnly: in.NewOnly,
	})
	if err != nil {
		return nil, mcpListings{}, err
	}
	v := res.Body
	limit := in.Limit
	if limit <= 0 {
		limit = mcpDefaultListings
	}
	out := mcpListings{Collection: v.Collection.ID, Day: v.Day, PreviousDay: v.PreviousDay, NextDay: v.NextDay, Graded: v.Graded, Hidden: v.Hidden, Matching: len(v.Rows), Listings: []mcpListingRow{}}
	for _, r := range v.Rows[:min(limit, len(v.Rows))] {
		l := r.Listing
		row := mcpListingRow{
			Rank: r.Rank, Source: string(l.Source), SourceID: l.SourceID, Address: l.Address.Formatted, Price: view.Money(l.Price.Cents, l.Offer),
			Beds: l.Beds, Baths: l.Baths, SqFt: l.SqFt, Match: r.Best.Match, Profile: r.Best.Profile, Summary: r.Best.Summary,
			Dealbreakers: r.Best.Dealbreakers, New: r.New, FirstSeen: r.FirstSeen, URL: l.URL,
		}
		if r.PreviousPriceCents != nil && *r.PreviousPriceCents != l.Price.Cents {
			row.PreviousPrice = view.Money(*r.PreviousPriceCents, l.Offer)
		}
		for _, also := range r.AlsoListed {
			row.AlsoListedOn = append(row.AlsoListedOn, string(also.Source))
		}
		out.Listings = append(out.Listings, row)
	}
	return nil, out, nil
}

type mcpGetListingInput struct {
	Source     string `json:"source" jsonschema:"listing source, e.g. zillow"`
	SourceID   string `json:"source_id"`
	Collection string `json:"collection,omitempty" jsonschema:"grade against this collection's profiles and show the listing as of one of its runs"`
	Day        string `json:"day,omitempty" jsonschema:"YYYY-MM-DD; with collection, the run on or before this day"`
	NoPhotos   bool   `json:"no_photos,omitempty" jsonschema:"leave out the photo collages"`
}

type mcpListing struct {
	Source       string            `json:"source"`
	SourceID     string            `json:"source_id"`
	URL          string            `json:"url"`
	Address      string            `json:"address"`
	Price        string            `json:"price"`
	Beds         *int              `json:"beds,omitempty"`
	Baths        *float64          `json:"baths,omitempty"`
	SqFt         *int              `json:"sqft,omitempty"`
	Amenities    []string          `json:"amenities,omitempty"`
	Description  string            `json:"description,omitempty"`
	PriceHistory []mcpPrice        `json:"price_history"`
	AlsoListed   []mcpAlsoListed   `json:"also_listed,omitempty"`
	Grades       []mcpListingGrade `json:"grades"`
	Photos       int               `json:"photos" jsonschema:"number of photo collages attached"`
}

type mcpPrice struct {
	Day   string `json:"day"`
	Price string `json:"price"`
}

type mcpAlsoListed struct {
	Source string `json:"source"`
	URL    string `json:"url"`
	Price  string `json:"price"`
}

type mcpListingGrade struct {
	Profile           string        `json:"profile"`
	ProfileName       string        `json:"profile_name"`
	Match             float64       `json:"match"`
	Summary           string        `json:"summary"`
	MissingEssentials []string      `json:"missing_essentials,omitempty"`
	Dealbreakers      []string      `json:"dealbreakers,omitempty"`
	Stale             bool          `json:"stale,omitempty" jsonschema:"graded before the profile or listing last changed"`
	Models            []mcpModelRun `json:"models"`
}

type mcpModelRun struct {
	Model    string         `json:"model"`
	Score    float64        `json:"score"`
	Criteria []mcpCriterion `json:"criteria"`
}

type mcpCriterion struct {
	Kind       string `json:"kind" jsonschema:"want or avoid"`
	Label      string `json:"label"`
	Importance string `json:"importance"`
	Verdict    string `json:"verdict" jsonschema:"present, partial, absent or unknown"`
	Confidence string `json:"confidence"`
	Evidence   string `json:"evidence"`
}

func (a *App) mcpGetListing(ctx context.Context, _ *mcp.CallToolRequest, in mcpGetListingInput) (*mcp.CallToolResult, mcpListing, error) {
	res, err := a.apiListing(ctx, &listingInput{Source: listing.Source(in.Source), SourceID: in.SourceID, Collection: in.Collection, Day: in.Day})
	if err != nil {
		return nil, mcpListing{}, err
	}
	v := res.Body
	l := v.Listing
	out := mcpListing{
		Source: string(l.Source), SourceID: l.SourceID, URL: l.URL, Address: l.Address.Formatted, Price: view.Money(l.Price.Cents, l.Offer),
		Beds: l.Beds, Baths: l.Baths, SqFt: l.SqFt, Description: l.Description, PriceHistory: []mcpPrice{}, Grades: []mcpListingGrade{},
	}
	for k, ok := range l.Amenities {
		if ok {
			out.Amenities = append(out.Amenities, k)
		}
	}
	slices.Sort(out.Amenities)
	for _, s := range v.History {
		out.PriceHistory = append(out.PriceHistory, mcpPrice{Day: s.Day, Price: view.Money(s.PriceCents, l.Offer)})
	}
	for _, also := range v.AlsoListed {
		out.AlsoListed = append(out.AlsoListed, mcpAlsoListed{Source: string(also.Source), URL: also.URL, Price: view.Money(also.Price.Cents, also.Offer)})
	}
	for _, g := range v.Grades {
		mg := mcpListingGrade{Profile: g.Profile, ProfileName: g.ProfileName, Match: g.Match, Summary: g.Summary, MissingEssentials: g.MissingEssentials, Dealbreakers: g.Dealbreakers, Stale: g.Stale, Models: []mcpModelRun{}}
		for _, m := range g.Models {
			run := mcpModelRun{Model: m.Model, Score: m.Score, Criteria: []mcpCriterion{}}
			add := func(kind string, rs []APICriterionResult) {
				for _, r := range rs {
					run.Criteria = append(run.Criteria, mcpCriterion{Kind: kind, Label: r.Label, Importance: string(r.Importance), Verdict: string(r.Verdict), Confidence: string(r.Confidence), Evidence: r.Evidence})
				}
			}
			add("want", m.Wants)
			add("avoid", m.Avoids)
			mg.Models = append(mg.Models, run)
		}
		out.Grades = append(out.Grades, mg)
	}

	var images []mcp.Content
	if !in.NoPhotos {
		for _, p := range l.Collages {
			if len(images) == mcpMaxCollages {
				break
			}
			img, err := a.mcpImage(strings.TrimPrefix(p, apiV1+"/media/"))
			if err != nil {
				a.log.WarnContext(ctx, "mcp collage", "path", p, "err", err)
				continue
			}
			images = append(images, img)
		}
	}
	out.Photos = len(images)
	if len(images) == 0 {
		return nil, out, nil
	}
	text, err := json.Marshal(out)
	if err != nil {
		return nil, mcpListing{}, err
	}
	return &mcp.CallToolResult{Content: append([]mcp.Content{&mcp.TextContent{Text: string(text)}}, images...)}, out, nil
}

func (a *App) mcpImage(key string) (*mcp.ImageContent, error) {
	p, err := a.svc.MediaPath(key)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	ctype := http.DetectContentType(b)
	if !strings.HasPrefix(ctype, "image/") {
		return nil, fmt.Errorf("not an image: %s", ctype)
	}
	return &mcp.ImageContent{Data: b, MIMEType: ctype}, nil
}

type mcpProfileSummary struct {
	ID      string `json:"id"`
	Kind    string `json:"kind" jsonschema:"want or avoid"`
	Name    string `json:"name"`
	Summary string `json:"summary"`
}

type mcpProfiles struct {
	Profiles []mcpProfileSummary `json:"profiles"`
}

func (a *App) mcpListProfiles(ctx context.Context, _ *mcp.CallToolRequest, _ mcpNoInput) (*mcp.CallToolResult, mcpProfiles, error) {
	ps, err := a.svc.Profiles(ctx)
	if err != nil {
		return nil, mcpProfiles{}, a.apiError(ctx, err)
	}
	out := mcpProfiles{Profiles: []mcpProfileSummary{}}
	for _, p := range ps {
		out.Profiles = append(out.Profiles, mcpProfileSummary{ID: p.ID, Kind: string(kindOf(p)), Name: p.Name, Summary: p.Summary})
	}
	return nil, out, nil
}

type mcpIDInput struct {
	ID string `json:"id"`
}

type mcpProfile struct {
	mcpProfileSummary
	Wants       []mcpProfileCriterion `json:"wants"`
	Avoids      []mcpProfileCriterion `json:"avoids" jsonschema:"the profile's own avoids plus those inherited from avoid profiles"`
	Collections []string              `json:"collections"`
}

type mcpProfileCriterion struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	LookFor    string `json:"look_for"`
	Importance string `json:"importance" jsonschema:"essential, high, medium or low; essential avoids are dealbreakers"`
	From       string `json:"from,omitempty" jsonschema:"the avoid profile this was inherited from"`
}

func (a *App) mcpGetProfile(ctx context.Context, _ *mcp.CallToolRequest, in mcpIDInput) (*mcp.CallToolResult, mcpProfile, error) {
	v, err := a.svc.ProfileDetail(ctx, in.ID)
	if err != nil {
		return nil, mcpProfile{}, a.apiError(ctx, err)
	}
	p := v.Profile
	out := mcpProfile{
		mcpProfileSummary: mcpProfileSummary{ID: p.ID, Kind: string(kindOf(p)), Name: p.Name, Summary: p.Summary},
		Wants:             []mcpProfileCriterion{}, Avoids: []mcpProfileCriterion{}, Collections: v.Collections,
	}
	if out.Collections == nil {
		out.Collections = []string{}
	}
	criterion := func(c profile.Criterion, from string) mcpProfileCriterion {
		return mcpProfileCriterion{ID: c.ID, Label: c.Label, LookFor: c.LookFor, Importance: string(c.Importance), From: from}
	}
	for _, c := range p.Want {
		out.Wants = append(out.Wants, criterion(c, ""))
	}
	for _, c := range p.Avoid {
		out.Avoids = append(out.Avoids, criterion(c, ""))
	}
	for _, c := range v.Inherited {
		out.Avoids = append(out.Avoids, criterion(c.Criterion, c.Profile))
	}
	return nil, out, nil
}

type mcpRunInput struct {
	Collection string `json:"collection" jsonschema:"collection id"`
}

type mcpJob struct {
	ID         int64           `json:"id"`
	Kind       string          `json:"kind"`
	Collection string          `json:"collection,omitempty"`
	Profile    string          `json:"profile,omitempty"`
	Status     string          `json:"status" jsonschema:"queued, running, succeeded, failed or cancelled"`
	CreatedAt  string          `json:"created_at"`
	StartedAt  string          `json:"started_at,omitempty"`
	FinishedAt string          `json:"finished_at,omitempty"`
	Error      string          `json:"error,omitempty"`
	CostUSD    float64         `json:"cost_usd"`
	Result     json.RawMessage `json:"result,omitempty"`
	Recent     []string        `json:"recent,omitempty" jsonschema:"most recent progress messages, oldest first"`
}

func mcpJobFor(j jobs.Job) mcpJob {
	out := mcpJob{ID: j.ID, Kind: j.Kind, Collection: j.CollectionID, Profile: j.ProfileID, Status: string(j.Status),
		CreatedAt: j.CreatedAt.Format(time.RFC3339), Error: j.Error, CostUSD: j.CostUSD, Result: j.Result}
	if !j.StartedAt.IsZero() {
		out.StartedAt = j.StartedAt.Format(time.RFC3339)
	}
	if !j.FinishedAt.IsZero() {
		out.FinishedAt = j.FinishedAt.Format(time.RFC3339)
	}
	return out
}

func (a *App) mcpRunCollection(ctx context.Context, _ *mcp.CallToolRequest, in mcpRunInput) (*mcp.CallToolResult, mcpJob, error) {
	res, err := a.apiRunCollection(ctx, &runInput{ID: in.Collection})
	if err != nil {
		return nil, mcpJob{}, err
	}
	j, err := a.jobs.Job(ctx, res.Body.ID)
	if err != nil {
		return nil, mcpJob{}, a.apiError(ctx, err)
	}
	return nil, mcpJobFor(j), nil
}

type mcpJobInput struct {
	ID int64 `json:"id"`
}

func (a *App) mcpGetJob(ctx context.Context, _ *mcp.CallToolRequest, in mcpJobInput) (*mcp.CallToolResult, mcpJob, error) {
	j, err := a.jobs.Job(ctx, in.ID)
	if err != nil {
		return nil, mcpJob{}, a.apiError(ctx, err)
	}
	es, err := a.jobs.Events(ctx, in.ID)
	if err != nil {
		return nil, mcpJob{}, a.apiError(ctx, err)
	}
	out := mcpJobFor(j)
	for _, e := range es[max(0, len(es)-mcpRecentJobEvents):] {
		out.Recent = append(out.Recent, e.Message)
	}
	return nil, out, nil
}
