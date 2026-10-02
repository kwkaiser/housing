package app

import (
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"testing/synctest"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func intp(n int) *int {
	return &n
}

func validForm() url.Values {
	return url.Values{
		"id":               {"cambridge"},
		"mode":             {"buy"},
		"sources":          {"redfin", "zillow"},
		"profiles":         {"attic", "loft"},
		"order_attic":      {"2"},
		"order_loft":       {"1"},
		"location":         {"Cambridge, MA"},
		"radius_miles":     {"2.5"},
		"min_price":        {""},
		"max_price":        {"900000"},
		"min_beds":         {"0"},
		"max_beds":         {""},
		"max_age_days":     {"14"},
		"amenities":        {"parking", "dishwasher"},
		"limit":            {"40"},
		"model":            {" test/other "},
		"max_run_cost_usd": {"2.5"},
		"schedule":         {"07:00"},
	}
}

func with(f url.Values, kv ...string) url.Values {
	out := url.Values{}
	for k, v := range f {
		out[k] = v
	}
	for i := 0; i < len(kv); i += 2 {
		if kv[i+1] == "\x00" {
			out.Del(kv[i])
		} else {
			out.Set(kv[i], kv[i+1])
		}
	}
	return out
}

func checkBody(t *testing.T, label, body string, contains, missing []string) {
	t.Helper()
	for _, s := range contains {
		if !strings.Contains(body, s) {
			t.Errorf("%s: body missing %q", label, s)
		}
	}
	for _, s := range missing {
		if strings.Contains(body, s) {
			t.Errorf("%s: body should not contain %q", label, s)
		}
	}
}

func TestCollectionsPage(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		a := seededApp(t)
		ctx := t.Context()
		c, err := a.svc.Collection(ctx, "zempty")
		if err != nil {
			t.Fatal(err)
		}
		c.Schedule, c.MaxRunCostUSD = "07:00", 2.5
		if err := a.svc.SaveCollection(ctx, c); err != nil {
			t.Fatal(err)
		}
		runJobs(t, a.jobs.(*jobs.Queue), &fakeExec{}, jobs.RunCollectionParams{CollectionID: "somerville"})

		res, body := get(t, a.Handler(), "/collections")
		if res.StatusCode != http.StatusOK {
			t.Fatalf("GET /collections = %d", res.StatusCode)
		}
		checkBody(t, "list", body, []string{
			"<title>Collections · housing</title>",
			`<a class="button" href="/collections/new">New collection</a>`,
			`<td><a href="/c/somerville">somerville</a></td>`,
			`<td class="tags">attic, loft</td>`,
			`<td class="nowrap">manual</td>`, `<td class="nowrap">daily at 07:00</td>`,
			`<td class="num">$1.00 <span class="muted">default</span></td>`, `<td class="num">$2.50</td>`,
			`<span class="status succeeded">succeeded</span>`, `<a href="/jobs/1">view</a>`, `<span class="muted">never</span>`,
			`<a class="button" href="/collections/zempty/edit">Edit</a>`,
			`<form method="post" action="/jobs/run"><input type="hidden" name="collection" value="zempty"><button type="submit">Run now</button></form>`,
		}, []string{"There are no collections yet"})
	})
}

func TestNewCollectionForm(t *testing.T) {
	a := seededApp(t)
	res, body := get(t, a.Handler(), "/collections/new")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /collections/new = %d", res.StatusCode)
	}
	checkBody(t, "new", body, []string{
		"<title>New collection · housing</title>",
		`<form method="post" action="/collections" class="form">`,
		`<input id="f-id" name="id" value=""`,
		`<input type="radio" name="mode" value="rent" checked> rent`, `<input type="radio" name="mode" value="buy"> buy`,
		`<input type="checkbox" name="sources" value="zillow" checked> zillow`,
		`<input type="checkbox" name="sources" value="redfin"> redfin`,
		`<input type="checkbox" name="sources" value="craigslist"> craigslist`,
		`<input type="checkbox" name="sources" value="facebook"> facebook`,
		`name="profiles" value="attic">`, `name="profiles" value="loft">`,
		`<code>corp</code> Corporate <span class="tags">always applied</span>`,
		`name="limit" type="number" min="0" step="1" value="50"`,
		`name="max_price" type="number" min="0" step="1" value=""`,
		`<input type="checkbox" name="amenities" value="in_unit_laundry"> in unit laundry`,
		`placeholder="` + profile.DefaultAssessModel + `"`, `placeholder="1.00"`,
		`<input id="f-schedule" name="schedule" type="time" value="">`,
		"Create collection",
	}, []string{`value="realtor"`, `value="corp"`})
}

func TestCreateCollection(t *testing.T) {
	a := seededApp(t)
	h := a.Handler()

	res, _ := post(t, h, "/collections", validForm(), sameOriginHeader)
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/collections" {
		t.Fatalf("POST /collections = %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	got, err := a.svc.Collection(t.Context(), "cambridge")
	if err != nil {
		t.Fatal(err)
	}
	want := collection.Collection{
		ID: "cambridge", Mode: profile.ModeBuy, Sources: []listing.Source{listing.SourceRedfin, listing.SourceZillow},
		Profiles: []string{"loft", "attic"},
		Search: profile.Search{
			Location: "Cambridge, MA", RadiusMiles: 2.5, MaxPrice: intp(900000), MinBeds: intp(0), MaxAgeDays: 14,
			Amenities: []listing.Amenity{listing.AmenityParking, listing.AmenityDishwasher}, Limit: 40,
		},
		Model: "test/other", MaxRunCostUSD: 2.5, Schedule: "07:00",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("saved\n%+v\nwant\n%+v", got, want)
	}

	res, body := post(t, h, "/collections", with(validForm(), "id", "somerville"), sameOriginHeader)
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "collection already exists: somerville") {
		t.Errorf("duplicate id = %d\n%s", res.StatusCode, body)
	}
	if c, _ := a.svc.Collection(t.Context(), "somerville"); c.Mode != profile.ModeRent || c.Search.Location != "Somerville, MA" {
		t.Errorf("duplicate create overwrote the collection: %+v", c)
	}

	res, _ = post(t, h, "/collections", with(validForm(), "id", "unordered", "order_attic", "", "order_loft", ""), sameOriginHeader)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("unordered create = %d", res.StatusCode)
	}
	if c, _ := a.svc.Collection(t.Context(), "unordered"); !reflect.DeepEqual(c.Profiles, []string{"attic", "loft"}) {
		t.Errorf("profiles without an order keep form order: %v", c.Profiles)
	}
}

func TestEditCollection(t *testing.T) {
	a := seededApp(t)
	h := a.Handler()
	ctx := t.Context()
	c, _ := a.svc.Collection(ctx, "somerville")
	c.Search.MaxPrice, c.Search.MinBeds, c.Search.Amenities = intp(4000), intp(1), []listing.Amenity{listing.AmenityParking}
	c.Sources, c.Model, c.MaxRunCostUSD = []listing.Source{listing.SourceZillow, listing.SourceFacebook}, "test/m", 1
	if err := a.svc.SaveCollection(ctx, c); err != nil {
		t.Fatal(err)
	}

	res, body := get(t, h, "/collections/somerville/edit")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET edit = %d", res.StatusCode)
	}
	checkBody(t, "edit", body, []string{
		"<title>Edit somerville · housing</title>",
		`<form method="post" action="/collections/somerville" class="form">`,
		`<input id="f-id" value="somerville" disabled>`,
		`<input type="radio" name="mode" value="rent" checked>`,
		`name="sources" value="zillow" checked>`, `name="sources" value="facebook" checked>`, `name="sources" value="redfin">`,
		`name="profiles" value="attic" checked>`, `name="order_attic" value="1"`, `name="order_loft" value="2"`,
		`name="location" value="Somerville, MA"`, `name="max_price" type="number" min="0" step="1" value="4000"`,
		`name="min_beds" type="number" min="0" step="1" value="1"`, `name="min_price" type="number" min="0" step="1" value=""`,
		`name="amenities" value="parking" checked>`, `name="model" value="test/m"`, `name="max_run_cost_usd" type="number" min="0" step="0.01" value="1"`,
		"Save changes",
	}, []string{`name="id"`})
	if strings.Index(body, `value="attic"`) > strings.Index(body, `value="loft"`) {
		t.Error("checked profiles should be listed in collection order")
	}

	form := with(validForm(), "id", "renamed", "mode", "rent", "order_attic", "1", "order_loft", "2")
	res, _ = post(t, h, "/collections/somerville", form, sameOriginHeader)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST update = %d", res.StatusCode)
	}
	got, err := a.svc.Collection(ctx, "somerville")
	if err != nil {
		t.Fatal(err)
	}
	if got.Mode != profile.ModeRent || got.Search.Location != "Cambridge, MA" || got.Schedule != "07:00" || !reflect.DeepEqual(got.Profiles, []string{"attic", "loft"}) ||
		got.Search.MinPrice != nil || *got.Search.MaxPrice != 900000 || got.Model != "test/other" {
		t.Errorf("updated = %+v", got)
	}
	if _, err := a.svc.Collection(ctx, "renamed"); err == nil {
		t.Error("an update must not create a collection under the posted id")
	}

	res, _ = post(t, h, "/collections/somerville", with(validForm(), "schedule", "", "max_run_cost_usd", "", "model", ""), sameOriginHeader)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST clear = %d", res.StatusCode)
	}
	if got, _ := a.svc.Collection(ctx, "somerville"); got.Schedule != "" || got.MaxRunCostUSD != 0 || got.Model != "" {
		t.Errorf("cleared = %+v", got)
	}

	for _, target := range []string{"/collections/nope/edit"} {
		if res, _ := get(t, h, target); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d", target, res.StatusCode)
		}
	}
	if res, _ := post(t, h, "/collections/nope", validForm(), sameOriginHeader); res.StatusCode != http.StatusNotFound {
		t.Errorf("POST unknown = %d", res.StatusCode)
	}

	_, body = get(t, h, "/c/somerville")
	checkBody(t, "listings", body, []string{`<a class="button" href="/collections/somerville/edit">Edit collection</a>`}, nil)
}

func TestCollectionFormErrors(t *testing.T) {
	a := seededApp(t)
	h := a.Handler()

	for _, tc := range []struct {
		name     string
		target   string
		form     url.Values
		contains []string
	}{
		{"missing location", "/collections", with(validForm(), "location", ""), []string{
			`<div class="field invalid">
  <label for="f-location">Location</label>`, `<p class="field-error">a search location is required</p>`,
			`name="max_price" type="number" min="0" step="1" value="900000"`,
		}},
		{"no profiles", "/collections", with(validForm(), "profiles", "\x00"), []string{`<p class="field-error">at least one profile is required</p>`}},
		{"no sources", "/collections", with(validForm(), "sources", "\x00"), []string{`<p class="field-error">at least one source is required</p>`}},
		{"bad schedule", "/collections", with(validForm(), "schedule", "7am"), []string{`<p class="field-error">invalid schedule &#34;7am&#34;: want a 24-hour HH:MM time</p>`, `name="schedule" type="time" value="7am"`}},
		{"min above max", "/collections/somerville", with(validForm(), "min_price", "5000", "max_price", "4000"), []string{`<p class="field-error">min price 5000 is above max price 4000</p>`, `value="5000"`}},
		{"bad radius", "/collections", with(validForm(), "radius_miles", "far", "location", ""), []string{
			`<p class="field-error">radius must be a number</p>`, `name="radius_miles" type="number" min="0" step="any" value="far"`,
			`<p class="field-error">a search location is required</p>`,
		}},
		{"bad order", "/collections", with(validForm(), "order_attic", "first"), []string{`<p class="field-error">order for attic must be a whole number</p>`, `name="order_attic" value="first"`, `name="profiles" value="attic" checked>`}},
		{"bad id", "/collections", with(validForm(), "id", "Bad ID"), []string{`<p class="field-error">invalid collection id &#34;Bad ID&#34;: use lowercase letters, digits and dashes</p>`, `name="id" value="Bad ID"`}},
		{"avoid profile", "/collections", with(validForm(), "profiles", "corp"), []string{`<p class="field-error">profile &#34;corp&#34; is an avoid profile; avoid profiles apply to every collection automatically</p>`}},
		{"unknown profile", "/collections", with(validForm(), "profiles", "ghost"), []string{`<p class="field-error">profile not found: ghost</p>`}},
		{"unsupported source", "/collections", with(validForm(), "sources", "realtor"), []string{`<p class="field-error">unsupported source &#34;realtor&#34;</p>`}},
		{"negative budget", "/collections", with(validForm(), "max_run_cost_usd", "-1"), []string{`<p class="field-error">max run cost must not be negative</p>`}},
	} {
		res, body := post(t, h, tc.target, tc.form, sameOriginHeader)
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", tc.name, res.StatusCode)
			continue
		}
		checkBody(t, tc.name, body, append(tc.contains, `<div class="errors" role="alert">`, `name="location" value="`+tc.form.Get("location")+`"`), nil)
	}
	if _, err := a.svc.Collection(t.Context(), "cambridge"); err == nil {
		t.Error("invalid forms must not save")
	}
	if c, _ := a.svc.Collection(t.Context(), "somerville"); c.Search.Location != "Somerville, MA" {
		t.Errorf("invalid update saved: %+v", c)
	}
}

func TestCollectionFormGuards(t *testing.T) {
	a := seededApp(t)
	h := a.Handler()
	if res, _ := post(t, h, "/collections", validForm(), map[string]string{"Origin": "https://evil.example"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site create = %d", res.StatusCode)
	}
	if res, _ := post(t, h, "/collections/somerville", validForm(), nil); res.StatusCode != http.StatusForbidden {
		t.Errorf("update without origin = %d", res.StatusCode)
	}
	big := with(validForm(), "location", strings.Repeat("x", maxFormBytes))
	if res, _ := post(t, h, "/collections", big, sameOriginHeader); res.StatusCode != http.StatusBadRequest {
		t.Errorf("oversized form = %d", res.StatusCode)
	}
	if _, err := a.svc.Collection(t.Context(), "cambridge"); err == nil {
		t.Error("rejected requests must not save")
	}
}
