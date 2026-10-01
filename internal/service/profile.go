package service

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

const DefaultLookupMaxChargeUSD = 0.1

type CreateProfileOptions struct {
	URL          string
	ID           string
	Kind         profile.Kind
	Name         string
	Notes        []string
	Model        string
	NoDraft      bool
	Force        bool
	MaxChargeUSD float64
}

type DraftOptions struct {
	Model string
	Notes []string
}

func (s *Service) CreateProfileFromURL(ctx context.Context, o CreateProfileOptions, progress Progress) (profile.Profile, error) {
	o = o.withDefaults()
	db, err := s.openCatalog(ctx)
	if err != nil {
		return profile.Profile{}, err
	}
	defer db.Close()
	if err := s.checkNewProfile(ctx, db, o); err != nil {
		return profile.Profile{}, err
	}

	if !o.NoDraft {
		if _, err := s.cfg.Keys.OpenRouter(); err != nil {
			return profile.Profile{}, err
		}
	}
	token, err := s.cfg.Keys.Apify()
	if err != nil {
		return profile.Profile{}, err
	}
	lookup, err := s.cfg.Clients.Lookup(o.URL, s.cfg.Clients.Apify(token, o.MaxChargeUSD))
	if err != nil {
		return profile.Profile{}, err
	}
	ref, err := lookup.Lookup(ctx, o.URL)
	if err != nil {
		return profile.Profile{}, err
	}
	s.emit(progress, StageProfile, "reference: %s (%d photos)", ref.Address.Formatted, len(ref.Photos))

	processor := media.NewProcessor(s.photoFetcher(), s.images(), media.NewGrid())
	processed, err := processor.Process(ctx, []listing.Listing{ref})
	if err != nil {
		return profile.Profile{}, err
	}
	ref = processed[0]
	if len(ref.Collages) == 0 {
		return profile.Profile{}, fmt.Errorf("no photos could be downloaded for %s", ref.URL)
	}
	s.emit(progress, StageProfile, "collages: %s", strings.Join(ref.Collages, ", "))

	p := profile.Profile{
		ID:     o.ID,
		Kind:   o.Kind,
		Name:   o.Name,
		Notes:  o.Notes,
		Ignore: profile.DefaultIgnore,
		References: []profile.Reference{{
			Source:   ref.Source,
			SourceID: ref.SourceID,
			URL:      ref.URL,
			Collages: ref.Collages,
		}},
	}
	if !o.NoDraft {
		if err := s.draft(ctx, &p, o.Model, []listing.Listing{ref}, progress); err != nil {
			if saveErr := db.SaveProfile(ctx, p, ref); saveErr != nil {
				return p, fmt.Errorf("%w (and saving undrafted profile failed: %w)", err, saveErr)
			}
			return p, fmt.Errorf("%w; saved undrafted profile, re-draft it from its profile page", err)
		}
	}
	return p, s.saveProfile(ctx, db, p, progress, ref)
}

func (s *Service) DraftProfile(ctx context.Context, id string, o DraftOptions, progress Progress) (profile.Profile, error) {
	db, err := s.openCatalog(ctx)
	if err != nil {
		return profile.Profile{}, err
	}
	defer db.Close()
	p, err := db.Profile(ctx, id)
	if err != nil {
		return profile.Profile{}, err
	}
	refs, err := db.ReferenceListings(ctx, p)
	if err != nil {
		return profile.Profile{}, err
	}
	if o.Notes != nil {
		p.Notes = o.Notes
	}
	if err := s.draft(ctx, &p, o.Model, refs, progress); err != nil {
		return p, err
	}
	return p, s.saveProfile(ctx, db, p, progress)
}

func (o CreateProfileOptions) withDefaults() CreateProfileOptions {
	o.URL, o.ID, o.Name = strings.TrimSpace(o.URL), strings.TrimSpace(o.ID), strings.TrimSpace(o.Name)
	o.Kind = cmp.Or(o.Kind, profile.KindWant)
	o.Model = cmp.Or(strings.TrimSpace(o.Model), profile.DefaultDraftModel)
	if o.MaxChargeUSD == 0 {
		o.MaxChargeUSD = DefaultLookupMaxChargeUSD
	}
	return o
}

func (s *Service) ValidateNewProfile(ctx context.Context, o CreateProfileOptions) error {
	db, err := s.openCatalog(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	return s.checkNewProfile(ctx, db, o.withDefaults())
}

func (s *Service) checkNewProfile(ctx context.Context, db *store.Store, o CreateProfileOptions) error {
	var errs []error
	fail := func(field string, err error) {
		errs = append(errs, profile.FieldError{Field: field, Err: err})
	}
	if err := checkLookupURL(o.URL); err != nil {
		fail("url", err)
	} else if _, err := s.cfg.Clients.Lookup(o.URL, nil); err != nil {
		fail("url", err)
	}
	if err := profile.ValidID(o.ID); err != nil {
		fail("id", err)
	} else if !o.Force {
		exists, err := db.ProfileExists(ctx, o.ID)
		if err != nil {
			return err
		}
		if exists {
			fail("id", fmt.Errorf("%w: %s", profile.ErrExists, o.ID))
		}
	}
	if _, err := profile.ParseKind(string(o.Kind)); err != nil {
		fail("kind", err)
	}
	if o.MaxChargeUSD < 0 || math.IsNaN(o.MaxChargeUSD) || math.IsInf(o.MaxChargeUSD, 0) {
		fail("max_charge_usd", fmt.Errorf("max charge must be a positive amount"))
	}
	return errors.Join(errs...)
}

func checkLookupURL(raw string) error {
	if raw == "" {
		return fmt.Errorf("a listing URL is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Hostname() == "" {
		return fmt.Errorf("invalid listing URL %q: want an http or https link", raw)
	}
	return nil
}

type ProfileEdit struct {
	Name     string
	Summary  string
	Notes    []string
	Criteria []CriterionEdit
}

type CriterionEdit struct {
	ID         string
	Label      string
	Importance profile.Importance
	Remove     bool
}

func (s *Service) UpdateProfile(ctx context.Context, id string, e ProfileEdit) (profile.Profile, error) {
	db, err := s.openCatalog(ctx)
	if err != nil {
		return profile.Profile{}, err
	}
	defer db.Close()
	p, err := db.Profile(ctx, id)
	if err != nil {
		return profile.Profile{}, err
	}
	edited, err := applyEdit(p, e)
	if err != nil {
		return p, err
	}
	if err := db.SaveProfile(ctx, edited); err != nil {
		return p, err
	}
	return edited, nil
}

func applyEdit(p profile.Profile, e ProfileEdit) (profile.Profile, error) {
	var errs []error
	edits := make(map[string]CriterionEdit, len(e.Criteria))
	for _, c := range e.Criteria {
		edits[c.ID] = c
	}
	known := map[string]bool{}
	apply := func(cs []profile.Criterion) []profile.Criterion {
		var out []profile.Criterion
		for _, c := range cs {
			known[c.ID] = true
			ce, ok := edits[c.ID]
			if !ok {
				out = append(out, c)
				continue
			}
			if ce.Remove {
				continue
			}
			if c.Label = ce.Label; strings.TrimSpace(c.Label) == "" {
				errs = append(errs, profile.FieldError{Field: "criteria." + c.ID + ".label", Err: fmt.Errorf("a label is required")})
			}
			if c.Importance = ce.Importance; !slices.Contains(profile.Importances, c.Importance) {
				errs = append(errs, profile.FieldError{Field: "criteria." + c.ID + ".importance", Err: fmt.Errorf("invalid importance %q", ce.Importance)})
			}
			out = append(out, c)
		}
		if out == nil {
			return cs[:0:0]
		}
		return out
	}
	next := p
	next.Name = strings.TrimSpace(e.Name)
	next.Summary = e.Summary
	next.Notes = e.Notes
	next.Want = apply(p.Want)
	next.Avoid = apply(p.Avoid)
	for _, c := range e.Criteria {
		if !known[c.ID] {
			errs = append(errs, profile.FieldError{Field: "criteria", Err: fmt.Errorf("criterion %q is no longer part of this profile; reload the page", c.ID)})
		}
	}
	if err := errors.Join(errs...); err != nil {
		return p, err
	}
	return next, next.Validate()
}

func (s *Service) Profile(ctx context.Context, id string) (profile.Profile, error) {
	db, err := s.openCatalog(ctx)
	if err != nil {
		return profile.Profile{}, err
	}
	defer db.Close()
	return db.Profile(ctx, id)
}

func (s *Service) Profiles(ctx context.Context) ([]profile.Profile, error) {
	db, err := s.openCatalog(ctx)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return db.Profiles(ctx)
}

func (s *Service) SaveProfile(ctx context.Context, p profile.Profile) error {
	db, err := s.openCatalog(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.SaveProfile(ctx, p)
}

func (s *Service) saveProfile(ctx context.Context, db *store.Store, p profile.Profile, progress Progress, refs ...listing.Listing) error {
	if err := db.SaveProfile(ctx, p, refs...); err != nil {
		return err
	}
	s.emit(progress, StageProfile, "saved: profile %s", p.ID)
	return nil
}

func (s *Service) references(p profile.Profile, listings []listing.Listing) ([]profile.ReferenceInput, error) {
	refs := make([]profile.ReferenceInput, len(p.References))
	for i, r := range p.References {
		collages, err := profile.ReadCollages(s.images(), r.Collages)
		if err != nil {
			return nil, err
		}
		refs[i] = profile.ReferenceInput{Listing: listings[i], Collages: collages, Avoid: r.Avoid}
	}
	return refs, nil
}

func (s *Service) draft(ctx context.Context, p *profile.Profile, model string, listings []listing.Listing, progress Progress) error {
	key, err := s.cfg.Keys.OpenRouter()
	if err != nil {
		return err
	}
	refs, err := s.references(*p, listings)
	if err != nil {
		return err
	}

	drafter := profile.Drafter{Client: s.cfg.Clients.OpenRouter(key), Model: cmp.Or(model, profile.DefaultDraftModel)}
	draft, meta, err := drafter.Draft(ctx, cmp.Or(p.Kind, profile.KindWant), p.Notes, refs)
	if err != nil {
		return err
	}
	if err := p.Apply(draft, meta); err != nil {
		return err
	}
	s.emit(progress, StageProfile, "drafted: %d want, %d avoid with %s ($%.4f)", len(p.Want), len(p.Avoid), meta.Model, meta.CostUSD)
	return nil
}
