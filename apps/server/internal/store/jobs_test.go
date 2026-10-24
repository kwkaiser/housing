package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func TestJobLifecycle(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	at := time.Date(2026, 9, 30, 7, 0, 0, 0, time.UTC)
	run := Job{Kind: "run_collection", Trigger: "manual", CollectionID: "somerville", Params: []byte(`{"collection_id":"somerville"}`), CreatedAt: at}

	id, created, err := s.EnqueueJob(ctx, run, EnqueueRule{UniqueActive: true})
	if err != nil || !created {
		t.Fatalf("enqueue: %d %v %v", id, created, err)
	}
	if again, created, err := s.EnqueueJob(ctx, run, EnqueueRule{UniqueActive: true}); err != nil || created || again != id {
		t.Errorf("an active job should be reused: %d %v %v", again, created, err)
	}
	draft, _, err := s.EnqueueJob(ctx, Job{Kind: "draft_profile", Trigger: "manual", ProfileID: "attic", CreatedAt: at}, EnqueueRule{})
	if err != nil {
		t.Fatal(err)
	}
	if j, ok, err := s.ActiveJob(ctx, "run_collection", "somerville"); err != nil || !ok || j.ID != id || j.Status != JobQueued {
		t.Errorf("active = %+v %v %v", j, ok, err)
	}

	j, ok, err := s.ClaimJob(ctx, at.Add(time.Second))
	if err != nil || !ok || j.ID != id || j.Status != JobRunning || !j.StartedAt.Equal(at.Add(time.Second)) || string(j.Params) != `{"collection_id":"somerville"}` {
		t.Fatalf("claim = %+v %v %v", j, ok, err)
	}
	for i, msg := range []string{"one", "two"} {
		if err := s.AppendJobEvent(ctx, JobEvent{JobID: id, At: at, Level: slog.Level(4 * i), Message: msg, Record: json.RawMessage(`{"msg":"` + msg + `","stage":"fetch"}`)}); err != nil {
			t.Fatal(err)
		}
	}
	j.Status, j.FinishedAt, j.CostUSD, j.Result = JobSucceeded, at.Add(time.Minute), 0.25, []byte(`{"fetched":3}`)
	if err := s.FinishJob(ctx, j); err != nil {
		t.Fatal(err)
	}
	got, err := s.Job(ctx, id)
	if err != nil || got.Status != JobSucceeded || got.CostUSD != 0.25 || string(got.Result) != `{"fetched":3}` || !got.FinishedAt.Equal(at.Add(time.Minute)) {
		t.Errorf("finished = %+v %v", got, err)
	}
	events, err := s.JobEvents(ctx, id)
	if err != nil || len(events) != 2 || events[0].Seq != 1 || events[1].Seq != 2 || events[1].Message != "two" || events[0].Level != slog.LevelInfo || events[1].Level != slog.LevelWarn || string(events[1].Record) != `{"msg":"two","stage":"fetch"}` {
		t.Errorf("events = %+v %v", events, err)
	}
	if _, ok, _ := s.ActiveJob(ctx, "run_collection", "somerville"); ok {
		t.Error("a finished job should not be active")
	}
	if next, created, _ := s.EnqueueJob(ctx, run, EnqueueRule{UniqueActive: true}); !created || next == id {
		t.Error("a finished job should not block a new one")
	}

	if all, err := s.Jobs(ctx, JobFilter{}); err != nil || len(all) != 3 || all[0].ID < all[2].ID {
		t.Errorf("jobs = %d %v", len(all), err)
	}
	if mine, _ := s.Jobs(ctx, JobFilter{ProfileID: "attic"}); len(mine) != 1 || mine[0].ID != draft {
		t.Errorf("profile filter = %+v", mine)
	}
	if queued, _ := s.Jobs(ctx, JobFilter{Status: JobQueued}); len(queued) != 2 || queued[0].Status != JobQueued || queued[1].Status != JobQueued {
		t.Errorf("status filter = %+v", queued)
	}
	if done, _ := s.Jobs(ctx, JobFilter{CollectionID: "somerville", Status: JobSucceeded}); len(done) != 1 || done[0].ID != id {
		t.Errorf("collection and status filter = %+v", done)
	}
	if one, _ := s.Jobs(ctx, JobFilter{CollectionID: "somerville", Limit: 1}); len(one) != 1 {
		t.Errorf("limit = %d", len(one))
	}
	if latest, ok, _ := s.LatestJob(ctx, "run_collection", "somerville"); !ok || latest.Status != JobQueued {
		t.Errorf("latest = %+v", latest)
	}
	if _, err := s.Job(ctx, 999); !errors.Is(err, ErrJobNotFound) {
		t.Errorf("missing job: %v", err)
	}
	j.Status = JobRunning
	if err := s.FinishJob(ctx, j); err == nil {
		t.Error("finishing as running should fail")
	}
}

func TestEnqueueNoneSince(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	day := time.Date(2026, 9, 30, 0, 0, 0, 0, time.Local)
	j := Job{Kind: "run_collection", Trigger: "schedule", CollectionID: "c", CreatedAt: day.Add(-time.Hour)}
	first, created, err := s.EnqueueJob(ctx, j, EnqueueRule{NoneSince: day.Add(-24 * time.Hour)})
	if err != nil || !created {
		t.Fatal(err)
	}
	claimed, _, _ := s.ClaimJob(ctx, day)
	claimed.Status, claimed.FinishedAt = JobFailed, day
	s.FinishJob(ctx, claimed)

	j.CreatedAt = day.Add(7 * time.Hour)
	second, created, err := s.EnqueueJob(ctx, j, EnqueueRule{UniqueActive: true, NoneSince: day})
	if err != nil || !created || second == first {
		t.Fatalf("yesterday's job should not block today: %d %v %v", second, created, err)
	}
	claimed, _, _ = s.ClaimJob(ctx, day)
	claimed.Status, claimed.FinishedAt = JobSucceeded, day
	s.FinishJob(ctx, claimed)
	j.CreatedAt = day.Add(8 * time.Hour)
	if id, created, err := s.EnqueueJob(ctx, j, EnqueueRule{UniqueActive: true, NoneSince: day}); err != nil || created || id != second {
		t.Errorf("a job created today should block another: %d %v %v", id, created, err)
	}
}

func TestFailRunningJobs(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	at := time.Date(2026, 9, 30, 7, 0, 0, 0, time.UTC)
	for _, c := range []string{"a", "b"} {
		if _, _, err := s.EnqueueJob(ctx, Job{Kind: "run_collection", Trigger: "manual", CollectionID: c, CreatedAt: at}, EnqueueRule{}); err != nil {
			t.Fatal(err)
		}
	}
	running, _, _ := s.ClaimJob(ctx, at)
	if n, err := s.FailRunningJobs(ctx, "interrupted", at.Add(time.Hour)); err != nil || n != 1 {
		t.Fatalf("failed %d %v", n, err)
	}
	if j, _ := s.Job(ctx, running.ID); j.Status != JobFailed || j.Error != "interrupted" || j.FinishedAt.IsZero() {
		t.Errorf("stale job = %+v", j)
	}
	if queued, _, _ := s.ActiveJob(ctx, "run_collection", "b"); queued.Status != JobQueued {
		t.Errorf("queued jobs should be left alone: %+v", queued)
	}
}

func TestMigrateJobsOverExistingDatabase(t *testing.T) {
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), FileName)
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	ms, err := migrations()
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range ms[:4] {
		if err := apply(ctx, db, m); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{
		`INSERT INTO profiles (id, kind, name, summary, notes, want, avoid, ignore, searches, updated_at) VALUES ('attic', 'want', '', '', '[]', '[]', '[]', '[]', '{}', '2026-09-30T00:00:00Z')`,
		`INSERT INTO collections (id, mode, sources, search, model, max_run_cost_usd, updated_at) VALUES ('somerville', 'rent', '["zillow"]', '{"location":"Somerville, MA","limit":5}', '', 2, '2026-09-30T00:00:00Z')`,
		`INSERT INTO collection_profiles (collection_id, position, profile_id) VALUES ('somerville', 0, 'attic')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()

	s, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if v, _ := s.SchemaVersion(ctx); v != len(ms) {
		t.Errorf("version = %d", v)
	}
	c, err := s.Collection(ctx, "somerville")
	if err != nil || c.Schedule != "" || c.MaxRunCostUSD != 2 {
		t.Fatalf("existing collection = %+v %v", c, err)
	}
	c.Schedule = "07:30"
	if err := s.SaveCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	if got, _ := s.Collection(ctx, "somerville"); got.Schedule != "07:30" {
		t.Errorf("schedule = %q", got.Schedule)
	}
	c.Schedule = ""
	s.SaveCollection(ctx, c)
	if got, _ := s.Collection(ctx, "somerville"); got.Schedule != "" {
		t.Errorf("cleared schedule = %q", got.Schedule)
	}
	if _, _, err := s.EnqueueJob(ctx, Job{Kind: "run_collection", Trigger: "manual", CollectionID: "somerville", CreatedAt: time.Now()}, EnqueueRule{}); err != nil {
		t.Error(err)
	}
	bad := collection.Collection{ID: "x", Mode: profile.ModeRent, Sources: []listing.Source{listing.SourceZillow}, Profiles: []string{"attic"}, Search: profile.Search{Location: "x", Limit: 1}, Schedule: "7am"}
	if err := s.SaveCollection(ctx, bad); err == nil {
		t.Error("an invalid schedule should be rejected")
	}
}
