package service

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"git.kwkaiser.io/kwkaiser/housing/internal/media"
	"git.kwkaiser.io/kwkaiser/housing/internal/telemetry"
)

const (
	DefaultMediaRetention = 30 * 24 * time.Hour
	MediaPruneGrace       = 24 * time.Hour
)

type PruneOptions struct {
	Retention time.Duration
	DryRun    bool
}

type PruneResult struct {
	media.SweepResult
	Since  time.Time
	DryRun bool
}

func (s *Service) PruneMedia(ctx context.Context, o PruneOptions, log *slog.Logger) (res PruneResult, err error) {
	ctx, span := telemetry.Start(ctx, "service.PruneMedia", attribute.Bool("dry_run", o.DryRun))
	defer func() { telemetry.End(span, err) }()
	log = s.logger(log).With("stage", StagePrune)
	unlock, err := s.runLock()
	if err != nil {
		return res, err
	}
	defer unlock()

	retention := o.Retention
	if retention <= 0 {
		retention = DefaultMediaRetention
	}
	now := s.cfg.Now()
	res.Since, res.DryRun = now.Add(-retention), o.DryRun
	refs, err := s.db.MediaRefs(ctx, res.Since)
	if err != nil {
		return res, err
	}
	keep := media.Keep{}
	for _, r := range refs {
		keep.Add(r.Photos, r.Collages)
	}
	log.InfoContext(ctx, "marked media in use", "since", res.Since, "versions", len(refs), "keys", len(keep))
	res.SweepResult, err = media.Sweep(ctx, s.cfg.DataDir, keep, media.SweepOptions{Before: now.Add(-MediaPruneGrace), DryRun: o.DryRun})
	if err != nil {
		return res, err
	}
	log.InfoContext(ctx, "pruned media", "dry_run", o.DryRun, "scanned", res.Scanned, "kept", res.Kept, "recent", res.Recent,
		"deleted", res.Deleted, "bytes", res.Bytes)
	return res, nil
}

func (s *Service) StoredCollages(ctx context.Context, keys []string) []string {
	images := s.images()
	var out []string
	for _, k := range keys {
		if ok, err := images.Has(ctx, k); err == nil && ok {
			out = append(out, k)
		}
	}
	return out
}
