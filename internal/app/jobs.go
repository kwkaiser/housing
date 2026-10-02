package app

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/app/view"
	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
)

const (
	defaultJobLimit = 50
	maxJobLimit     = 500
)

var jobStatuses = []jobs.Status{jobs.StatusQueued, jobs.StatusRunning, jobs.StatusSucceeded, jobs.StatusFailed, jobs.StatusCancelled}

type jobView struct {
	jobs.Job
	Href        string
	KindLabel   string
	Target      string
	TargetHref  string
	StatusClass string
	Active      bool
	Created     string
	Ago         string
	Duration    string
	Cost        string
	ShortError  string
}

type option struct {
	Value    string
	Selected bool
}

type jobsData struct {
	Title       string
	Jobs        []jobView
	Collection  string
	Status      string
	Limit       int
	CustomLimit bool
	Filtered    bool
	Collections []option
	Statuses    []option
}

type field struct {
	Label string
	Value string
	Href  string
}

type jobData struct {
	jobView
	Title   string
	Params  []field
	Result  []field
	Profile *field
	Events  []eventView
}

type eventView struct {
	jobs.Event
	Time       string
	LevelClass string
	Stage      string
	Attrs      []jobs.Attr
}

func (a *App) jobView(j jobs.Job, now time.Time) jobView {
	v := jobView{
		Job:         j,
		Href:        jobURL(j.ID),
		KindLabel:   view.KindLabel(j.Kind),
		StatusClass: view.StatusClass(string(j.Status)),
		Active:      j.Status.Active(),
		Created:     j.CreatedAt.Local().Format("2006-01-02 15:04:05"),
		Ago:         view.Ago(j.CreatedAt, now),
		Duration:    "-",
		Cost:        view.Cost(j.CostUSD),
		ShortError:  view.Truncate(j.Error, 80),
	}
	switch {
	case j.CollectionID != "":
		v.Target, v.TargetHref = j.CollectionID, collectionURL(j.CollectionID)
	case j.ProfileID != "":
		v.Target, v.TargetHref = j.ProfileID, profileURL(j.ProfileID)
	}
	if !j.StartedAt.IsZero() {
		end := j.FinishedAt
		if end.IsZero() {
			end = now
		}
		v.Duration = view.Elapsed(end.Sub(j.StartedAt))
	}
	return v
}

func (a *App) jobsPage(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	d := jobsData{Title: "Jobs", Collection: strings.TrimSpace(q.Get("collection")), Limit: defaultJobLimit}
	if s := jobs.Status(q.Get("status")); slices.Contains(jobStatuses, s) {
		d.Status = string(s)
	}
	if n, err := strconv.Atoi(q.Get("limit")); err == nil && n > 0 {
		d.Limit = min(n, maxJobLimit)
		d.CustomLimit = d.Limit != defaultJobLimit
	}
	d.Filtered = d.Collection != "" || d.Status != ""

	js, err := a.jobs.Jobs(r.Context(), jobs.Filter{CollectionID: d.Collection, Status: jobs.Status(d.Status), Limit: d.Limit})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	now := a.now()
	for _, j := range js {
		d.Jobs = append(d.Jobs, a.jobView(j, now))
	}

	cs, err := a.svc.Collections(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	for _, c := range cs {
		d.Collections = append(d.Collections, option{Value: c.ID, Selected: c.ID == d.Collection})
	}
	if d.Collection != "" && !slices.ContainsFunc(cs, func(c collection.Collection) bool { return c.ID == d.Collection }) {
		d.Collections = append(d.Collections, option{Value: d.Collection, Selected: true})
	}
	for _, s := range jobStatuses {
		d.Statuses = append(d.Statuses, option{Value: string(s), Selected: string(s) == d.Status})
	}
	a.render(w, r, http.StatusOK, "jobs", d)
}

func (a *App) jobPage(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		a.notFound(w, r)
		return
	}
	j, err := a.jobs.Job(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	events, err := a.jobs.Events(r.Context(), id)
	if err != nil {
		a.fail(w, r, err)
		return
	}
	d := jobData{jobView: a.jobView(j, a.now()), Title: "Job " + strconv.FormatInt(j.ID, 10), Params: paramFields(j.Params)}
	for _, e := range events {
		v := eventView{Event: e, Time: e.At.Local().Format(time.TimeOnly), LevelClass: strings.ToLower(e.Level.String())}
		attrs, err := jobs.EventAttrs(e)
		if err != nil {
			a.fail(w, r, err)
			return
		}
		for _, at := range attrs {
			if at.Key == "stage" {
				v.Stage = at.Value
				continue
			}
			v.Attrs = append(v.Attrs, at)
		}
		d.Events = append(d.Events, v)
	}
	if len(j.Result) > 0 {
		switch jobs.Kind(j.Kind) {
		case jobs.KindRunCollection:
			var res jobs.RunResult
			if err := json.Unmarshal(j.Result, &res); err == nil {
				d.Result = runFields(res)
			}
		case jobs.KindCreateProfile, jobs.KindDraftProfile:
			var res jobs.ProfileResult
			if err := json.Unmarshal(j.Result, &res); err == nil && res.ProfileID != "" {
				d.Profile = &field{Label: cmp.Or(res.Name, res.ProfileID), Value: res.ProfileID, Href: profileURL(res.ProfileID)}
				d.Result = []field{
					{Label: "Wants", Value: strconv.Itoa(res.Want)},
					{Label: "Avoids", Value: strconv.Itoa(res.Avoid)},
				}
			}
		}
	}
	a.render(w, r, http.StatusOK, "job", d)
}

func (a *App) runCollection(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
	if err := r.ParseForm(); err != nil {
		a.renderMessage(w, r, http.StatusBadRequest, "Bad request", "The form could not be read.")
		return
	}
	id := strings.TrimSpace(r.PostForm.Get("collection"))
	if id == "" {
		a.renderMessage(w, r, http.StatusBadRequest, "Bad request", "Choose a collection to run.")
		return
	}
	if _, err := a.svc.Collection(r.Context(), id); err != nil {
		if errors.Is(err, collection.ErrNotFound) {
			a.renderMessage(w, r, http.StatusNotFound, "Collection not found", "There is no collection named "+id+".")
			return
		}
		a.fail(w, r, err)
		return
	}
	jobID, err := a.jobs.Enqueue(r.Context(), jobs.RunCollectionParams{CollectionID: id})
	if err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, jobURL(jobID), http.StatusSeeOther)
}

func (a *App) runStatus(ctx context.Context, collectionID string) (*jobView, error) {
	j, ok, err := a.jobs.ActiveForCollection(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	if !ok {
		if j, ok, err = a.jobs.LatestForCollection(ctx, collectionID); err != nil || !ok {
			return nil, err
		}
	}
	ref := j.FinishedAt
	if ref.IsZero() {
		ref = j.CreatedAt
	}
	now := a.now()
	v := a.jobView(j, now)
	v.Ago = view.Ago(ref, now)
	return &v, nil
}

func paramFields(raw json.RawMessage) []field {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		if len(raw) == 0 {
			return nil
		}
		return []field{{Label: "Params", Value: string(raw)}}
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	out := make([]field, 0, len(keys))
	for _, k := range keys {
		f := field{Label: strings.ReplaceAll(k, "_", " "), Value: paramValue(m[k])}
		switch k {
		case "collection_id":
			f.Label, f.Href = "collection", collectionURL(f.Value)
		case "profile_id":
			f.Label, f.Href = "profile", profileURL(f.Value)
		case "url":
			f.Href = safeURL(f.Value)
		}
		out = append(out, f)
	}
	return out
}

func paramValue(v any) string {
	switch v := v.(type) {
	case string:
		return v
	case bool:
		return map[bool]string{false: "no", true: "yes"}[v]
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case nil:
		return "-"
	case []any:
		s := make([]string, len(v))
		for i, x := range v {
			s[i] = paramValue(x)
		}
		return strings.Join(s, "; ")
	}
	b, _ := json.Marshal(v)
	return string(b)
}

func runFields(r jobs.RunResult) []field {
	n := strconv.Itoa
	yes := map[bool]string{false: "no", true: "yes"}
	fs := []field{{Label: "Day", Value: cmp.Or(r.Day, "-")}}
	if r.Day != "" && r.Collection != "" {
		fs[0].Href = listingsState{collection: r.Collection, day: r.Day}.href()
	}
	fs = append(fs,
		field{Label: "Fetched", Value: n(r.Fetched)},
		field{Label: "Distinct", Value: n(r.Distinct)},
		field{Label: "Duplicates", Value: n(r.Duplicates)},
		field{Label: "Collaged", Value: n(r.Collaged) + " listings, " + n(r.Collages) + " collages"},
		field{Label: "Without collages", Value: n(r.WithoutCollages)},
		field{Label: "Graded", Value: n(r.Stats.Updated)},
		field{Label: "Already current", Value: n(r.Stats.Cached)},
		field{Label: "Failed", Value: n(r.Stats.Failed)},
		field{Label: "Over budget", Value: n(r.Stats.OverBudget)},
		field{Label: "Model calls", Value: n(r.Stats.Calls)},
		field{Label: "Model cost", Value: view.Cost(r.Stats.CostUSD)},
		field{Label: "Tokens", Value: r.Stats.Tokens.String()},
		field{Label: "Budget reached", Value: yes[r.BudgetReached]},
	)
	if r.AssessError != "" {
		fs = append(fs, field{Label: "Assess error", Value: r.AssessError})
	}
	if r.Notified > 0 || r.NotifyError != "" {
		fs = append(fs, field{Label: "Notified", Value: n(r.Notified)})
	}
	if r.NotifyError != "" {
		fs = append(fs, field{Label: "Notify error", Value: r.NotifyError})
	}
	return fs
}

func safeURL(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return s
}

func jobURL(id int64) string {
	return "/jobs/" + strconv.FormatInt(id, 10)
}

func profileURL(id string) string {
	return "/profiles/" + url.PathEscape(id)
}
