package jobs

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

const DefaultPoll = 5 * time.Second

type Runner struct {
	Queue *Queue
	Exec  Executor
	Log   *slog.Logger
	Poll  time.Duration
}

func (r *Runner) Run(ctx context.Context) error {
	log := r.logger()
	n, err := r.Queue.db.FailRunningJobs(ctx, Interrupted, r.Queue.now())
	if err != nil {
		return fmt.Errorf("recover interrupted jobs: %w", err)
	}
	if n > 0 {
		log.Warn("marked interrupted jobs failed", "jobs", n)
	}
	poll := r.Poll
	if poll <= 0 {
		poll = DefaultPoll
	}
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		for ctx.Err() == nil {
			ran, err := r.next(ctx)
			if err != nil {
				if ctx.Err() == nil {
					log.Error("job runner", "err", err)
				}
				break
			}
			if !ran {
				break
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-r.Queue.wake:
		case <-ticker.C:
		}
	}
}

func (r *Runner) next(ctx context.Context) (bool, error) {
	q := r.Queue
	j, ok, err := q.db.ClaimJob(ctx, q.now())
	if err != nil || !ok {
		return false, err
	}
	log := r.logger().With("job", j.ID, "kind", j.Kind)
	log.Info("job started")
	persist := context.WithoutCancel(ctx)
	progress := func(e service.Event) {
		log.Info("job progress", "stage", e.Stage, "msg", e.Message)
		ev := store.JobEvent{JobID: j.ID, At: q.now(), Stage: string(e.Stage), Message: e.Message}
		if err := q.db.AppendJobEvent(persist, ev); err != nil {
			log.Error("record job event", "err", err)
		}
	}

	cost, result, runErr := r.execute(ctx, j, progress)
	j.FinishedAt, j.CostUSD = q.now(), cost
	switch {
	case ctx.Err() != nil:
		j.Status, j.Error = StatusCancelled, cmp.Or(runErr, ctx.Err()).Error()
	case runErr != nil:
		j.Status, j.Error = StatusFailed, runErr.Error()
	default:
		j.Status = StatusSucceeded
	}
	if result != nil {
		if b, err := json.Marshal(result); err == nil {
			j.Result = b
		} else {
			log.Error("encode job result", "err", err)
		}
	}
	if err := q.db.FinishJob(persist, j); err != nil {
		return true, err
	}
	log.Info("job finished", "status", j.Status, "cost_usd", j.CostUSD, "err", j.Error)
	return true, nil
}

func (r *Runner) execute(ctx context.Context, j Job, progress service.Progress) (float64, any, error) {
	switch Kind(j.Kind) {
	case KindRunCollection:
		var p RunCollectionParams
		if err := decode(j, &p); err != nil {
			return 0, nil, err
		}
		res, err := r.Exec.RunCollection(ctx, p.CollectionID, progress)
		if err != nil && res.Day == "" {
			return res.Stats.CostUSD, nil, err
		}
		return res.Stats.CostUSD, runResult(res), err
	case KindCreateProfile:
		var p CreateProfileParams
		if err := decode(j, &p); err != nil {
			return 0, nil, err
		}
		prof, err := r.Exec.CreateProfileFromURL(ctx, p.options(), progress)
		return draftCost(prof), profileResult(prof), err
	case KindDraftProfile:
		var p DraftProfileParams
		if err := decode(j, &p); err != nil {
			return 0, nil, err
		}
		prof, err := r.Exec.DraftProfile(ctx, p.ProfileID, service.DraftOptions{Model: p.Model, Notes: p.Notes}, progress)
		return draftCost(prof), profileResult(prof), err
	default:
		return 0, nil, fmt.Errorf("unsupported job kind %q", j.Kind)
	}
}

func decode(j Job, v any) error {
	if err := json.Unmarshal(j.Params, v); err != nil {
		return fmt.Errorf("job %d params: %w", j.ID, err)
	}
	return nil
}

func draftCost(p profile.Profile) float64 {
	if p.Drafted == nil {
		return 0
	}
	return p.Drafted.CostUSD
}

func profileResult(p profile.Profile) any {
	if p.ID == "" {
		return nil
	}
	return ProfileResult{ProfileID: p.ID, Name: p.Name, Want: len(p.Want), Avoid: len(p.Avoid)}
}

func (r *Runner) logger() *slog.Logger {
	if r.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return r.Log
}
