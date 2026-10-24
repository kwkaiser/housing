package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/app/view"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/notify"
	"git.kwkaiser.io/kwkaiser/housing/internal/report"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

func (s *Service) notify(ctx context.Context, o RunOptions, day string, log *slog.Logger) (int, error) {
	n := o.Notify
	if n.URL == "" || day == "" {
		return 0, nil
	}
	log = s.logger(log).With("stage", StageNotify)
	collectionID := o.Fetch.Collection
	db, err := s.catalog(ctx)
	if err != nil {
		return 0, err
	}
	templates, err := s.templates(ctx, db, o.Assess.ProfileIDs)
	if err != nil {
		return 0, err
	}
	listings, err := db.LoadDay(ctx, day, collectionID)
	if err != nil {
		return 0, err
	}
	r := report.Build(reportTemplates(templates), listings, report.Options{
		Offer: o.Fetch.Mode.Offer(), Model: o.Assess.Model, MinScore: n.Threshold(),
	})
	sent, err := db.Notified(ctx, collectionID)
	if err != nil {
		return 0, err
	}
	notifier := s.cfg.Clients.Notifier(n.URL)
	var (
		count int
		errs  []error
	)
	for _, row := range r.Rows {
		members := rowListings(row)
		if slices.ContainsFunc(members, func(l listing.Listing) bool { return sent[store.Key(l)] }) {
			continue
		}
		name := report.AddressLine(row.Listing)
		if err := notifier.Notify(ctx, message(collectionID, row)); err != nil {
			log.Warn("notification failed", "listing", name, "match", row.Match, "err", err)
			errs = append(errs, fmt.Errorf("%s: %w", name, err))
			if ctx.Err() != nil {
				break
			}
			continue
		}
		if err := db.RecordNotification(context.WithoutCancel(ctx), collectionID, members, row.Match, s.cfg.Now()); err != nil {
			return count, err
		}
		count++
		log.Info("notified", "listing", name, "match", row.Match)
	}
	log.Info("notifications sent", "sent", count, "min_score", n.Threshold(), "failed", len(errs))
	return count, errors.Join(errs...)
}

func rowListings(row report.Row) []listing.Listing {
	out := []listing.Listing{row.Listing}
	for _, also := range row.AlsoListed {
		out = append(out, listing.Listing{Source: also.Source, SourceID: also.SourceID})
	}
	return out
}

func message(collectionID string, row report.Row) notify.Message {
	l := row.Listing
	details := []string{view.Money(l.Price.Cents, l.Offer)}
	if l.Beds != nil {
		details = append(details, view.Beds(l.Beds)+" bd")
	}
	if l.Baths != nil {
		details = append(details, view.Baths(l.Baths)+" ba")
	}
	body := []string{report.AddressLine(l), strings.Join(details, " · ")}
	if row.Summary != "" {
		body = append(body, row.Summary)
	}
	m := notify.Message{
		Title: fmt.Sprintf("%s match in %s", view.Match(row.Match, row.Calibrated, false), collectionID),
		Body:  strings.Join(body, "\n"),
		Click: l.URL,
		Tags:  []string{"house"},
	}
	if len(l.Photos) > 0 {
		m.Attach = l.Photos[0]
	}
	return m
}
