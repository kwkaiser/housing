package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"image"
	"image/jpeg"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

func apiSend(t *testing.T, h http.Handler, method, target, key string, body any) (*http.Response, []byte) {
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, target, r)
	req.Header.Set("Authorization", "Bearer "+key)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	b, _ := io.ReadAll(res.Body)
	return res, b
}

func decode[T any](t *testing.T, b []byte) T {
	t.Helper()
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("%v\n%s", err, b)
	}
	return v
}

func problemFields(t *testing.T, b []byte) map[string]string {
	t.Helper()
	p := decode[huma.ErrorModel](t, b)
	out := map[string]string{}
	for _, d := range p.Errors {
		out[d.Location] = d.Message
	}
	return out
}

func TestAPICreateAndUpdateCollection(t *testing.T) {
	_, h, key := apiApp(t)

	body := map[string]any{"id": "cambridge", "mode": "rent", "sources": []string{"zillow"}, "profiles": []string{"loft", "attic"}, "location": "02139", "max_price": 3500, "schedule": "07:30"}
	res, b := apiSend(t, h, http.MethodPost, "/api/v1/collections", key, body)
	if res.StatusCode != http.StatusCreated {
		t.Fatalf("create = %d\n%s", res.StatusCode, b)
	}
	c := decode[APICollection](t, b)
	if c.ID != "cambridge" || strings.Join(c.Profiles, ",") != "loft,attic" || c.Search.Location != "02139" || *c.Search.MaxPrice != 3500 || c.Schedule != "07:30" || c.LastRun != nil {
		t.Errorf("created = %+v", c)
	}

	res, b = apiSend(t, h, http.MethodPost, "/api/v1/collections", key, body)
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(problemFields(t, b)["body.id"], "exists") {
		t.Errorf("duplicate = %d\n%s", res.StatusCode, b)
	}
	res, b = apiSend(t, h, http.MethodPost, "/api/v1/collections", key, map[string]any{"id": "bad", "mode": "rent", "sources": []string{"zillow"}, "profiles": []string{"corp"}, "location": "02139"})
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(problemFields(t, b)["body.profiles"], "avoid profile") {
		t.Errorf("avoid profile = %d\n%s", res.StatusCode, b)
	}
	if res, b := apiSend(t, h, http.MethodPost, "/api/v1/collections", key, map[string]any{"id": "bad", "mode": "lease"}); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("bad mode = %d\n%s", res.StatusCode, b)
	}

	res, b = apiSend(t, h, http.MethodPatch, "/api/v1/collections/cambridge", key, map[string]any{"location": "02138", "notify_url": "https://ntfy.example/x", "clear": []string{"max_price"}})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update = %d\n%s", res.StatusCode, b)
	}
	c = decode[APICollection](t, b)
	if c.Search.Location != "02138" || c.Search.MaxPrice != nil || c.NotifyURL != "https://ntfy.example/x" || strings.Join(c.Profiles, ",") != "loft,attic" {
		t.Errorf("updated = %+v", c)
	}
	if res, b := apiSend(t, h, http.MethodPatch, "/api/v1/collections/cambridge", key, map[string]any{"schedule": "25:00"}); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("bad schedule = %d\n%s", res.StatusCode, b)
	}
	if res, _ := apiSend(t, h, http.MethodPatch, "/api/v1/collections/nope", key, map[string]any{"location": "02138"}); res.StatusCode != http.StatusNotFound {
		t.Errorf("update missing = %d", res.StatusCode)
	}

	if res, b := apiSend(t, h, http.MethodPost, "/api/v1/collections/cambridge/runs", key, nil); res.StatusCode != http.StatusAccepted {
		t.Fatalf("run = %d\n%s", res.StatusCode, b)
	}
	if c := apiGet[APICollection](t, h, key, "/api/v1/collections/cambridge"); c.LastRun == nil || c.LastRun.Status != "queued" {
		t.Errorf("last run = %+v", c.LastRun)
	}
}

func TestAPIFavorites(t *testing.T) {
	_, h, key := apiApp(t)

	if res, b := apiSend(t, h, http.MethodPut, "/api/v1/listings/zillow/z1/favorite", key, nil); res.StatusCode != http.StatusNoContent {
		t.Fatalf("add = %d\n%s", res.StatusCode, b)
	}
	if res, _ := apiSend(t, h, http.MethodPut, "/api/v1/listings/zillow/z1/favorite", key, nil); res.StatusCode != http.StatusNoContent {
		t.Errorf("add again = %d", res.StatusCode)
	}
	if res, _ := apiSend(t, h, http.MethodPut, "/api/v1/listings/zillow/nope/favorite", key, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("add missing = %d", res.StatusCode)
	}
	fs := apiGet[[]APIFavorite](t, h, key, "/api/v1/favorites")
	if len(fs) != 1 || fs[0].Listing.SourceID != "z1" || fs[0].Best == nil || fs[0].LastSeen != day2 {
		t.Errorf("favorites = %+v", fs)
	}
	if d := apiGet[APIListingDetail](t, h, key, "/api/v1/listings/zillow/z1"); !d.Favorite || d.Location == nil || !d.Location.Approx {
		t.Errorf("detail = favorite %v location %+v", d.Favorite, d.Location)
	}
	v := apiGet[APICollectionListings](t, h, key, "/api/v1/collections/somerville/listings")
	if !v.Rows[0].Favorite || v.Rows[1].Favorite {
		t.Errorf("row favorites = %v %v", v.Rows[0].Favorite, v.Rows[1].Favorite)
	}
	if res, _ := apiSend(t, h, http.MethodDelete, "/api/v1/listings/zillow/z1/favorite", key, nil); res.StatusCode != http.StatusNoContent {
		t.Errorf("remove = %d", res.StatusCode)
	}
	if fs := apiGet[[]APIFavorite](t, h, key, "/api/v1/favorites"); len(fs) != 0 {
		t.Errorf("after remove = %+v", fs)
	}
}

func TestAPIListingsPaging(t *testing.T) {
	_, h, key := apiApp(t)
	v := apiGet[APICollectionListings](t, h, key, "/api/v1/collections/somerville/listings?offset=1&limit=1")
	if v.Matched != 3 || len(v.Rows) != 1 || v.Rows[0].Listing.SourceID != "z4" {
		t.Errorf("page = matched %d rows %+v", v.Matched, v.Rows)
	}
	if v := apiGet[APICollectionListings](t, h, key, "/api/v1/collections/somerville/listings?offset=5"); v.Matched != 3 || len(v.Rows) != 0 {
		t.Errorf("past the end = matched %d rows %d", v.Matched, len(v.Rows))
	}
}

func TestAPIProfileWrites(t *testing.T) {
	_, h, key := apiApp(t)

	ps := apiGet[[]APIProfileListItem](t, h, key, "/api/v1/profiles")
	var attic APIProfileListItem
	for _, p := range ps {
		if p.ID == "attic" {
			attic = p
		}
	}
	if attic.Wants != 2 || attic.References != 1 || strings.Join(attic.Collections, ",") != "somerville,zempty" {
		t.Errorf("attic summary = %+v", attic)
	}
	if d := apiGet[APIProfileDetail](t, h, key, "/api/v1/profiles/attic"); len(d.References) != 1 || d.References[0].SourceID != "ref" || d.References[0].Scores[model] == 0 {
		t.Errorf("attic references = %+v", d.References)
	}

	res, b := apiSend(t, h, http.MethodPatch, "/api/v1/profiles/attic", key, map[string]any{
		"name":     "Bright attic",
		"criteria": []map[string]any{{"id": "skylights", "importance": "essential"}, {"id": "floors", "remove": true}},
	})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("update = %d\n%s", res.StatusCode, b)
	}
	d := decode[APIProfileDetail](t, b)
	if d.Profile.Name != "Bright attic" || d.Profile.Summary != "Top floor with skylights." || len(d.Profile.Want) != 1 || d.Profile.Want[0].Importance != "essential" {
		t.Errorf("updated = %+v", d.Profile)
	}
	res, b = apiSend(t, h, http.MethodPatch, "/api/v1/profiles/attic", key, map[string]any{"criteria": []map[string]any{{"id": "carpet", "label": "x"}}})
	if res.StatusCode != http.StatusUnprocessableEntity || problemFields(t, b)["body.criteria"] == "" {
		t.Errorf("inherited criterion = %d\n%s", res.StatusCode, b)
	}
	if res, _ := apiSend(t, h, http.MethodPatch, "/api/v1/profiles/nope", key, map[string]any{"name": "x"}); res.StatusCode != http.StatusNotFound {
		t.Errorf("update missing = %d", res.StatusCode)
	}

	res, b = apiSend(t, h, http.MethodPost, "/api/v1/profiles/attic/drafts", key, nil)
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("redraft = %d\n%s", res.StatusCode, b)
	}
	if j := decode[APIJob](t, b); j.Kind != "draft_profile" || j.ProfileID != "attic" {
		t.Errorf("redraft job = %+v", j)
	}
	if res, b := apiSend(t, h, http.MethodPost, "/api/v1/profiles/loft/drafts", key, map[string]any{"model": "x/y"}); res.StatusCode != http.StatusConflict {
		t.Errorf("redraft without references = %d\n%s", res.StatusCode, b)
	}

	res, b = apiSend(t, h, http.MethodPost, "/api/v1/profiles", key, map[string]any{"url": "ftp://example.com/x", "id": "Bad Id"})
	f := problemFields(t, b)
	if res.StatusCode != http.StatusUnprocessableEntity || f["body.url"] == "" || f["body.id"] == "" {
		t.Errorf("create invalid = %d\n%s", res.StatusCode, b)
	}
}

func TestAPIJobsPaging(t *testing.T) {
	a, h, key := apiApp(t)
	var ids []int64
	for _, p := range []jobs.Params{jobs.RunCollectionParams{CollectionID: "somerville"}, jobs.RunCollectionParams{CollectionID: "zempty"}, jobs.DraftProfileParams{ProfileID: "attic"}} {
		id, err := a.jobs.Enqueue(t.Context(), p)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}

	res, b := apiDo(t, h, http.MethodGet, "/api/v1/jobs?limit=2", key)
	page := decode[[]APIJob](t, b)
	if len(page) != 2 || page[0].ID != ids[2] || page[1].ID != ids[1] {
		t.Fatalf("first page = %+v", page)
	}
	link := res.Header.Get("Link")
	next, ok := strings.CutPrefix(strings.TrimSuffix(link, `>; rel="next"`), "<")
	if !ok || !strings.Contains(next, "before="+strconv.FormatInt(ids[1], 10)) {
		t.Fatalf("link = %q", link)
	}
	res, b = apiDo(t, h, http.MethodGet, next, key)
	if page := decode[[]APIJob](t, b); len(page) != 1 || page[0].ID != ids[0] || res.Header.Get("Link") != "" {
		t.Errorf("second page = %+v, link %q", page, res.Header.Get("Link"))
	}
	for target, want := range map[string]int{
		"/api/v1/jobs?profile=attic":       1,
		"/api/v1/jobs?kind=run_collection": 2,
		"/api/v1/jobs?kind=draft_profile":  1,
	} {
		if js := apiGet[[]APIJob](t, h, key, target); len(js) != want {
			t.Errorf("GET %s = %d jobs, want %d", target, len(js), want)
		}
	}
}

func addEvents(t *testing.T, db *store.Store, id int64, from, to int) {
	t.Helper()
	for seq := from; seq <= to; seq++ {
		e := store.JobEvent{JobID: id, Seq: seq, At: time.Now(), Level: slog.LevelInfo, Message: "step " + strconv.Itoa(seq), Record: json.RawMessage(`{}`)}
		if err := db.AppendJobEvent(t.Context(), e); err != nil {
			t.Fatal(err)
		}
	}
}

type sseMessage struct {
	id, event, data string
}

func readSSE(t *testing.T, r io.Reader) []sseMessage {
	t.Helper()
	var out []sseMessage
	var cur sseMessage
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if cur.data != "" {
				out = append(out, cur)
			}
			cur = sseMessage{}
		case strings.HasPrefix(line, "id: "):
			cur.id = line[4:]
		case strings.HasPrefix(line, "event: "):
			cur.event = line[7:]
		case strings.HasPrefix(line, "data: "):
			cur.data = line[6:]
		}
	}
	return out
}

func TestAPIJobStream(t *testing.T) {
	a, h, key := apiApp(t)
	a.streamPoll = 10 * time.Millisecond
	db := a.svc.Store()
	id, err := a.jobs.Enqueue(t.Context(), jobs.RunCollectionParams{CollectionID: "somerville"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := db.ClaimJob(t.Context(), time.Now()); err != nil {
		t.Fatal(err)
	}
	addEvents(t, db, id, 1, 2)

	srv := httptest.NewServer(h)
	defer srv.Close()
	req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+"/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/stream", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Last-Event-ID", "1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "text/event-stream" {
		t.Fatalf("stream = %d %q", res.StatusCode, res.Header.Get("Content-Type"))
	}

	go func() {
		time.Sleep(50 * time.Millisecond)
		addEvents(t, db, id, 3, 3)
		j, _ := db.Job(t.Context(), id)
		j.Status, j.FinishedAt = jobs.StatusSucceeded, time.Now()
		db.FinishJob(t.Context(), j)
	}()

	msgs := readSSE(t, res.Body)
	var got []string
	for _, m := range msgs {
		switch m.event {
		case "event":
			got = append(got, "event "+m.id+" "+decode[APIJobEvent](t, []byte(m.data)).Message)
		case "job":
			got = append(got, "job "+string(decode[APIJob](t, []byte(m.data)).Status))
		default:
			t.Errorf("unexpected message %+v", m)
		}
	}
	if want := "event 2 step 2|job running|event 3 step 3|job succeeded"; strings.Join(got, "|") != want {
		t.Errorf("messages = %s, want %s", strings.Join(got, "|"), want)
	}

	if res, _ := apiDo(t, h, http.MethodGet, "/api/v1/jobs/999/stream", key); res.StatusCode != http.StatusNotFound {
		t.Errorf("missing job stream = %d", res.StatusCode)
	}
	if es := apiGet[[]APIJobEvent](t, h, key, "/api/v1/jobs/"+strconv.FormatInt(id, 10)+"/events?after=2"); len(es) != 1 || es[0].Seq != 3 {
		t.Errorf("events after 2 = %+v", es)
	}
}

func TestAPIMapPins(t *testing.T) {
	_, h, key := apiApp(t)

	v := apiGet[APIMapPins](t, h, key, "/api/v1/map/pins")
	if v.FirstDay != day1 || v.LastDay != day2 || len(v.Collections) != 2 || v.Collections[0].ID != "somerville" || v.Collections[0].Color == "" {
		t.Errorf("summary = %+v", v)
	}
	var z1 *APIMapPin
	for i, p := range v.Pins {
		if p.SourceID == "z1" {
			z1 = &v.Pins[i]
		}
	}
	if z1 == nil {
		t.Fatalf("no z1 pin in %+v", v.Pins)
	}
	g, ok := z1.Grades["somerville"]
	if !z1.Approx || z1.PriceCents != 290000 || z1.Href != "/api/v1/listings/zillow/z1" || z1.Preview != "/api/v1/media/"+collageKey || !ok || g.Match != 87.5 {
		t.Errorf("z1 = %+v", z1)
	}
	if v := apiGet[APIMapPins](t, h, key, "/api/v1/map/pins?from="+day2+"&to="+day1); v.From != day1 || v.To != day2 {
		t.Errorf("swapped range = %s..%s", v.From, v.To)
	}
	if res, _ := apiDo(t, h, http.MethodGet, "/api/v1/map/pins?from=yesterday", key); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("bad date = %d", res.StatusCode)
	}

	c := apiGet[APIMapConfig](t, h, key, "/api/v1/map/config")
	if c.Positron != nil || c.Overlays == nil {
		t.Errorf("config = %+v", c)
	}
	for _, o := range c.Overlays {
		if !strings.HasSuffix(o.URL, "/{y}.json") || !strings.HasSuffix(o.VectorURL, "/{y}.pbf") {
			t.Errorf("overlay = %+v", o)
		}
	}
	for _, target := range []string{"/api/v1/tiles/positron/1/0/0.png", "/api/v1/overlays/rail/1/0/0.json", "/api/v1/overlays/nope/14/0/0.json", "/api/v1/overlays/bike/14/0/0.txt"} {
		if res, _ := apiDo(t, h, http.MethodGet, target, key); res.StatusCode != http.StatusNotFound && res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("GET %s = %d", target, res.StatusCode)
		}
	}
}

func TestAPIKeys(t *testing.T) {
	_, h, key := apiApp(t)

	res, b := apiSend(t, h, http.MethodPost, "/api/v1/keys", key, map[string]any{"name": "phone"})
	if res.StatusCode != http.StatusCreated || res.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("create = %d %v\n%s", res.StatusCode, res.Header, b)
	}
	created := decode[APICreatedKey](t, b)
	if created.Key.Name != "phone" || !strings.HasPrefix(created.Token, created.Key.Hint) {
		t.Errorf("created = %+v", created)
	}
	if res, _ := apiDo(t, h, http.MethodGet, "/api/v1/collections", created.Token); res.StatusCode != http.StatusOK {
		t.Errorf("new key = %d", res.StatusCode)
	}
	if ks := apiGet[[]APIKey](t, h, key, "/api/v1/keys"); len(ks) != 2 {
		t.Errorf("keys = %+v", ks)
	}
	if res, b := apiSend(t, h, http.MethodPost, "/api/v1/keys", key, map[string]any{"name": " "}); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("blank name = %d\n%s", res.StatusCode, b)
	}
	target := "/api/v1/keys/" + strconv.FormatInt(created.Key.ID, 10)
	if res, _ := apiSend(t, h, http.MethodDelete, target, key, nil); res.StatusCode != http.StatusNoContent {
		t.Errorf("delete = %d", res.StatusCode)
	}
	if res, _ := apiSend(t, h, http.MethodDelete, target, key, nil); res.StatusCode != http.StatusNotFound {
		t.Errorf("delete again = %d", res.StatusCode)
	}
	if res, _ := apiDo(t, h, http.MethodGet, "/api/v1/collections", created.Token); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("deleted key = %d", res.StatusCode)
	}
}

func TestAPIETags(t *testing.T) {
	_, h, key := apiApp(t)

	res, b := apiDo(t, h, http.MethodGet, "/api/v1/collections", key)
	tag := res.Header.Get("ETag")
	if res.StatusCode != http.StatusOK || tag == "" || len(b) == 0 || res.Header.Get("Cache-Control") != "private, no-cache" {
		t.Fatalf("first = %d %v", res.StatusCode, res.Header)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/collections", nil)
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("If-None-Match", "W/"+tag)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified || rec.Body.Len() != 0 || rec.Header().Get("ETag") != tag {
		t.Errorf("conditional = %d %q", rec.Code, rec.Body.String())
	}
	if res, _ := apiDo(t, h, http.MethodGet, "/api/v1/collections", ""); res.StatusCode != http.StatusUnauthorized || res.Header.Get("ETag") != "" {
		t.Errorf("unauthorized = %d %v", res.StatusCode, res.Header)
	}
}

func TestAPIMediaSize(t *testing.T) {
	a, h, key := apiApp(t)
	const big = "collages/cd/big"
	p, err := a.svc.MediaPath(big)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 1000, 500)), nil); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		res, b := apiDo(t, h, http.MethodGet, "/api/v1/media/"+big+"?size=320", key)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/jpeg" {
			t.Fatalf("resized = %d %v", res.StatusCode, res.Header)
		}
		cfg, err := jpeg.DecodeConfig(bytes.NewReader(b))
		if err != nil || cfg.Width != 320 || cfg.Height != 160 {
			t.Errorf("resized = %+v %v", cfg, err)
		}
	}
	if _, err := os.Stat(p + ".s320.jpg"); err != nil {
		t.Errorf("resized copy not cached: %v", err)
	}
	if res, b := apiDo(t, h, http.MethodGet, "/api/v1/media/"+collageKey+"?size=320", key); res.StatusCode != http.StatusOK || len(b) == 0 {
		t.Errorf("small image = %d", res.StatusCode)
	}
	if res, _ := apiDo(t, h, http.MethodGet, "/api/v1/media/"+big+"?size=100", key); res.StatusCode != http.StatusUnprocessableEntity {
		t.Errorf("unsupported size = %d", res.StatusCode)
	}
}

func TestAPIMeta(t *testing.T) {
	_, h, key := apiApp(t)
	m := apiGet[APIMeta](t, h, key, "/api/v1/meta")
	if m.Version != "dev" || len(m.Modes) != 2 || len(m.Sources) == 0 || len(m.Amenities) == 0 || len(m.Importances) != 4 || m.Defaults.DraftModel == "" || m.Defaults.CollectionBudgetUSD == 0 {
		t.Errorf("meta = %+v", m)
	}
}

func TestAPIListingsRange(t *testing.T) {
	_, h, key := apiApp(t)
	summary := func(v APICollectionListings) string {
		var out []string
		for _, r := range v.Rows {
			out = append(out, r.Listing.SourceID+"@"+r.Day+":"+string(r.Status))
		}
		return strings.Join(out, ",")
	}

	all := apiGet[APICollectionListings](t, h, key, "/api/v1/collections/somerville/listings?from="+day1)
	if all.Day != "" || all.From != day1 || all.Rows[0].Rank != 1 || all.Rows[1].Rank != 2 {
		t.Errorf("range = %+v", all)
	}
	if got, want := summary(all), "z1@"+day2+":new,z4@"+day2+":new,z2@"+day2+":new"; got != want {
		t.Errorf("from %s = %s, want %s", day1, got, want)
	}
	if got, want := summary(apiGet[APICollectionListings](t, h, key, "/api/v1/collections/somerville/listings?to="+day1)), "z1@"+day1+":new,z2@"+day1+":new"; got != want {
		t.Errorf("to %s = %s, want %s", day1, got, want)
	}
	late := apiGet[APICollectionListings](t, h, key, "/api/v1/collections/somerville/listings?from="+day2+"&dealbreakers=true")
	if got, want := summary(late), "z1@"+day2+":changed,z4@"+day2+":new,z2@"+day2+":repeat,z3@"+day2+":repeat"; got != want {
		t.Errorf("from %s with dealbreakers = %s, want %s", day2, got, want)
	}
	if day := apiGet[APICollectionListings](t, h, key, "/api/v1/collections/somerville/listings"); day.Rows[0].Day != day2 {
		t.Errorf("day rows should carry their day, got %q", day.Rows[0].Day)
	}
}

func TestSearchArea(t *testing.T) {
	area := searchArea(collection.Collection{Search: profile.Search{Location: " 02144 ", RadiusMiles: 1}})
	if area == nil || area.South >= 42.4001 || area.North <= 42.4001 || area.West >= -71.1231 || area.East <= -71.1231 {
		t.Errorf("searchArea(02144) = %+v", area)
	}
	wide := searchArea(collection.Collection{Search: profile.Search{Location: "02144"}})
	if wide.North-wide.South <= area.North-area.South {
		t.Errorf("default radius area %+v should be wider than 1 mile area %+v", wide, area)
	}
	if area := searchArea(collection.Collection{Search: profile.Search{Location: "Somerville, MA"}}); area != nil {
		t.Error("searchArea should not resolve a non-zip location")
	}
}
