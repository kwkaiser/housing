package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

func (a *App) addMCPMutations(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_collection",
		Description: "Create a collection: a saved search graded against one or more want profiles. Does not run it; call run_collection for that.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false)},
	}, a.mcpCreateCollection)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_collection",
		Description: "Change a collection's settings. Only the fields given are changed; call get_collection first to see the current values.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(true), IdempotentHint: true},
	}, a.mcpUpdateCollection)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "update_profile",
		Description: "Change a profile's name, summary, notes, or its own criteria's labels and importance, or remove criteria. Only the fields given are changed. Criteria inherited from avoid profiles must be edited on that avoid profile. Changed criteria make existing grades stale until the next run.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(true), IdempotentHint: true},
	}, a.mcpUpdateProfile)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "create_profile",
		Description: "Queue creation of a profile from a reference listing URL. Looking up the listing spends money on scraping, and drafting criteria spends money on a model call; only call it when the user asks. Poll get_job for progress.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(false), OpenWorldHint: new(true)},
	}, a.mcpCreateProfile)
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "redraft_profile",
		Description: "Queue a re-draft of a profile's criteria from its reference listings, replacing the current criteria. This spends money on a model call; only call it when the user asks. Poll get_job for progress.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: new(true), OpenWorldHint: new(true)},
	}, a.mcpRedraftProfile)
}

func (a *App) mcpSaveError(ctx context.Context, err error) error {
	var fe profile.FieldError
	if errors.As(err, &fe) {
		return err
	}
	return a.apiError(ctx, err)
}

type mcpCollectionDetail struct {
	ID             string         `json:"id"`
	Mode           string         `json:"mode" jsonschema:"rent or buy"`
	Sources        []string       `json:"sources"`
	Profiles       []string       `json:"profiles" jsonschema:"want profile ids, in priority order"`
	Search         profile.Search `json:"search"`
	Model          string         `json:"model,omitempty" jsonschema:"grading model; empty means the default"`
	MaxRunCostUSD  float64        `json:"max_run_cost_usd,omitempty" jsonschema:"spending cap per run; 0 means the default"`
	Schedule       string         `json:"schedule,omitempty" jsonschema:"daily run time, HH:MM server-local; empty means not scheduled"`
	NotifyURL      string         `json:"notify_url,omitempty"`
	NotifyMinScore float64        `json:"notify_min_score,omitempty" jsonschema:"minimum match to notify about; 0 means the default"`
}

func mcpCollectionFor(c collection.Collection) mcpCollectionDetail {
	out := mcpCollectionDetail{
		ID: c.ID, Mode: string(c.Mode), Sources: []string{}, Profiles: c.Profiles, Search: c.Search, Model: c.Model,
		MaxRunCostUSD: c.MaxRunCostUSD, Schedule: c.Schedule, NotifyURL: c.Notify.URL, NotifyMinScore: c.Notify.MinScore,
	}
	for _, s := range c.Sources {
		out.Sources = append(out.Sources, string(s))
	}
	if out.Profiles == nil {
		out.Profiles = []string{}
	}
	return out
}

func (a *App) mcpGetCollection(ctx context.Context, _ *mcp.CallToolRequest, in mcpIDInput) (*mcp.CallToolResult, mcpCollectionDetail, error) {
	c, err := a.svc.Collection(ctx, in.ID)
	if err != nil {
		return nil, mcpCollectionDetail{}, a.apiError(ctx, err)
	}
	return nil, mcpCollectionFor(c), nil
}

type mcpCollectionFields struct {
	Mode           *string  `json:"mode,omitempty" jsonschema:"rent or buy"`
	Sources        []string `json:"sources,omitempty" jsonschema:"listing sources to search, e.g. zillow, redfin, streeteasy, craigslist, facebook"`
	Profiles       []string `json:"profiles,omitempty" jsonschema:"want profile ids, in priority order"`
	Location       *string  `json:"location,omitempty" jsonschema:"search area, e.g. a ZIP code or city"`
	RadiusMiles    *float64 `json:"radius_miles,omitempty"`
	MinPrice       *int     `json:"min_price,omitempty" jsonschema:"whole dollars"`
	MaxPrice       *int     `json:"max_price,omitempty" jsonschema:"whole dollars"`
	MinBeds        *int     `json:"min_beds,omitempty"`
	MaxBeds        *int     `json:"max_beds,omitempty"`
	MaxAgeDays     *int     `json:"max_age_days,omitempty" jsonschema:"only listings posted within this many days; 0 means no limit"`
	Amenities      []string `json:"amenities,omitempty" jsonschema:"required amenities; an empty list clears them"`
	Limit          *int     `json:"limit,omitempty" jsonschema:"maximum listings fetched per source; 0 means the default"`
	Model          *string  `json:"model,omitempty" jsonschema:"grading model; empty means the default"`
	MaxRunCostUSD  *float64 `json:"max_run_cost_usd,omitempty" jsonschema:"spending cap per run; 0 means the default"`
	Schedule       *string  `json:"schedule,omitempty" jsonschema:"daily run time, HH:MM server-local; empty to unschedule"`
	NotifyURL      *string  `json:"notify_url,omitempty" jsonschema:"ntfy topic URL for notifications; empty to turn them off"`
	NotifyMinScore *float64 `json:"notify_min_score,omitempty" jsonschema:"minimum match to notify about; 0 means the default"`
}

func (f mcpCollectionFields) apply(c *collection.Collection) {
	set := func(dst *string, src *string) {
		if src != nil {
			*dst = strings.TrimSpace(*src)
		}
	}
	setPtr := func(dst **int, src *int) {
		if src != nil {
			*dst = new(*src)
		}
	}
	if f.Mode != nil {
		c.Mode = profile.Mode(strings.TrimSpace(*f.Mode))
	}
	if f.Sources != nil {
		c.Sources = nil
		for _, s := range f.Sources {
			c.Sources = append(c.Sources, listing.Source(strings.TrimSpace(s)))
		}
	}
	if f.Profiles != nil {
		c.Profiles = slices.Clone(f.Profiles)
	}
	set(&c.Search.Location, f.Location)
	if f.RadiusMiles != nil {
		c.Search.RadiusMiles = *f.RadiusMiles
	}
	setPtr(&c.Search.MinPrice, f.MinPrice)
	setPtr(&c.Search.MaxPrice, f.MaxPrice)
	setPtr(&c.Search.MinBeds, f.MinBeds)
	setPtr(&c.Search.MaxBeds, f.MaxBeds)
	if f.MaxAgeDays != nil {
		c.Search.MaxAgeDays = *f.MaxAgeDays
	}
	if f.Amenities != nil {
		c.Search.Amenities = nil
		for _, am := range f.Amenities {
			c.Search.Amenities = append(c.Search.Amenities, listing.Amenity(strings.TrimSpace(am)))
		}
	}
	if f.Limit != nil {
		c.Search.Limit = *f.Limit
	}
	set(&c.Model, f.Model)
	if f.MaxRunCostUSD != nil {
		c.MaxRunCostUSD = *f.MaxRunCostUSD
	}
	set(&c.Schedule, f.Schedule)
	set(&c.Notify.URL, f.NotifyURL)
	if f.NotifyMinScore != nil {
		c.Notify.MinScore = *f.NotifyMinScore
	}
}

type mcpCreateCollectionInput struct {
	ID string `json:"id" jsonschema:"lowercase letters, digits and dashes"`
	mcpCollectionFields
}

func (a *App) mcpCreateCollection(ctx context.Context, _ *mcp.CallToolRequest, in mcpCreateCollectionInput) (*mcp.CallToolResult, mcpCollectionDetail, error) {
	c := collection.Collection{ID: strings.TrimSpace(in.ID)}
	in.apply(&c)
	if err := a.svc.CreateCollection(ctx, c); err != nil {
		return nil, mcpCollectionDetail{}, a.mcpSaveError(ctx, err)
	}
	return a.mcpGetCollection(ctx, nil, mcpIDInput{ID: c.ID})
}

type mcpUpdateCollectionInput struct {
	ID string `json:"id"`
	mcpCollectionFields
	Clear []string `json:"clear,omitempty" jsonschema:"price and bed bounds to remove: min_price, max_price, min_beds or max_beds"`
}

func (a *App) mcpUpdateCollection(ctx context.Context, _ *mcp.CallToolRequest, in mcpUpdateCollectionInput) (*mcp.CallToolResult, mcpCollectionDetail, error) {
	c, err := a.svc.Collection(ctx, in.ID)
	if err != nil {
		return nil, mcpCollectionDetail{}, a.apiError(ctx, err)
	}
	in.apply(&c)
	bounds := map[string]**int{"min_price": &c.Search.MinPrice, "max_price": &c.Search.MaxPrice, "min_beds": &c.Search.MinBeds, "max_beds": &c.Search.MaxBeds}
	for _, name := range in.Clear {
		p, ok := bounds[name]
		if !ok {
			return nil, mcpCollectionDetail{}, fmt.Errorf("cannot clear %q: only min_price, max_price, min_beds and max_beds can be cleared", name)
		}
		*p = nil
	}
	if err := a.svc.SaveCollection(ctx, c); err != nil {
		return nil, mcpCollectionDetail{}, a.mcpSaveError(ctx, err)
	}
	return a.mcpGetCollection(ctx, nil, mcpIDInput{ID: c.ID})
}

type mcpUpdateProfileInput struct {
	ID       string             `json:"id"`
	Name     *string            `json:"name,omitempty"`
	Summary  *string            `json:"summary,omitempty"`
	Notes    []string           `json:"notes,omitempty" jsonschema:"guidance for drafting criteria, one note per entry; an empty list clears them"`
	Criteria []mcpCriterionEdit `json:"criteria,omitempty" jsonschema:"edits to the profile's own want and avoid criteria, by id"`
}

type mcpCriterionEdit struct {
	ID         string `json:"id"`
	Label      string `json:"label,omitempty" jsonschema:"new label; unchanged when empty"`
	Importance string `json:"importance,omitempty" jsonschema:"essential, high, medium or low; unchanged when empty"`
	Remove     bool   `json:"remove,omitempty"`
}

func (a *App) mcpUpdateProfile(ctx context.Context, _ *mcp.CallToolRequest, in mcpUpdateProfileInput) (*mcp.CallToolResult, mcpProfile, error) {
	p, err := a.svc.Profile(ctx, in.ID)
	if err != nil {
		return nil, mcpProfile{}, a.apiError(ctx, err)
	}
	e := service.ProfileEdit{Name: p.Name, Summary: p.Summary, Notes: p.Notes}
	if in.Name != nil {
		e.Name = *in.Name
	}
	if in.Summary != nil {
		e.Summary = *in.Summary
	}
	if in.Notes != nil {
		e.Notes = nil
		for _, n := range in.Notes {
			if n = strings.TrimSpace(n); n != "" {
				e.Notes = append(e.Notes, n)
			}
		}
	}
	own := slices.Concat(p.Want, p.Avoid)
	for _, ce := range in.Criteria {
		i := slices.IndexFunc(own, func(c profile.Criterion) bool { return c.ID == ce.ID })
		if i < 0 {
			return nil, mcpProfile{}, fmt.Errorf("profile %s has no criterion %q of its own", p.ID, ce.ID)
		}
		edit := service.CriterionEdit{ID: ce.ID, Label: own[i].Label, Importance: own[i].Importance, Remove: ce.Remove}
		if ce.Label != "" {
			edit.Label = ce.Label
		}
		if ce.Importance != "" {
			edit.Importance = profile.Importance(ce.Importance)
		}
		e.Criteria = append(e.Criteria, edit)
	}
	if _, err := a.svc.UpdateProfile(ctx, p.ID, e); err != nil {
		return nil, mcpProfile{}, a.mcpSaveError(ctx, err)
	}
	return a.mcpGetProfile(ctx, nil, mcpIDInput{ID: p.ID})
}

type mcpCreateProfileInput struct {
	URL          string   `json:"url" jsonschema:"a listing URL to use as the profile's reference"`
	ID           string   `json:"id" jsonschema:"lowercase letters, digits and dashes"`
	Kind         string   `json:"kind,omitempty" jsonschema:"want or avoid; defaults to want"`
	Name         string   `json:"name,omitempty"`
	Notes        []string `json:"notes,omitempty" jsonschema:"guidance for drafting criteria, one note per entry"`
	NoDraft      bool     `json:"no_draft,omitempty" jsonschema:"save the reference without drafting criteria"`
	Model        string   `json:"model,omitempty" jsonschema:"drafting model; empty means the default"`
	MaxChargeUSD float64  `json:"max_charge_usd,omitempty" jsonschema:"scraping spend cap for the lookup; 0 means the default"`
}

func (a *App) mcpCreateProfile(ctx context.Context, _ *mcp.CallToolRequest, in mcpCreateProfileInput) (*mcp.CallToolResult, mcpJob, error) {
	p := jobs.CreateProfileParams{
		URL: strings.TrimSpace(in.URL), ID: strings.TrimSpace(in.ID), ProfileKind: profile.Kind(strings.TrimSpace(in.Kind)), Name: strings.TrimSpace(in.Name),
		Notes: in.Notes, NoDraft: in.NoDraft, MaxChargeUSD: in.MaxChargeUSD,
	}
	if !p.NoDraft {
		p.Model = strings.TrimSpace(in.Model)
	}
	err := a.svc.ValidateNewProfile(ctx, service.CreateProfileOptions{
		URL: p.URL, ID: p.ID, Kind: p.ProfileKind, Name: p.Name, Notes: p.Notes, Model: p.Model, NoDraft: p.NoDraft, MaxChargeUSD: p.MaxChargeUSD,
	})
	if err != nil {
		return nil, mcpJob{}, a.mcpSaveError(ctx, err)
	}
	return a.mcpEnqueue(ctx, p)
}

type mcpRedraftProfileInput struct {
	ID    string   `json:"id"`
	Model string   `json:"model,omitempty" jsonschema:"drafting model; empty means the default"`
	Notes []string `json:"notes,omitempty" jsonschema:"replacement drafting notes; the profile's current notes are used when omitted"`
}

func (a *App) mcpRedraftProfile(ctx context.Context, _ *mcp.CallToolRequest, in mcpRedraftProfileInput) (*mcp.CallToolResult, mcpJob, error) {
	p, err := a.svc.Profile(ctx, in.ID)
	if err != nil {
		return nil, mcpJob{}, a.apiError(ctx, err)
	}
	if len(p.References) == 0 {
		return nil, mcpJob{}, fmt.Errorf("profile %s has no reference listings, so its criteria can't be re-drafted", p.ID)
	}
	params := jobs.DraftProfileParams{ProfileID: p.ID, Model: cmp.Or(strings.TrimSpace(in.Model), profile.DefaultDraftModel)}
	if in.Notes != nil {
		params.Notes = slices.Clone(in.Notes)
	}
	return a.mcpEnqueue(ctx, params)
}

func (a *App) mcpEnqueue(ctx context.Context, p jobs.Params) (*mcp.CallToolResult, mcpJob, error) {
	id, err := a.jobs.Enqueue(ctx, p)
	if err != nil {
		return nil, mcpJob{}, a.apiError(ctx, err)
	}
	j, err := a.jobs.Job(ctx, id)
	if err != nil {
		return nil, mcpJob{}, a.apiError(ctx, err)
	}
	return nil, mcpJobFor(j), nil
}
