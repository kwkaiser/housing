package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

type CollectionFields struct {
	Mode           *string  `json:"mode,omitempty" enum:"rent,buy" jsonschema:"rent or buy" doc:"rent or buy"`
	Sources        []string `json:"sources,omitempty" jsonschema:"listing sources to search, e.g. zillow, redfin, streeteasy, craigslist, facebook" doc:"Listing sources to search"`
	Profiles       []string `json:"profiles,omitempty" jsonschema:"want profile ids, in priority order" doc:"Want profile ids, in priority order"`
	Location       *string  `json:"location,omitempty" jsonschema:"search area, e.g. a ZIP code or city" doc:"Search area, e.g. a ZIP code or city"`
	RadiusMiles    *float64 `json:"radius_miles,omitempty" minimum:"0"`
	MinPrice       *int     `json:"min_price,omitempty" jsonschema:"whole dollars" doc:"Whole dollars"`
	MaxPrice       *int     `json:"max_price,omitempty" jsonschema:"whole dollars" doc:"Whole dollars"`
	MinBeds        *int     `json:"min_beds,omitempty"`
	MaxBeds        *int     `json:"max_beds,omitempty"`
	MaxAgeDays     *int     `json:"max_age_days,omitempty" jsonschema:"only listings posted within this many days; 0 means no limit" doc:"Only listings posted within this many days; 0 means no limit"`
	Amenities      []string `json:"amenities,omitempty" jsonschema:"required amenities; an empty list clears them" doc:"Required amenities; an empty list clears them"`
	Limit          *int     `json:"limit,omitempty" jsonschema:"maximum listings fetched per source; 0 means the default" doc:"Maximum listings fetched per source; 0 means the default"`
	Model          *string  `json:"model,omitempty" jsonschema:"grading model; empty means the default" doc:"Grading model; empty means the default"`
	MaxRunCostUSD  *float64 `json:"max_run_cost_usd,omitempty" jsonschema:"spending cap per run; 0 means the default" doc:"Spending cap per run; 0 means the default"`
	Schedule       *string  `json:"schedule,omitempty" jsonschema:"daily run time, HH:MM server-local; empty to unschedule" doc:"Daily run time, HH:MM server-local; empty to unschedule"`
	NotifyURL      *string  `json:"notify_url,omitempty" jsonschema:"ntfy topic URL for notifications; empty to turn them off" doc:"ntfy topic URL for notifications; empty to turn them off"`
	NotifyMinScore *float64 `json:"notify_min_score,omitempty" jsonschema:"minimum match to notify about; 0 means the default" doc:"Minimum match to notify about; 0 means the default"`
}

func (f CollectionFields) apply(c *collection.Collection) {
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

func clearBounds(c *collection.Collection, names []string) error {
	bounds := map[string]**int{"min_price": &c.Search.MinPrice, "max_price": &c.Search.MaxPrice, "min_beds": &c.Search.MinBeds, "max_beds": &c.Search.MaxBeds}
	for _, name := range names {
		p, ok := bounds[name]
		if !ok {
			return profile.FieldError{Field: "clear", Err: fmt.Errorf("cannot clear %q: only min_price, max_price, min_beds and max_beds can be cleared", name)}
		}
		*p = nil
	}
	return nil
}

type APICreateCollection struct {
	ID string `json:"id" doc:"Lowercase letters, digits and dashes"`
	CollectionFields
}

type APIUpdateCollection struct {
	CollectionFields
	Clear []string `json:"clear,omitempty" doc:"Price and bed bounds to remove" enum:"min_price,max_price,min_beds,max_beds"`
}

func (a *App) apiCollectionWithRun(ctx context.Context, c collection.Collection) (APICollection, error) {
	out := apiCollection(c)
	j, ok, err := a.jobs.ActiveForCollection(ctx, c.ID)
	if err != nil {
		return out, err
	}
	if !ok {
		if j, ok, err = a.jobs.LatestForCollection(ctx, c.ID); err != nil {
			return out, err
		}
	}
	if ok {
		run := apiJob(j)
		out.LastRun = &run
	}
	return out, nil
}

func (a *App) apiCreateCollection(ctx context.Context, in *struct{ Body APICreateCollection }) (*struct{ Body APICollection }, error) {
	c := collection.Collection{ID: strings.TrimSpace(in.Body.ID)}
	in.Body.apply(&c)
	if err := a.svc.CreateCollection(ctx, c); err != nil {
		return nil, a.apiSaveError(ctx, err)
	}
	return a.apiCollection(ctx, &collectionInput{ID: c.ID})
}

func (a *App) apiUpdateCollection(ctx context.Context, in *struct {
	ID   string `path:"id"`
	Body APIUpdateCollection
}) (*struct{ Body APICollection }, error) {
	c, err := a.svc.Collection(ctx, in.ID)
	if err != nil {
		return nil, a.apiError(ctx, err)
	}
	in.Body.apply(&c)
	if err := clearBounds(&c, in.Body.Clear); err != nil {
		return nil, a.apiSaveError(ctx, err)
	}
	if err := a.svc.SaveCollection(ctx, c); err != nil {
		return nil, a.apiSaveError(ctx, err)
	}
	return a.apiCollection(ctx, &collectionInput{ID: c.ID})
}
