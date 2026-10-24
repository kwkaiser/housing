package app

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

type fakeExec struct {
	started chan string
	release chan struct{}
}

func (f *fakeExec) RunCollection(ctx context.Context, id string, log *slog.Logger) (service.RunResult, error) {
	log.Info("fetching <b>"+id+"</b>", "stage", service.StageFetch)
	if f.started != nil {
		f.started <- id
		<-f.release
	}
	if id == "zempty" {
		return service.RunResult{Collection: id}, errors.New(`apify: token missing <script>alert("x")</script>`)
	}
	log.Info("graded", "stage", service.StageAssess, "listings", 4, "missing", []string{"skylights", "porch"})
	log.Warn("2 over budget", "stage", service.StageAssess)
	return service.RunResult{
		Collection: id, Mode: profile.ModeRent, Day: day2, Fetched: 6, Distinct: 5, Duplicates: 1, Collaged: 5, Collages: 9, WithoutCollages: 1,
		Stats:         profile.BatchStats{Calls: 4, Updated: 4, Cached: 1, OverBudget: 2, CostUSD: 0.125},
		BudgetReached: true,
	}, nil
}

func (f *fakeExec) CreateProfileFromURL(_ context.Context, o service.CreateProfileOptions, log *slog.Logger) (profile.Profile, error) {
	log.Info("reference "+o.URL, "stage", service.StageProfile)
	return profile.Profile{ID: o.ID, Name: "New place", Want: make([]profile.Criterion, 3), Avoid: make([]profile.Criterion, 1), Drafted: &profile.Drafted{CostUSD: 0.01}}, nil
}

func (f *fakeExec) DraftProfile(context.Context, string, service.DraftOptions, *slog.Logger) (profile.Profile, error) {
	return profile.Profile{}, errors.New("no references")
}

func runJobs(t *testing.T, q *jobs.Queue, exec jobs.Executor, ps ...jobs.Params) []int64 {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- (&jobs.Runner{Queue: q, Exec: exec, Poll: 10 * time.Millisecond}).Run(ctx) }()
	defer func() {
		cancel()
		<-done
	}()
	var ids []int64
	for _, p := range ps {
		id, err := q.Enqueue(ctx, p)
		if err != nil {
			t.Fatal(err)
		}
		waitJob(t, q, id, func(j jobs.Job) bool { return !j.Status.Active() })
		ids = append(ids, id)
	}
	return ids
}

func waitJob(t *testing.T, q *jobs.Queue, id int64, ok func(jobs.Job) bool) jobs.Job {
	t.Helper()
	synctest.Wait()
	j, err := q.Job(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	if !ok(j) {
		t.Fatalf("job %d is unexpectedly %s", id, j.Status)
	}
	return j
}

func post(t *testing.T, h http.Handler, target string, form url.Values, header map[string]string) (*http.Response, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

var (
	sameOriginHeader = map[string]string{"Origin": "http://example.com"}
	jobRow           = regexp.MustCompile(`<td class="num"><a href="/jobs/(\d+)">`)
	eventRow         = regexp.MustCompile(`<td class="message">(?:<span class="level">\w+</span> )?([^<]*)`)
)

func jobIDs(body string) []string {
	var out []string
	for _, m := range jobRow.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

func id(n int64) string {
	return strconv.FormatInt(n, 10)
}

func TestJobPages(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := seededApp(t)
		q := a.jobs.(*jobs.Queue)
		h := a.Handler()

		ids := runJobs(t, q, &fakeExec{},
			jobs.RunCollectionParams{CollectionID: "somerville"},
			jobs.RunCollectionParams{CollectionID: "zempty"},
			jobs.CreateProfileParams{URL: "https://example.com/ref", ID: "newp", Notes: []string{"quiet", "sunny"}, NoDraft: true},
		)
		run, failed, created := ids[0], ids[1], ids[2]
		queued, err := q.Enqueue(t.Context(), jobs.RunCollectionParams{CollectionID: "somerville"})
		if err != nil {
			t.Fatal(err)
		}
		all := []string{id(queued), id(created), id(failed), id(run)}

		for _, tc := range []struct {
			target   string
			ids      []string
			contains []string
		}{
			{"/jobs", all, []string{
				"<title>Jobs · housing</title>", `<a href="/jobs" aria-current="page">Jobs</a>`,
				`<td>Collection run</td>`, `<td>Create profile</td>`,
				`<a href="/c/somerville">somerville</a>`, `<a href="/profiles/newp">newp</a>`,
				`<span class="status succeeded">succeeded</span>`, `<span class="status failed">failed</span>`, `<span class="status queued">queued</span>`,
				`<td class="tags">manual</td>`, `<td class="num">$0.125</td>`,
				`apify: token missing &lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;`,
				`<option value="zempty">zempty</option>`,
			}},
			{"/jobs?status=failed", []string{id(failed)}, []string{`<option value="failed" selected>failed</option>`, `<a class="button" href="/jobs">Clear</a>`}},
			{"/jobs?collection=somerville", []string{id(queued), id(run)}, []string{`<option value="somerville" selected>somerville</option>`}},
			{"/jobs?collection=somerville&status=succeeded", []string{id(run)}, nil},
			{"/jobs?collection=gone", nil, []string{`<option value="gone" selected>gone</option>`, "No jobs match these filters."}},
			{"/jobs?status=bogus&limit=2", all[:2], []string{`<input type="hidden" name="limit" value="2">`}},
		} {
			res, body := get(t, h, tc.target)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("GET %s = %d\n%s", tc.target, res.StatusCode, body)
			}
			if got := jobIDs(body); strings.Join(got, ",") != strings.Join(tc.ids, ",") {
				t.Errorf("GET %s ids = %v, want %v", tc.target, got, tc.ids)
			}
			for _, s := range tc.contains {
				if !strings.Contains(body, s) {
					t.Errorf("GET %s: missing %q", tc.target, s)
				}
			}
			if strings.Contains(body, "<script>") {
				t.Errorf("GET %s: unescaped markup", tc.target)
			}
		}

		for _, tc := range []struct {
			job      int64
			refresh  bool
			contains []string
		}{
			{run, false, []string{
				"<title>Job " + id(run) + " · housing</title>",
				`<span class="status succeeded">succeeded</span>`,
				`<dt>collection</dt><dd><a href="/c/somerville">somerville</a></dd>`,
				`<dt>Day</dt><dd><a href="/c/somerville?day=2026-09-30">2026-09-30</a></dd>`,
				`<dt>Fetched</dt><dd>6</dd>`, `<dt>Distinct</dt><dd>5</dd>`, `<dt>Duplicates</dt><dd>1</dd>`,
				`<dt>Collaged</dt><dd>5 listings, 9 collages</dd>`, `<dt>Graded</dt><dd>4</dd>`, `<dt>Failed</dt><dd>0</dd>`,
				`<dt>Over budget</dt><dd>2</dd>`, `<dt>Budget reached</dt><dd>yes</dd>`, `<dt>Cost</dt><dd>$0.125</dd>`,
				`<tr class="info"><td class="message">graded<div class="attrs"><span><span class="key">listings</span> 4</span><span><span class="key">missing</span> skylights, porch</span></div></td><td class="meta">assess · `,
				`<tr class="warn"><td class="message"><span class="level">WARN</span> 2 over budget</td>`,
			}},
			{failed, false, []string{
				`<span class="status failed">failed</span>`,
				`<pre class="error">apify: token missing &lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;</pre>`,
			}},
			{created, false, []string{
				`<dt>Profile</dt><dd><a href="/profiles/newp">New place</a> <span class="muted">newp</span></dd>`,
				`<dt>Wants</dt><dd>3</dd>`, `<dt>Avoids</dt><dd>1</dd>`,
				`<dt>url</dt><dd><a href="https://example.com/ref">https://example.com/ref</a></dd>`,
				`<dt>notes</dt><dd>quiet; sunny</dd>`, `<dt>no draft</dt><dd>yes</dd>`,
				`<dt>Target</dt><dd><a href="/profiles/newp">newp</a></dd>`,
			}},
			{queued, true, []string{`<span class="status queued">queued</span>`, "Queued, waiting for the runner…", "No events yet."}},
		} {
			target := "/jobs/" + id(tc.job)
			res, body := get(t, h, target)
			if res.StatusCode != http.StatusOK {
				t.Fatalf("GET %s = %d\n%s", target, res.StatusCode, body)
			}
			if got := strings.Contains(body, `<meta http-equiv="refresh" content="3">`); got != tc.refresh {
				t.Errorf("GET %s refresh = %v, want %v", target, got, tc.refresh)
			}
			for _, s := range tc.contains {
				if !strings.Contains(body, s) {
					t.Errorf("GET %s: missing %q", target, s)
				}
			}
			if strings.Contains(body, "<script>") || strings.Contains(body, "<b>") {
				t.Errorf("GET %s: unescaped markup", target)
			}
		}

		_, body := get(t, h, "/jobs/"+id(run))
		var events []string
		for _, m := range eventRow.FindAllStringSubmatch(body, -1) {
			events = append(events, m[1])
		}
		if want := []string{"fetching &lt;b&gt;somerville&lt;/b&gt;", "graded", "2 over budget"}; strings.Join(events, "|") != strings.Join(want, "|") {
			t.Errorf("events = %q, want %q", events, want)
		}

		for _, target := range []string{"/jobs/999", "/jobs/abc", "/jobs/0", "/jobs/-1"} {
			if res, _ := get(t, h, target); res.StatusCode != http.StatusNotFound {
				t.Errorf("GET %s = %d, want 404", target, res.StatusCode)
			}
		}

		_, body = get(t, h, "/c/somerville")
		for _, s := range []string{
			`<form method="post" action="/jobs/run"><input type="hidden" name="collection" value="somerville"><button type="submit">Run now</button></form>`,
			`<span class="status queued">Queued…</span> · <a href="/jobs/` + id(queued) + `">view</a>`,
		} {
			if !strings.Contains(body, s) {
				t.Errorf("collection page missing %q", s)
			}
		}
		_, body = get(t, h, "/c/zempty")
		for _, s := range []string{
			`<input type="hidden" name="collection" value="zempty">`,
			`Last run: <span class="status failed">failed</span> just now · <a href="/jobs/` + id(failed) + `">view</a>`,
		} {
			if !strings.Contains(body, s) {
				t.Errorf("empty collection page missing %q", s)
			}
		}
	})
}

func TestRunningJob(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := seededApp(t)
		q := a.jobs.(*jobs.Queue)
		h := a.Handler()

		exec := &fakeExec{started: make(chan string, 1), release: make(chan struct{})}
		ctx, cancel := context.WithCancel(t.Context())
		done := make(chan error, 1)
		go func() { done <- (&jobs.Runner{Queue: q, Exec: exec, Poll: 10 * time.Millisecond}).Run(ctx) }()
		defer func() {
			cancel()
			<-done
		}()

		res, _ := post(t, h, "/jobs/run", url.Values{"collection": {"somerville"}}, sameOriginHeader)
		jobID, _ := strconv.ParseInt(strings.TrimPrefix(res.Header.Get("Location"), "/jobs/"), 10, 64)
		<-exec.started
		waitJob(t, q, jobID, func(j jobs.Job) bool { return j.Status == jobs.StatusRunning })

		_, body := get(t, h, "/jobs/"+id(jobID))
		for _, s := range []string{`<meta http-equiv="refresh" content="3">`, `<span class="status running">running</span>`, "Running… This page refreshes", "fetching &lt;b&gt;somerville&lt;/b&gt;"} {
			if !strings.Contains(body, s) {
				t.Errorf("running job page missing %q", s)
			}
		}
		if _, body := get(t, h, "/c/somerville"); !strings.Contains(body, `<span class="status running">Running…</span> · <a href="/jobs/`+id(jobID)+`">view</a>`) {
			t.Error("collection page should show the running job")
		}

		close(exec.release)
		waitJob(t, q, jobID, func(j jobs.Job) bool { return j.Status == jobs.StatusSucceeded })
		_, body = get(t, h, "/jobs/"+id(jobID))
		if strings.Contains(body, `http-equiv="refresh"`) || strings.Contains(body, "Running…") {
			t.Error("finished job page should stop refreshing")
		}
		if _, body := get(t, h, "/c/somerville"); !strings.Contains(body, `Last run: <span class="status succeeded">succeeded</span> just now`) {
			t.Error("collection page should show the finished job")
		}
	})
}

func TestRunNow(t *testing.T) {
	a := seededApp(t)
	q := a.jobs.(*jobs.Queue)
	h := a.Handler()
	form := func(c string) url.Values { return url.Values{"collection": {c}} }

	res, _ := post(t, h, "/jobs/run", form("somerville"), sameOriginHeader)
	loc := res.Header.Get("Location")
	if res.StatusCode != http.StatusSeeOther || !strings.HasPrefix(loc, "/jobs/") {
		t.Fatalf("POST /jobs/run = %d %q", res.StatusCode, loc)
	}
	jobID, _ := strconv.ParseInt(strings.TrimPrefix(loc, "/jobs/"), 10, 64)
	j, err := q.Job(t.Context(), jobID)
	if err != nil || j.Kind != string(jobs.KindRunCollection) || j.CollectionID != "somerville" || j.Trigger != string(jobs.TriggerManual) || j.Status != jobs.StatusQueued {
		t.Fatalf("enqueued job = %+v %v", j, err)
	}
	if res, _ := post(t, h, "/jobs/run", form("somerville"), sameOriginHeader); res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != loc {
		t.Errorf("duplicate run = %d %q, want %q", res.StatusCode, res.Header.Get("Location"), loc)
	}
	if res, _ := post(t, h, "/jobs/run", form("zempty"), sameOriginHeader); res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") == loc {
		t.Errorf("other collection = %d %q", res.StatusCode, res.Header.Get("Location"))
	}

	for _, tc := range []struct {
		collection string
		status     int
		contains   string
	}{
		{"nope", http.StatusNotFound, "There is no collection named nope."},
		{"<b>x</b>", http.StatusNotFound, "There is no collection named &lt;b&gt;x&lt;/b&gt;."},
		{"  ", http.StatusBadRequest, "Choose a collection to run."},
	} {
		res, body := post(t, h, "/jobs/run", form(tc.collection), sameOriginHeader)
		if res.StatusCode != tc.status || !strings.Contains(body, tc.contains) || !strings.Contains(body, "<nav>") {
			t.Errorf("POST %q = %d\n%s", tc.collection, res.StatusCode, body)
		}
	}
	if js, _ := q.Jobs(t.Context(), jobs.Filter{}); len(js) != 2 {
		t.Errorf("jobs = %d, want 2", len(js))
	}
	if res, _ := get(t, h, "/jobs/run"); res.StatusCode != http.StatusNotFound {
		t.Errorf("GET /jobs/run = %d, want 404", res.StatusCode)
	}
}

func TestSameOrigin(t *testing.T) {
	a := seededApp(t)
	q := a.jobs.(*jobs.Queue)
	h := a.Handler()
	form := url.Values{"collection": {"somerville"}}

	for _, tc := range []struct {
		name   string
		header map[string]string
		status int
	}{
		{"no headers", nil, http.StatusForbidden},
		{"cross-site fetch", map[string]string{"Sec-Fetch-Site": "cross-site", "Origin": "http://example.com"}, http.StatusForbidden},
		{"same-site fetch", map[string]string{"Sec-Fetch-Site": "same-site"}, http.StatusForbidden},
		{"foreign origin", map[string]string{"Origin": "https://evil.example"}, http.StatusForbidden},
		{"null origin", map[string]string{"Origin": "null"}, http.StatusForbidden},
		{"foreign referer", map[string]string{"Referer": "https://evil.example/c/somerville"}, http.StatusForbidden},
		{"lookalike origin", map[string]string{"Origin": "http://example.com.evil.example"}, http.StatusForbidden},
		{"same-origin fetch", map[string]string{"Sec-Fetch-Site": "same-origin"}, http.StatusSeeOther},
		{"user typed", map[string]string{"Sec-Fetch-Site": "none"}, http.StatusSeeOther},
		{"matching origin", map[string]string{"Origin": "https://EXAMPLE.com"}, http.StatusSeeOther},
		{"matching referer", map[string]string{"Referer": "http://example.com/c/somerville"}, http.StatusSeeOther},
	} {
		res, body := post(t, h, "/jobs/run", form, tc.header)
		if res.StatusCode != tc.status {
			t.Errorf("%s: POST = %d, want %d", tc.name, res.StatusCode, tc.status)
		}
		if tc.status == http.StatusForbidden && !strings.Contains(body, "Request blocked") {
			t.Errorf("%s: 403 should render the error page\n%s", tc.name, body)
		}
	}

	ctx := t.Context()
	if js, _ := q.Jobs(ctx, jobs.Filter{}); len(js) != 1 {
		t.Errorf("only same-origin posts should enqueue; jobs = %d", len(js))
	}

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("cross-site GET = %d", rec.Code)
	}
	req = httptest.NewRequest(http.MethodHead, "/healthz", nil)
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("cross-site HEAD = %d", rec.Code)
	}
	if res, _ := post(t, h, "/nope", nil, map[string]string{"Sec-Fetch-Site": "cross-site"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site POST to an unknown path = %d, want 403", res.StatusCode)
	}
}
