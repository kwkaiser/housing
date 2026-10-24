package jobs

import (
	"context"
	"log/slog"
	"time"
)

const DefaultTick = time.Minute

type Scheduler struct {
	Queue    *Queue
	Now      func() time.Time
	Location *time.Location
	Tick     time.Duration
	Log      *slog.Logger
}

func (s *Scheduler) Run(ctx context.Context) error {
	tick := s.Tick
	if tick <= 0 {
		tick = DefaultTick
	}
	ticker := time.NewTicker(tick)
	defer ticker.Stop()
	for {
		if _, err := s.Check(ctx); err != nil && ctx.Err() == nil {
			s.logger().Error("scheduler", "err", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

func (s *Scheduler) Check(ctx context.Context) ([]int64, error) {
	cs, err := s.Queue.db.Collections(ctx)
	if err != nil {
		return nil, err
	}
	now := s.now().In(s.location())
	y, m, d := now.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	var ids []int64
	for _, c := range cs {
		at, ok := c.ScheduledAt(today)
		if !ok || now.Before(at) {
			continue
		}
		id, created, err := s.Queue.enqueue(ctx, RunCollectionParams{CollectionID: c.ID}, TriggerSchedule, today)
		if err != nil {
			return ids, err
		}
		if created {
			s.logger().Info("scheduled run enqueued", "collection", c.ID, "job", id, "schedule", c.Schedule)
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (s *Scheduler) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return s.Queue.now()
}

func (s *Scheduler) location() *time.Location {
	if s.Location != nil {
		return s.Location
	}
	return time.Local
}

func (s *Scheduler) logger() *slog.Logger {
	if s.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return s.Log
}
