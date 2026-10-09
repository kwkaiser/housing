package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

type fakeExec struct {
	mu        sync.Mutex
	runs      []string
	started   chan string
	block     bool
	err       error
	assessErr error
}

func (f *fakeExec) RunCollection(ctx context.Context, id string, log *slog.Logger) (service.RunResult, error) {
	f.mu.Lock()
	f.runs = append(f.runs, id)
	f.mu.Unlock()
	log.Info("fetched "+id, "stage", service.StageFetch, "listings", 3)
	if f.started != nil {
		f.started <- id
	}
	if f.block {
		<-ctx.Done()
		log.Warn("stopping", "stage", service.StageRun)
		return service.RunResult{Collection: id}, ctx.Err()
	}
	res := service.RunResult{Collection: id, Mode: profile.ModeRent, Fetched: 3, Stats: profile.BatchStats{Calls: 2, CostUSD: 0.04}}
	res.AssessErr = f.assessErr
	if f.err != nil {
		return res, f.err
	}
	log.Info("graded", "stage", service.StageAssess)
	return res, nil
}

func (f *fakeExec) CreateProfileFromURL(_ context.Context, o service.CreateProfileOptions, log *slog.Logger) (profile.Profile, error) {
	log.Info("reference "+o.URL, "stage", service.StageProfile)
	return profile.Profile{ID: o.ID, Name: o.Name, Want: make([]profile.Criterion, 2), Drafted: &profile.Drafted{CostUSD: 0.01}}, nil
}

func (f *fakeExec) DraftProfile(context.Context, string, service.DraftOptions, *slog.Logger) (profile.Profile, error) {
	return profile.Profile{}, errors.New("no references")
}

func openStore(t *testing.T) *store.Store {
	t.Helper()
	db, err := store.Open(t.Context(), filepath.Join(t.TempDir(), store.FileName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func seed(t *testing.T, db *store.Store, schedules map[string]string) {
	t.Helper()
	ctx := t.Context()
	if err := db.SaveProfile(ctx, profile.Profile{ID: "attic", Ignore: profile.DefaultIgnore}); err != nil {
		t.Fatal(err)
	}
	for id, schedule := range schedules {
		c := collection.Collection{ID: id, Mode: profile.ModeRent, Sources: []listing.Source{listing.SourceZillow}, Profiles: []string{"attic"},
			Search: profile.Search{Location: "Somerville, MA", Limit: 5}, Schedule: schedule}
		if err := db.SaveCollection(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
}

func start(t *testing.T, q *Queue, exec Executor) (context.CancelFunc, chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	r := &Runner{Queue: q, Exec: exec, Poll: 10 * time.Millisecond}
	go func() { done <- r.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})
	return cancel, done
}

func wait(t *testing.T, q *Queue, id int64) Job {
	t.Helper()
	synctest.Wait()
	j, err := q.Job(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if j.Status.Active() {
		t.Fatalf("job %d is still %s", id, j.Status)
	}
	return j
}

func TestRunnerRecordsJobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		q := New(openStore(t), nil)
		exec := &fakeExec{}
		start(t, q, exec)

		id, err := q.Enqueue(ctx, RunCollectionParams{CollectionID: "somerville"})
		if err != nil {
			t.Fatal(err)
		}
		j := wait(t, q, id)
		var res RunResult
		if err := json.Unmarshal(j.Result, &res); err != nil {
			t.Fatal(err)
		}
		if j.Status != StatusSucceeded || j.CostUSD != 0.04 || j.Error != "" || j.Trigger != string(TriggerManual) || res.Fetched != 3 || res.Stats.Calls != 2 || j.StartedAt.IsZero() || j.FinishedAt.IsZero() {
			t.Errorf("job = %+v result = %+v", j, res)
		}
		events, err := q.Events(ctx, id)
		if err != nil || len(events) != 2 || events[0].Message != "fetched somerville" || events[1].Message != "graded" {
			t.Fatalf("events = %+v %v", events, err)
		}
		if attrs, err := EventAttrs(events[0]); err != nil || !slices.Equal(attrs, []Attr{{"stage", "fetch"}, {"listings", "3"}}) {
			t.Errorf("attrs = %+v %v", attrs, err)
		}
		if !strings.Contains(string(events[0].Record), `"level":"INFO"`) {
			t.Errorf("record %s should carry the level", events[0].Record)
		}
		if latest, ok, _ := q.LatestForCollection(ctx, "somerville"); !ok || latest.ID != id {
			t.Errorf("latest = %+v", latest)
		}

		pid, err := q.Enqueue(ctx, CreateProfileParams{URL: "https://zillow.com/x", ID: "loft", Name: "Loft"})
		if err != nil {
			t.Fatal(err)
		}
		p := wait(t, q, pid)
		if p.Status != StatusSucceeded || p.ProfileID != "loft" || p.CostUSD != 0.01 || string(p.Result) != `{"profile_id":"loft","name":"Loft","want":2,"avoid":0}` {
			t.Errorf("create profile job = %+v %s", p, p.Result)
		}

		did, _ := q.Enqueue(ctx, DraftProfileParams{ProfileID: "loft"})
		if d := wait(t, q, did); d.Status != StatusFailed || d.Error != "no references" || d.Result != nil {
			t.Errorf("draft job = %+v", d)
		}
		if _, err := q.Enqueue(ctx, CreateProfileParams{ID: "Bad Id"}); err == nil {
			t.Error("an invalid profile id should be rejected")
		}
	})
}

func TestRunnerRecordsFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := New(openStore(t), nil)
		start(t, q, &fakeExec{err: errors.New("OPENROUTER_API_KEY is not set")})
		id, _ := q.Enqueue(t.Context(), RunCollectionParams{CollectionID: "c"})
		j := wait(t, q, id)
		if j.Status != StatusFailed || j.Error != "OPENROUTER_API_KEY is not set" || j.CostUSD != 0.04 || j.Result != nil {
			t.Errorf("job = %+v", j)
		}
	})
}

func TestRunnerRecordsDegraded(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		q := New(openStore(t), nil)
		start(t, q, &fakeExec{assessErr: errors.New("openrouter: 402 out of credits")})
		id, _ := q.Enqueue(t.Context(), RunCollectionParams{CollectionID: "c"})
		j := wait(t, q, id)
		var res RunResult
		if err := json.Unmarshal(j.Result, &res); err != nil {
			t.Fatal(err)
		}
		if j.Status != StatusDegraded || j.Error != "assess: openrouter: 402 out of credits" || res.AssessError != "openrouter: 402 out of credits" || res.Fetched != 3 {
			t.Errorf("job = %+v result = %+v", j, res)
		}
	})
}

func TestEnqueueReusesActiveRun(t *testing.T) {
	ctx := t.Context()
	q := New(openStore(t), nil)
	first, err := q.Enqueue(ctx, RunCollectionParams{CollectionID: "c"})
	if err != nil {
		t.Fatal(err)
	}
	if again, err := q.Enqueue(ctx, RunCollectionParams{CollectionID: "c"}); err != nil || again != first {
		t.Errorf("duplicate enqueue = %d %v, want %d", again, err, first)
	}
	if other, _ := q.Enqueue(ctx, RunCollectionParams{CollectionID: "d"}); other == first {
		t.Error("another collection should get its own job")
	}
	if active, ok, _ := q.ActiveForCollection(ctx, "c"); !ok || active.ID != first {
		t.Errorf("active = %+v", active)
	}
	if _, err := q.Enqueue(ctx, RunCollectionParams{}); err == nil {
		t.Error("a run without a collection should be rejected")
	}
}

func TestRunnerRecoversInterruptedJobs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		db := openStore(t)
		q := New(db, nil)
		stale, _ := q.Enqueue(ctx, RunCollectionParams{CollectionID: "c"})
		if _, _, err := db.ClaimJob(ctx, time.Now()); err != nil {
			t.Fatal(err)
		}
		queued, _ := q.Enqueue(ctx, RunCollectionParams{CollectionID: "d"})

		start(t, q, &fakeExec{})
		if j := wait(t, q, stale); j.Status != StatusFailed || j.Error != Interrupted {
			t.Errorf("stale job = %+v", j)
		}
		if j := wait(t, q, queued); j.Status != StatusSucceeded {
			t.Errorf("queued job should still run: %+v", j)
		}
	})
}

func TestRunnerShutdownCancelsJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		q := New(openStore(t), nil)
		exec := &fakeExec{block: true, started: make(chan string, 1)}
		cancel, done := start(t, q, exec)
		id, _ := q.Enqueue(ctx, RunCollectionParams{CollectionID: "c"})
		<-exec.started
		cancel()
		err := <-done
		done <- err
		if err != nil {
			t.Errorf("runner: %v", err)
		}
		j, err := q.Job(ctx, id)
		if err != nil || j.Status != StatusCancelled || j.Error != context.Canceled.Error() || j.FinishedAt.IsZero() {
			t.Errorf("job = %+v %v", j, err)
		}
		if events, _ := q.Events(ctx, id); len(events) != 2 || events[1].Message != "stopping" || events[1].Level != slog.LevelWarn {
			t.Errorf("events after cancel should still be recorded: %+v", events)
		}
	})
}

func TestScheduler(t *testing.T) {
	ctx := t.Context()
	db := openStore(t)
	seed(t, db, map[string]string{"morning": "07:00", "evening": "19:30", "manual": ""})
	clk := &clock{now: time.Date(2026, 9, 30, 6, 59, 0, 0, time.Local)}
	q := New(db, clk.Now)
	sched := &Scheduler{Queue: q, Now: clk.Now}

	check := func(want ...string) {
		t.Helper()
		ids, err := sched.Check(ctx)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, id := range ids {
			j, _ := q.Job(ctx, id)
			if j.Trigger != string(TriggerSchedule) || j.Kind != string(KindRunCollection) {
				t.Errorf("scheduled job = %+v", j)
			}
			got = append(got, j.CollectionID)
		}
		if len(got) != len(want) || len(got) > 0 && got[0] != want[0] {
			t.Errorf("at %s enqueued %v, want %v", clk.Now().Format(time.Kitchen), got, want)
		}
	}

	check()
	clk.Set(time.Date(2026, 9, 30, 7, 0, 0, 0, time.Local))
	check("morning")
	clk.Set(time.Date(2026, 9, 30, 7, 1, 0, 0, time.Local))
	check()

	j, _, _ := q.ActiveForCollection(ctx, "morning")
	claimed, _, _ := db.ClaimJob(ctx, clk.Now())
	claimed.Status, claimed.FinishedAt = StatusSucceeded, clk.Now()
	db.FinishJob(ctx, claimed)
	if claimed.ID != j.ID {
		t.Fatalf("claimed %d, want %d", claimed.ID, j.ID)
	}
	clk.Set(time.Date(2026, 9, 30, 12, 0, 0, 0, time.Local))
	restarted := &Scheduler{Queue: New(db, clk.Now), Now: clk.Now}
	if ids, _ := restarted.Check(ctx); len(ids) != 0 {
		t.Errorf("a restart should not enqueue a run that already happened today: %v", ids)
	}

	clk.Set(time.Date(2026, 9, 30, 21, 0, 0, 0, time.Local))
	check("evening")
	clk.Set(time.Date(2026, 10, 1, 7, 5, 0, 0, time.Local))
	check("morning")
	if all, _ := q.Jobs(ctx, Filter{CollectionID: "manual"}); len(all) != 0 {
		t.Errorf("unscheduled collections should never be enqueued: %v", all)
	}
}

func TestSchedulerCatchesUpOnStartup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := t.Context()
		db := openStore(t)
		seed(t, db, map[string]string{"morning": "07:00", "manual-today": "08:00"})
		clk := &clock{now: time.Date(2026, 9, 30, 9, 0, 0, 0, time.Local)}
		q := New(db, clk.Now)

		clk.Set(time.Date(2026, 9, 30, 7, 30, 0, 0, time.Local))
		if _, err := q.Enqueue(ctx, RunCollectionParams{CollectionID: "manual-today"}); err != nil {
			t.Fatal(err)
		}
		claimed, _, _ := db.ClaimJob(ctx, clk.Now())
		claimed.Status, claimed.FinishedAt = StatusFailed, clk.Now()
		db.FinishJob(ctx, claimed)

		clk.Set(time.Date(2026, 9, 30, 9, 0, 0, 0, time.Local))
		sched := &Scheduler{Queue: q, Now: clk.Now, Tick: time.Hour}
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- sched.Run(runCtx) }()
		synctest.Wait()
		if _, ok, _ := q.ActiveForCollection(ctx, "morning"); !ok {
			t.Fatal("a missed schedule should be enqueued on startup")
		}
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
		if js, _ := q.Jobs(ctx, Filter{CollectionID: "manual-today"}); len(js) != 1 {
			t.Errorf("a manual run earlier today should count as today's run: %d jobs", len(js))
		}
	})
}

func TestDraftParamsNotes(t *testing.T) {
	for _, tc := range []struct {
		notes []string
		json  string
	}{
		{nil, `{"profile_id":"attic","notes":null}`},
		{[]string{}, `{"profile_id":"attic","notes":[]}`},
		{[]string{"quiet"}, `{"profile_id":"attic","notes":["quiet"]}`},
	} {
		b, err := json.Marshal(DraftProfileParams{ProfileID: "attic", Notes: tc.notes})
		if err != nil || string(b) != tc.json {
			t.Errorf("marshal %#v = %s %v", tc.notes, b, err)
		}
		var back DraftProfileParams
		if err := json.Unmarshal(b, &back); err != nil || (back.Notes == nil) != (tc.notes == nil) || len(back.Notes) != len(tc.notes) {
			t.Errorf("round trip %#v = %#v %v", tc.notes, back.Notes, err)
		}
	}
}
