package app

import (
	"context"
	"errors"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
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

type mcpCreateCollectionInput struct {
	ID string `json:"id" jsonschema:"lowercase letters, digits and dashes"`
	CollectionFields
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
	CollectionFields
	Clear []string `json:"clear,omitempty" jsonschema:"price and bed bounds to remove: min_price, max_price, min_beds or max_beds"`
}

func (a *App) mcpUpdateCollection(ctx context.Context, _ *mcp.CallToolRequest, in mcpUpdateCollectionInput) (*mcp.CallToolResult, mcpCollectionDetail, error) {
	c, err := a.svc.Collection(ctx, in.ID)
	if err != nil {
		return nil, mcpCollectionDetail{}, a.apiError(ctx, err)
	}
	in.apply(&c)
	if err := clearBounds(&c, in.Clear); err != nil {
		return nil, mcpCollectionDetail{}, err
	}
	if err := a.svc.SaveCollection(ctx, c); err != nil {
		return nil, mcpCollectionDetail{}, a.mcpSaveError(ctx, err)
	}
	return a.mcpGetCollection(ctx, nil, mcpIDInput{ID: c.ID})
}

type mcpUpdateProfileInput struct {
	ID string `json:"id"`
	ProfilePatch
}

func (a *App) mcpUpdateProfile(ctx context.Context, _ *mcp.CallToolRequest, in mcpUpdateProfileInput) (*mcp.CallToolResult, mcpProfile, error) {
	if err := a.patchProfile(ctx, in.ID, in.ProfilePatch); err != nil {
		return nil, mcpProfile{}, a.mcpSaveError(ctx, err)
	}
	return a.mcpGetProfile(ctx, nil, mcpIDInput{ID: in.ID})
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
	params, err := a.redraftParams(ctx, in.ID, in.Model, in.Notes)
	if errors.Is(err, errNoReferences) {
		return nil, mcpJob{}, err
	}
	if err != nil {
		return nil, mcpJob{}, a.apiError(ctx, err)
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
