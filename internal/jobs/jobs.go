package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

type Kind string

const (
	KindRunCollection Kind = "run_collection"
	KindCreateProfile Kind = "create_profile"
	KindDraftProfile  Kind = "draft_profile"
)

type Trigger string

const (
	TriggerManual   Trigger = "manual"
	TriggerSchedule Trigger = "schedule"
)

type (
	Job    = store.Job
	Event  = store.JobEvent
	Filter = store.JobFilter
	Status = store.JobStatus
)

const (
	StatusQueued    = store.JobQueued
	StatusRunning   = store.JobRunning
	StatusSucceeded = store.JobSucceeded
	StatusFailed    = store.JobFailed
	StatusCancelled = store.JobCancelled
)

var ErrNotFound = store.ErrJobNotFound

const Interrupted = "interrupted"

type Params interface {
	Kind() Kind
}

type RunCollectionParams struct {
	CollectionID string `json:"collection_id"`
}

func (RunCollectionParams) Kind() Kind { return KindRunCollection }

type CreateProfileParams struct {
	URL          string       `json:"url"`
	ID           string       `json:"id"`
	ProfileKind  profile.Kind `json:"kind,omitempty"`
	Name         string       `json:"name,omitempty"`
	Notes        []string     `json:"notes,omitempty"`
	Model        string       `json:"model,omitempty"`
	NoDraft      bool         `json:"no_draft,omitempty"`
	Force        bool         `json:"force,omitempty"`
	MaxChargeUSD float64      `json:"max_charge_usd,omitempty"`
}

func (CreateProfileParams) Kind() Kind { return KindCreateProfile }

func (p CreateProfileParams) options() service.CreateProfileOptions {
	return service.CreateProfileOptions{
		URL:          p.URL,
		ID:           p.ID,
		Kind:         p.ProfileKind,
		Name:         p.Name,
		Notes:        p.Notes,
		Model:        p.Model,
		NoDraft:      p.NoDraft,
		Force:        p.Force,
		MaxChargeUSD: p.MaxChargeUSD,
	}
}

type DraftProfileParams struct {
	ProfileID string   `json:"profile_id"`
	Model     string   `json:"model,omitempty"`
	Notes     []string `json:"notes"`
}

func (DraftProfileParams) Kind() Kind { return KindDraftProfile }

type RunResult struct {
	Collection      string             `json:"collection"`
	Mode            profile.Mode       `json:"mode"`
	Day             string             `json:"day,omitempty"`
	Fetched         int                `json:"fetched"`
	Distinct        int                `json:"distinct"`
	Duplicates      int                `json:"duplicates"`
	Collaged        int                `json:"collaged"`
	Collages        int                `json:"collages"`
	WithoutCollages int                `json:"without_collages"`
	Stats           profile.BatchStats `json:"stats"`
	BudgetReached   bool               `json:"budget_reached"`
	AssessError     string             `json:"assess_error,omitempty"`
}

func runResult(r service.RunResult) RunResult {
	out := RunResult{
		Collection:      r.Collection,
		Mode:            r.Mode,
		Day:             r.Day,
		Fetched:         r.Fetched,
		Distinct:        r.Distinct,
		Duplicates:      r.Duplicates,
		Collaged:        r.Collaged,
		Collages:        r.Collages,
		WithoutCollages: r.WithoutCollages,
		Stats:           r.Stats,
		BudgetReached:   r.BudgetReached,
	}
	if r.AssessErr != nil {
		out.AssessError = r.AssessErr.Error()
	}
	return out
}

type ProfileResult struct {
	ProfileID string `json:"profile_id"`
	Name      string `json:"name,omitempty"`
	Want      int    `json:"want"`
	Avoid     int    `json:"avoid"`
}

type Executor interface {
	RunCollection(ctx context.Context, collectionID string, progress service.Progress) (service.RunResult, error)
	CreateProfileFromURL(ctx context.Context, o service.CreateProfileOptions, progress service.Progress) (profile.Profile, error)
	DraftProfile(ctx context.Context, id string, o service.DraftOptions, progress service.Progress) (profile.Profile, error)
}

type Queue struct {
	db   *store.Store
	now  func() time.Time
	wake chan struct{}
}

func New(db *store.Store, now func() time.Time) *Queue {
	if now == nil {
		now = time.Now
	}
	return &Queue{db: db, now: now, wake: make(chan struct{}, 1)}
}

func (q *Queue) Enqueue(ctx context.Context, p Params) (int64, error) {
	id, _, err := q.enqueue(ctx, p, TriggerManual, time.Time{})
	return id, err
}

func (q *Queue) enqueue(ctx context.Context, p Params, trigger Trigger, noneSince time.Time) (int64, bool, error) {
	j := Job{Kind: string(p.Kind()), Trigger: string(trigger), CreatedAt: q.now()}
	var rule store.EnqueueRule
	switch p := p.(type) {
	case RunCollectionParams:
		if p.CollectionID == "" {
			return 0, false, fmt.Errorf("run_collection: a collection id is required")
		}
		j.CollectionID = p.CollectionID
		rule = store.EnqueueRule{UniqueActive: true, NoneSince: noneSince}
	case CreateProfileParams:
		if err := profile.ValidID(p.ID); err != nil {
			return 0, false, err
		}
		j.ProfileID = p.ID
	case DraftProfileParams:
		if p.ProfileID == "" {
			return 0, false, fmt.Errorf("draft_profile: a profile id is required")
		}
		j.ProfileID = p.ProfileID
	default:
		return 0, false, fmt.Errorf("unsupported job kind %q", p.Kind())
	}
	b, err := json.Marshal(p)
	if err != nil {
		return 0, false, err
	}
	j.Params = b
	id, created, err := q.db.EnqueueJob(ctx, j, rule)
	if err != nil {
		return 0, false, err
	}
	if created {
		select {
		case q.wake <- struct{}{}:
		default:
		}
	}
	return id, created, nil
}

func (q *Queue) Job(ctx context.Context, id int64) (Job, error) {
	return q.db.Job(ctx, id)
}

func (q *Queue) Jobs(ctx context.Context, f Filter) ([]Job, error) {
	return q.db.Jobs(ctx, f)
}

func (q *Queue) Events(ctx context.Context, id int64) ([]Event, error) {
	return q.db.JobEvents(ctx, id)
}

func (q *Queue) ActiveForCollection(ctx context.Context, collectionID string) (Job, bool, error) {
	return q.db.ActiveJob(ctx, string(KindRunCollection), collectionID)
}

func (q *Queue) LatestForCollection(ctx context.Context, collectionID string) (Job, bool, error) {
	return q.db.LatestJob(ctx, string(KindRunCollection), collectionID)
}
