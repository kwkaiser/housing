package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
)

func apiApp(t *testing.T) (*App, http.Handler, string) {
	t.Helper()
	a := seededApp(t)
	_, key, err := a.svc.CreateAPIKey(t.Context(), "test")
	if err != nil {
		t.Fatal(err)
	}
	return a, a.Handler(), key
}

func apiDo(t *testing.T, h http.Handler, method, target, key string) (*http.Response, []byte) {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	res := rec.Result()
	b, _ := io.ReadAll(res.Body)
	return res, b
}

func apiGet[T any](t *testing.T, h http.Handler, key, target string) T {
	t.Helper()
	res, b := apiDo(t, h, http.MethodGet, target, key)
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d\n%s", target, res.StatusCode, b)
	}
	var v T
	if err := json.Unmarshal(b, &v); err != nil {
		t.Fatalf("GET %s: %v\n%s", target, err, b)
	}
	return v
}

func TestAPIAuth(t *testing.T) {
	_, h, key := apiApp(t)
	for _, bad := range []string{"", "wrong", "hk_wrong", key + "x"} {
		res, b := apiDo(t, h, http.MethodGet, "/api/v1/collections", bad)
		if res.StatusCode != http.StatusUnauthorized || res.Header.Get("WWW-Authenticate") == "" || !strings.HasPrefix(res.Header.Get("Content-Type"), "application/problem+json") {
			t.Errorf("key %q: %d %v\n%s", bad, res.StatusCode, res.Header, b)
		}
	}
	if res, b := apiDo(t, h, http.MethodGet, "/api/v1/collections", key); res.StatusCode != http.StatusOK {
		t.Errorf("valid key = %d\n%s", res.StatusCode, b)
	}
	if res, b := apiDo(t, h, http.MethodGet, "/api/v1/media/"+collageKey, ""); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("media without key = %d\n%s", res.StatusCode, b)
	}
}

func TestAPIKeyLifecycle(t *testing.T) {
	a, h, key := apiApp(t)
	if res, _ := apiDo(t, h, http.MethodGet, "/api/v1/collections", key); res.StatusCode != http.StatusOK {
		t.Fatalf("valid key = %d", res.StatusCode)
	}
	ks, err := a.svc.APIKeys(t.Context())
	if err != nil || len(ks) != 1 || ks[0].LastUsedAt.IsZero() || ks[0].Hint != key[:9] {
		t.Fatalf("keys after use = %+v %v", ks, err)
	}
	if err := a.svc.DeleteAPIKey(t.Context(), ks[0].ID); err != nil {
		t.Fatal(err)
	}
	if res, _ := apiDo(t, h, http.MethodGet, "/api/v1/collections", key); res.StatusCode != http.StatusUnauthorized {
		t.Errorf("deleted key = %d, want 401", res.StatusCode)
	}
}

func TestAPISpec(t *testing.T) {
	h := seededApp(t).Handler()
	for _, target := range []string{"/api/openapi.json", "/api/openapi-3.0.json", "/api/openapi.yaml"} {
		res, b := apiDo(t, h, http.MethodGet, target, "")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET %s = %d", target, res.StatusCode)
		}
		if strings.Contains(string(b), "...}") {
			t.Errorf("GET %s: wildcard pattern leaked into spec", target)
		}
		for _, s := range []string{"/api/v1/collections/{id}/listings", "/api/v1/media/{key}", "listCollectionListings", "bearer"} {
			if !strings.Contains(string(b), s) {
				t.Errorf("GET %s: missing %q", target, s)
			}
		}
	}
}

func TestAPICollections(t *testing.T) {
	_, h, key := apiApp(t)

	cs := apiGet[[]APICollection](t, h, key, "/api/v1/collections")
	if len(cs) != 2 || cs[0].ID != "somerville" || cs[0].Search.Location != "Somerville, MA" {
		t.Errorf("collections = %+v", cs)
	}
	days := apiGet[[]APIDay](t, h, key, "/api/v1/collections/somerville/days")
	if len(days) != 2 || days[0] != (APIDay{day1, 3}) || days[1] != (APIDay{day2, 5}) {
		t.Errorf("days = %+v", days)
	}
	if res, _ := apiDo(t, h, http.MethodGet, "/api/v1/collections/nope", key); res.StatusCode != http.StatusNotFound {
		t.Errorf("missing collection = %d", res.StatusCode)
	}
}

func TestAPICollectionListings(t *testing.T) {
	_, h, key := apiApp(t)
	ids := func(v APICollectionListings) string {
		var out []string
		for _, r := range v.Rows {
			out = append(out, r.Listing.SourceID)
		}
		return strings.Join(out, ",")
	}

	v := apiGet[APICollectionListings](t, h, key, "/api/v1/collections/somerville/listings")
	if v.Day != day2 || v.PreviousDay != day1 || v.NextDay != "" || v.Listings != 5 || v.Graded != 4 || v.Hidden != 1 {
		t.Errorf("summary = %+v", v)
	}
	if got := ids(v); got != "z1,z4,z2" {
		t.Errorf("rows = %s", got)
	}
	z1 := v.Rows[0]
	if z1.Rank != 1 || len(z1.Grades) != 2 || z1.Grades[0].Profile != "attic" || z1.Grades[1].Profile != "loft" || z1.Best.Match != 87.5 {
		t.Errorf("z1 grades = %+v", z1)
	}
	if z1.PreviousPriceCents == nil || *z1.PreviousPriceCents != 300000 || z1.Listing.Price.Cents != 290000 || z1.New {
		t.Errorf("z1 price history = %+v", z1)
	}
	if len(z1.Listing.Collages) != 1 || z1.Listing.Collages[0] != "/api/v1/media/"+collageKey || z1.Listing.Href != "/api/v1/listings/zillow/z1" {
		t.Errorf("z1 links = %+v", z1.Listing)
	}
	if len(z1.AlsoListed) != 1 || z1.AlsoListed[0].SourceID != "r1" {
		t.Errorf("z1 also listed = %+v", z1.AlsoListed)
	}

	for target, want := range map[string]string{
		"/api/v1/collections/somerville/listings?dealbreakers=true": "z1,z4,z2,z3",
		"/api/v1/collections/somerville/listings?new_only=true":     "z4",
		"/api/v1/collections/somerville/listings?limit=2":           "z1,z4",
		"/api/v1/collections/somerville/listings?min_match=80":      "z1",
		"/api/v1/collections/somerville/listings?day=" + day1:       "z1,z2",
	} {
		if got := ids(apiGet[APICollectionListings](t, h, key, target)); got != want {
			t.Errorf("GET %s = %s, want %s", target, got, want)
		}
	}

	empty := apiGet[APICollectionListings](t, h, key, "/api/v1/collections/zempty/listings")
	if empty.Day != "" || empty.Rows == nil || len(empty.Rows) != 0 {
		t.Errorf("empty collection = %+v", empty)
	}
}

func TestAPIListing(t *testing.T) {
	_, h, key := apiApp(t)

	v := apiGet[APIListingDetail](t, h, key, "/api/v1/listings/zillow/z1?collection=somerville")
	if v.Day != day2 || v.Listing.Price.Cents != 290000 || len(v.AlsoListed) != 1 || len(v.History) != 2 {
		t.Errorf("detail = %+v", v)
	}
	if len(v.Grades) != 2 || v.Grades[0].ProfileName != "Sunny attic" || len(v.Grades[0].Models) != 1 {
		t.Fatalf("grades = %+v", v.Grades)
	}
	m := v.Grades[0].Models[0]
	if len(m.Wants) != 2 || m.Wants[0].Label != "Skylights" || m.Wants[0].Verdict != "present" || len(m.Avoids) != 1 || !m.Avoids[0].Dealbreaker {
		t.Errorf("model result = %+v", m)
	}
	for _, target := range []string{"/api/v1/listings/zillow/nope", "/api/v1/listings/zillow/z1?collection=nope"} {
		if res, _ := apiDo(t, h, http.MethodGet, target, key); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", target, res.StatusCode)
		}
	}
}

func TestAPIProfiles(t *testing.T) {
	_, h, key := apiApp(t)

	ps := apiGet[[]APIProfileSummary](t, h, key, "/api/v1/profiles")
	if len(ps) != 3 {
		t.Fatalf("profiles = %+v", ps)
	}
	v := apiGet[APIProfileDetail](t, h, key, "/api/v1/profiles/attic")
	if v.Profile.Name != "Sunny attic" || v.Profile.Kind != "want" || len(v.InheritedAvoids) != 1 || v.InheritedAvoids[0].ID != "carpet" {
		t.Errorf("attic = %+v", v)
	}
	if strings.Join(v.Collections, ",") != "somerville,zempty" {
		t.Errorf("attic collections = %v", v.Collections)
	}
}

func TestAPIRunAndJobs(t *testing.T) {
	_, h, key := apiApp(t)

	res, b := apiDo(t, h, http.MethodPost, "/api/v1/collections/somerville/runs", key)
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("run = %d\n%s", res.StatusCode, b)
	}
	var j APIJob
	if err := json.Unmarshal(b, &j); err != nil {
		t.Fatal(err)
	}
	if j.ID == 0 || j.Kind != "run_collection" || j.CollectionID != "somerville" || j.Status != "queued" || j.StartedAt != nil {
		t.Errorf("job = %+v", j)
	}
	if res, _ := apiDo(t, h, http.MethodPost, "/api/v1/collections/nope/runs", key); res.StatusCode != http.StatusNotFound {
		t.Errorf("run missing collection = %d", res.StatusCode)
	}

	js := apiGet[[]APIJob](t, h, key, "/api/v1/jobs?collection=somerville")
	if len(js) != 1 || js[0].ID != j.ID {
		t.Errorf("jobs = %+v", js)
	}
	if got := apiGet[APIJob](t, h, key, "/api/v1/jobs/"+strconv.FormatInt(j.ID, 10)); got.ID != j.ID {
		t.Errorf("job = %+v", got)
	}
	if es := apiGet[[]APIJobEvent](t, h, key, "/api/v1/jobs/"+strconv.FormatInt(j.ID, 10)+"/events"); es == nil {
		t.Errorf("events should be an empty list, got null")
	}
	if res, _ := apiDo(t, h, http.MethodGet, "/api/v1/jobs/999", key); res.StatusCode != http.StatusNotFound {
		t.Errorf("missing job = %d", res.StatusCode)
	}
}

func TestAPIMedia(t *testing.T) {
	_, h, key := apiApp(t)

	for _, target := range []string{"/api/v1/media/" + collageKey, "/api/v1/media/" + url.PathEscape(collageKey)} {
		res, b := apiDo(t, h, http.MethodGet, target, key)
		if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/jpeg" || len(b) == 0 {
			t.Errorf("GET %s = %d %q", target, res.StatusCode, res.Header.Get("Content-Type"))
		}
	}
	for _, m := range []string{"photos/ab/page.html", "collages/missing", "etc/passwd"} {
		if res, _ := apiDo(t, h, http.MethodGet, "/api/v1/media/"+m, key); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET media %s = %d, want 404", m, res.StatusCode)
		}
	}
}
