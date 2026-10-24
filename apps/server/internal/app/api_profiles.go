package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

var errNoReferences = errors.New("profile has no reference listings")

type ProfilePatch struct {
	Name     *string          `json:"name,omitempty"`
	Summary  *string          `json:"summary,omitempty"`
	Notes    []string         `json:"notes,omitempty" jsonschema:"guidance for drafting criteria, one note per entry; an empty list clears them" doc:"Guidance for drafting criteria, one note per entry; an empty list clears them"`
	Criteria []CriterionPatch `json:"criteria,omitempty" jsonschema:"edits to the profile's own want and avoid criteria, by id" doc:"Edits to the profile's own want and avoid criteria, by id"`
}

type CriterionPatch struct {
	ID         string `json:"id"`
	Label      string `json:"label,omitempty" jsonschema:"new label; unchanged when empty" doc:"New label; unchanged when empty"`
	Importance string `json:"importance,omitempty" enum:"essential,high,medium,low" jsonschema:"essential, high, medium or low; unchanged when empty" doc:"Unchanged when empty"`
	Remove     bool   `json:"remove,omitempty"`
}

func (a *App) patchProfile(ctx context.Context, id string, in ProfilePatch) error {
	p, err := a.svc.Profile(ctx, id)
	if err != nil {
		return err
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
			return profile.FieldError{Field: "criteria", Err: fmt.Errorf("profile %s has no criterion %q of its own", p.ID, ce.ID)}
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
	_, err = a.svc.UpdateProfile(ctx, p.ID, e)
	return err
}

func (a *App) redraftParams(ctx context.Context, id, model string, notes []string) (jobs.DraftProfileParams, error) {
	p, err := a.svc.Profile(ctx, id)
	if err != nil {
		return jobs.DraftProfileParams{}, err
	}
	if len(p.References) == 0 {
		return jobs.DraftProfileParams{}, fmt.Errorf("%w: %s can't be re-drafted", errNoReferences, p.ID)
	}
	params := jobs.DraftProfileParams{ProfileID: p.ID, Model: cmp.Or(strings.TrimSpace(model), profile.DefaultDraftModel)}
	if notes != nil {
		params.Notes = slices.Clone(notes)
	}
	return params, nil
}

type APIProfileListItem struct {
	APIProfileSummary
	Wants       int              `json:"wants"`
	Avoids      int              `json:"avoids"`
	References  int              `json:"references"`
	Collections []string         `json:"collections" doc:"Collections that use this profile"`
	Drafted     *profile.Drafted `json:"drafted,omitempty"`
}

type APIReference struct {
	Source   listing.Source     `json:"source"`
	SourceID string             `json:"source_id"`
	URL      string             `json:"url"`
	Address  string             `json:"address"`
	Collages []string           `json:"collages" doc:"API paths of the reference's photo collages"`
	Scores   map[string]float64 `json:"scores" doc:"The reference's own score under each grading model"`
}

type APICreateProfile struct {
	URL          string       `json:"url" doc:"A listing URL to use as the profile's reference"`
	ID           string       `json:"id" doc:"Lowercase letters, digits and dashes"`
	Kind         profile.Kind `json:"kind,omitempty" enum:"want,avoid" doc:"Defaults to want"`
	Name         string       `json:"name,omitempty"`
	Notes        []string     `json:"notes,omitempty" doc:"Guidance for drafting criteria, one note per entry"`
	NoDraft      bool         `json:"no_draft,omitempty" doc:"Save the reference without drafting criteria"`
	Force        bool         `json:"force,omitempty" doc:"Replace an existing profile with the same id"`
	Model        string       `json:"model,omitempty" doc:"Drafting model; empty means the default"`
	MaxChargeUSD float64      `json:"max_charge_usd,omitempty" minimum:"0" doc:"Scraping spend cap for the lookup; 0 means the default"`
}

type APIRedraftProfile struct {
	Model string   `json:"model,omitempty" doc:"Drafting model; empty means the default"`
	Notes []string `json:"notes,omitempty" doc:"Replacement drafting notes; the profile's current notes are used when omitted"`
}

func (a *App) apiCreateProfile(ctx context.Context, in *struct{ Body APICreateProfile }) (*struct{ Body APIJob }, error) {
	b := in.Body
	p := jobs.CreateProfileParams{
		URL: strings.TrimSpace(b.URL), ID: strings.TrimSpace(b.ID), ProfileKind: b.Kind, Name: strings.TrimSpace(b.Name),
		Notes: b.Notes, NoDraft: b.NoDraft, Force: b.Force, MaxChargeUSD: b.MaxChargeUSD,
	}
	if !p.NoDraft {
		p.Model = cmp.Or(strings.TrimSpace(b.Model), profile.DefaultDraftModel)
	}
	err := a.svc.ValidateNewProfile(ctx, service.CreateProfileOptions{
		URL: p.URL, ID: p.ID, Kind: p.ProfileKind, Name: p.Name, Notes: p.Notes, Model: p.Model,
		NoDraft: p.NoDraft, Force: p.Force, MaxChargeUSD: p.MaxChargeUSD,
	})
	if err != nil {
		return nil, a.apiSaveError(ctx, err)
	}
	return a.apiEnqueue(ctx, p)
}

func (a *App) apiUpdateProfile(ctx context.Context, in *struct {
	ID   string `path:"id"`
	Body ProfilePatch
}) (*struct{ Body APIProfileDetail }, error) {
	if err := a.patchProfile(ctx, in.ID, in.Body); err != nil {
		return nil, a.apiSaveError(ctx, err)
	}
	return a.apiProfile(ctx, &profileInput{ID: in.ID})
}

func (a *App) apiRedraftProfile(ctx context.Context, in *struct {
	ID   string `path:"id"`
	Body *APIRedraftProfile
}) (*struct{ Body APIJob }, error) {
	var b APIRedraftProfile
	if in.Body != nil {
		b = *in.Body
	}
	params, err := a.redraftParams(ctx, in.ID, b.Model, b.Notes)
	if errors.Is(err, errNoReferences) {
		return nil, huma.Error409Conflict(err.Error())
	}
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	return a.apiEnqueue(ctx, params)
}

func (a *App) apiEnqueue(ctx context.Context, p jobs.Params) (*struct{ Body APIJob }, error) {
	id, err := a.jobs.Enqueue(ctx, p)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	j, err := a.jobs.Job(ctx, id)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	return &struct{ Body APIJob }{apiJob(j)}, nil
}

func kindOf(p profile.Profile) profile.Kind {
	return cmp.Or(p.Kind, profile.KindWant)
}
