package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/jobs"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

func profileApp(t *testing.T) *App {
	t.Helper()
	a := seededApp(t)
	ctx := context.Background()
	db, err := a.svc.OpenStore(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	p, err := db.Profile(ctx, "attic")
	if err != nil {
		t.Fatal(err)
	}
	p.Notes = []string{"skylights", "top floor"}
	p.Summary = "Top floor <script>alert(1)</script> with skylights."
	p.Want[0].NotThis = "Solar tubes"
	p.Want[0].Keywords = []string{"skylight", "roof window"}
	p.Avoid = []profile.Criterion{{ID: "basement", Label: "Basement unit", LookFor: "Below grade", Importance: profile.Low, Evidence: profile.EvidencePhotos}}
	p.References[0].Collages = []string{collageKey}
	p.Drafted = &profile.Drafted{Model: "test/draft", At: time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC), CostUSD: 0.02}
	if err := db.SaveProfile(ctx, p); err != nil {
		t.Fatal(err)
	}
	eff, err := db.EffectiveProfile(ctx, "attic")
	if err != nil {
		t.Fatal(err)
	}
	ref := rental(listing.SourceZillow, "ref", "1 Reference Rd", 400000)
	ref.Collages = []string{collageKey}
	if err := db.SaveReferenceListings(ctx, "attic", []listing.Listing{assessed(ref, eff, 80)}); err != nil {
		t.Fatal(err)
	}
	return a
}

func validProfileForm() url.Values {
	return url.Values{
		"url":            {" https://www.zillow.com/homedetails/1-Main-St/123_zpid/ "},
		"id":             {"cozy"},
		"kind":           {"avoid"},
		"name":           {" Cozy "},
		"notes":          {"low ceilings\r\n\r\n  dark kitchen  \r\n"},
		"model":          {""},
		"max_charge_usd": {"0.25"},
		"draft":          {"1"},
	}
}

func jobParams(t *testing.T, a *App, loc string, into any) jobs.Job {
	t.Helper()
	id, err := strconv.ParseInt(strings.TrimPrefix(loc, "/jobs/"), 10, 64)
	if err != nil {
		t.Fatalf("redirect %q is not a job", loc)
	}
	j, err := a.jobs.Job(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(j.Params, into); err != nil {
		t.Fatal(err)
	}
	return j
}

func TestProfilesPage(t *testing.T) {
	a := profileApp(t)
	res, body := get(t, a.Handler(), "/profiles")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /profiles = %d", res.StatusCode)
	}
	checkBody(t, "list", body, []string{
		"<title>Profiles · housing</title>", `<a href="/profiles" aria-current="page">Profiles</a>`,
		`<a class="button" href="/profiles/new">New profile</a>`,
		`<td><a href="/profiles/attic"><code>attic</code></a></td>`,
		`<td><span class="kind want">want</span></td>`, `<td><span class="kind avoid">avoid</span></td>`,
		`<td class="num">2</td>
  <td class="num">1</td>
  <td class="num">1</td>`,
		`<a href="/c/somerville">somerville</a>, <a href="/c/zempty">zempty</a>`,
		`<td class="tags">every collection</td>`,
		`<td class="nowrap">draft · 2026-09-01`,
	}, []string{"There are no profiles yet"})

	empty, _ := newApp(t)
	_, body = get(t, empty.Handler(), "/profiles")
	checkBody(t, "empty", body, []string{"There are no profiles yet", `<a href="/profiles/new">Create one</a>`}, []string{"<table"})
}

func TestProfilePage(t *testing.T) {
	a := profileApp(t)
	h := a.Handler()
	runJobs(t, a.jobs.(*jobs.Queue), &fakeExec{}, jobs.DraftProfileParams{ProfileID: "attic"})

	res, body := get(t, h, "/profiles/attic")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /profiles/attic = %d\n%s", res.StatusCode, body)
	}
	checkBody(t, "attic", body, []string{
		"<title>Sunny attic · housing</title>",
		`<a class="button" href="/profiles/attic/edit">Edit</a>`,
		`Latest job: <span class="status failed">failed</span> Draft profile just now · <a href="/jobs/1">view</a>`,
		`<dd><a href="/c/somerville">somerville</a>, <a href="/c/zempty">zempty</a></dd>`,
		`<dd>draft · 2026-09-01`, `$0.020`,
		`Top floor &lt;script&gt;alert(1)&lt;/script&gt; with skylights.`,
		`<li>skylights</li><li>top floor</li>`,
		`<a href="https://example.com/ref" target="_blank" rel="noopener">1 Reference Rd, Somerville, MA 02144</a>`,
		`<span class="tags">model 80.0</span>`,
		`<img src="/media/collages/ab/abcdef" alt="Reference collage 0"`,
		`<h2>Want criteria</h2>`,
		`<td>Skylights <span class="muted">skylights</span></td><td class="tags">high</td><td class="tags">either</td><td class="summary">Skylights</td><td class="summary">Solar tubes</td><td class="tags">skylight, roof window</td>`,
		`<td>Basement unit <span class="muted">basement</span></td><td class="tags">low</td><td class="tags">photos</td>`,
		`Inherited from <a href="/profiles/corp">Corporate</a> <span class="muted">corp · read-only</span>`,
		`<td>Wall-to-wall carpet <span class="muted">carpet</span></td><td class="tags"><span class="bad">essential</span></td>`,
		`<form method="post" action="/profiles/attic/draft" class="form">`,
		"<textarea id=\"f-draft-notes\" name=\"notes\" rows=\"4\">\nskylights\ntop floor</textarea>",
		`placeholder="` + profile.DefaultDraftModel + `"`,
	}, []string{"<script>alert"})

	_, body = get(t, h, "/profiles/corp")
	checkBody(t, "corp", body, []string{
		`<span class="kind avoid">avoid</span>`, `<dd>applied to every collection</dd>`, `<dd>never</dd>`,
		"No reference listings.", "This profile has no reference listings to draft from.", "Wall-to-wall carpet",
	}, []string{"Want criteria", "Inherited from", "Latest job"})

	_, body = get(t, h, "/profiles/loft")
	checkBody(t, "loft", body, []string{"Inherited from", `<dd><a href="/c/somerville">somerville</a></dd>`, "No reference listings."}, nil)

	for _, target := range []string{"/profiles/ghost", "/profiles/ghost/edit"} {
		if res, _ := get(t, h, target); res.StatusCode != http.StatusNotFound {
			t.Errorf("GET %s = %d", target, res.StatusCode)
		}
	}
}

func TestProfileLinks(t *testing.T) {
	a := profileApp(t)
	h := a.Handler()
	_, body := get(t, h, "/c/somerville")
	checkBody(t, "listings", body, []string{`<a href="/profiles/attic"><code>attic</code></a>`, `<a href="/profiles/loft"><code>loft</code></a>`}, nil)
	_, body = get(t, h, "/listing/zillow/z1?collection=somerville")
	checkBody(t, "listing", body, []string{`<h2><a href="/profiles/attic">Sunny attic</a>`, `<h2><a href="/profiles/loft">Loft</a>`}, nil)
}

func TestNewProfileForm(t *testing.T) {
	a := profileApp(t)
	res, body := get(t, a.Handler(), "/profiles/new")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET /profiles/new = %d", res.StatusCode)
	}
	checkBody(t, "new", body, []string{
		"<title>New profile · housing</title>", `<a href="/profiles" aria-current="page">Profiles</a>`,
		`<form method="post" action="/profiles" class="form">`,
		`<input id="f-url" name="url" type="url" value="" required`,
		`<input id="f-id" name="id" value="" required`,
		`<input type="radio" name="kind" value="want" checked> want`, `<input type="radio" name="kind" value="avoid"> avoid`,
		`<input type="checkbox" name="draft" value="1" checked> Draft criteria`,
		`<input type="checkbox" name="force" value="1"> Overwrite`,
		`placeholder="` + profile.DefaultDraftModel + `"`, `placeholder="0.10"`,
	}, nil)
}

func TestCreateProfile(t *testing.T) {
	a := profileApp(t)
	h := a.Handler()

	res, _ := post(t, h, "/profiles", validProfileForm(), sameOriginHeader)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("POST /profiles = %d", res.StatusCode)
	}
	var got jobs.CreateProfileParams
	j := jobParams(t, a, res.Header.Get("Location"), &got)
	want := jobs.CreateProfileParams{
		URL: "https://www.zillow.com/homedetails/1-Main-St/123_zpid/", ID: "cozy", ProfileKind: profile.KindAvoid, Name: "Cozy",
		Notes: []string{"low ceilings", "dark kitchen"}, Model: profile.DefaultDraftModel, MaxChargeUSD: 0.25,
	}
	if j.Kind != string(jobs.KindCreateProfile) || j.ProfileID != "cozy" || j.Status != jobs.StatusQueued || !reflect.DeepEqual(got, want) {
		t.Errorf("job %+v params\n%+v\nwant\n%+v", j, got, want)
	}

	form := with(validProfileForm(), "id", "attic", "force", "1", "draft", "\x00", "model", "test/x", "max_charge_usd", "", "kind", "want", "notes", "")
	res, _ = post(t, h, "/profiles", form, sameOriginHeader)
	if res.StatusCode != http.StatusSeeOther {
		t.Fatalf("overwrite = %d", res.StatusCode)
	}
	got = jobs.CreateProfileParams{}
	jobParams(t, a, res.Header.Get("Location"), &got)
	want = jobs.CreateProfileParams{
		URL: "https://www.zillow.com/homedetails/1-Main-St/123_zpid/", ID: "attic", ProfileKind: profile.KindWant, Name: "Cozy",
		NoDraft: true, Force: true, MaxChargeUSD: service.DefaultLookupMaxChargeUSD,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("overwrite params\n%+v\nwant\n%+v", got, want)
	}
}

func TestCreateProfileFromEachSource(t *testing.T) {
	a := profileApp(t)
	h := a.Handler()
	_, body := get(t, h, "/profiles/new")
	checkBody(t, "form", body, []string{"on Zillow, Redfin, Craigslist or Facebook Marketplace."}, []string{"Only Zillow"})
	for i, u := range []string{
		"https://www.redfin.com/MA/Somerville/91-Heath-St-02145/home/8714201",
		"https://boston.craigslist.org/gbs/apa/d/somerville-sunny-2br/7881234567.html",
		"https://www.facebook.com/marketplace/item/1071136835630251/?ref=search",
	} {
		id := "ref-" + strconv.Itoa(i)
		res, body := post(t, h, "/profiles", with(validProfileForm(), "url", u, "id", id), sameOriginHeader)
		if res.StatusCode != http.StatusSeeOther {
			t.Errorf("%s: POST /profiles = %d\n%s", u, res.StatusCode, body)
			continue
		}
		var got jobs.CreateProfileParams
		j := jobParams(t, a, res.Header.Get("Location"), &got)
		if j.Kind != string(jobs.KindCreateProfile) || j.Status != jobs.StatusQueued || got.URL != u || got.ID != id {
			t.Errorf("%s: job %+v params %+v", u, j, got)
		}
	}
}

func TestCreateProfileErrors(t *testing.T) {
	a := profileApp(t)
	h := a.Handler()
	for _, tc := range []struct {
		name     string
		form     url.Values
		contains []string
	}{
		{"missing url", with(validProfileForm(), "url", ""), []string{`<p class="field-error">a listing URL is required</p>`}},
		{"bad url", with(validProfileForm(), "url", "zillow.com/x"), []string{
			`<p class="field-error">invalid listing URL &#34;zillow.com/x&#34;: want an http or https link</p>`, `name="url" type="url" value="zillow.com/x"`,
		}},
		{"unsupported host", with(validProfileForm(), "url", "https://www.trulia.com/x"), []string{`<p class="field-error">no lookup available for &#34;www.trulia.com&#34;: use a listing from Zillow, Redfin, Craigslist or Facebook Marketplace</p>`}},
		{"search page", with(validProfileForm(), "url", "https://www.redfin.com/zipcode/02144"), []string{`<p class="field-error">not a redfin listing page: &#34;https://www.redfin.com/zipcode/02144&#34;`}},
		{"bad id", with(validProfileForm(), "id", "Bad ID"), []string{
			`<p class="field-error">invalid profile id &#34;Bad ID&#34;: use lowercase letters, digits and dashes</p>`, `name="id" value="Bad ID"`,
		}},
		{"existing id", with(validProfileForm(), "id", "attic"), []string{`<p class="field-error">profile already exists: attic</p>`}},
		{"bad kind", with(validProfileForm(), "kind", "maybe"), []string{`<p class="field-error">invalid kind &#34;maybe&#34;: use want or avoid</p>`}},
		{"bad charge", with(validProfileForm(), "max_charge_usd", "0", "id", "Bad"), []string{
			`<p class="field-error">max Apify charge must be a positive amount</p>`, `name="max_charge_usd" type="number" min="0.01" step="0.01" value="0"`,
			`<p class="field-error">invalid profile id`,
		}},
	} {
		res, body := post(t, h, "/profiles", tc.form, sameOriginHeader)
		if res.StatusCode != http.StatusUnprocessableEntity {
			t.Errorf("%s: status %d", tc.name, res.StatusCode)
			continue
		}
		checkBody(t, tc.name, body, append(tc.contains,
			`<div class="errors" role="alert">`, `name="name" value=" Cozy "`,
			"name=\"notes\" rows=\"4\">\nlow ceilings\r\n\r\n  dark kitchen  \r\n</textarea>",
			`<input type="checkbox" name="draft" value="1" checked>`,
		), nil)
	}
	if js, _ := a.jobs.Jobs(context.Background(), jobs.Filter{}); len(js) != 0 {
		t.Errorf("invalid forms must not enqueue: %d jobs", len(js))
	}
}

func TestEditProfile(t *testing.T) {
	a := profileApp(t)
	h := a.Handler()
	ctx := context.Background()
	before, _ := a.svc.Profile(ctx, "attic")

	res, body := get(t, h, "/profiles/attic/edit")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("GET edit = %d", res.StatusCode)
	}
	checkBody(t, "edit", body, []string{
		"<title>Edit attic · housing</title>", `<form method="post" action="/profiles/attic" class="form">`,
		"regraded on the next run",
		`<input id="f-name" name="name" value="Sunny attic">`,
		"name=\"summary\" rows=\"4\">\nTop floor &lt;script&gt;alert(1)&lt;/script&gt; with skylights.</textarea>",
		"name=\"notes\" rows=\"4\">\nskylights\ntop floor</textarea>",
		`<input type="hidden" name="criteria" value="skylights"><input name="label_skylights" value="Skylights"`,
		`<select name="importance_skylights" aria-label="Importance of skylights"><option value="essential">essential</option><option value="high" selected>high</option>`,
		`<input type="checkbox" name="remove_floors" value="1"> remove`,
		`name="label_basement" value="Basement unit"`,
	}, []string{"<script>alert", "carpet"})

	form := url.Values{
		"name":            {"Bright attic"},
		"summary":         {"Top floor.\r\nLots of light."},
		"notes":           {"skylights\r\nquiet street"},
		"criteria":        {"skylights", "floors", "basement"},
		"label_skylights": {"Big skylights"}, "importance_skylights": {"essential"},
		"label_floors": {"Wood floors"}, "importance_floors": {"medium"}, "remove_floors": {"1"},
		"label_basement": {"Basement unit"}, "importance_basement": {"high"},
	}
	res, _ = post(t, h, "/profiles/attic", form, sameOriginHeader)
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/profiles/attic" {
		t.Fatalf("POST edit = %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	after, _ := a.svc.Profile(ctx, "attic")
	want := before
	want.Name, want.Summary, want.Notes = "Bright attic", "Top floor.\nLots of light.", []string{"skylights", "quiet street"}
	want.Want = []profile.Criterion{before.Want[0]}
	want.Want[0].Label, want.Want[0].Importance = "Big skylights", profile.Essential
	want.Avoid = []profile.Criterion{before.Avoid[0]}
	want.Avoid[0].Importance = profile.High
	if !reflect.DeepEqual(after, want) {
		t.Errorf("saved\n%+v\nwant\n%+v", after, want)
	}
	if after.Hash() == before.Hash() {
		t.Error("edits should change the profile hash")
	}
	_, body = get(t, h, "/profiles/attic")
	checkBody(t, "after edit", body, []string{
		`<img src="/media/collages/ab/abcdef"`, "not graded against the current profile", "<dd>draft · 2026-09-01",
	}, []string{"model 80.0"})

	bad := with(form, "importance_skylights", "huge", "label_basement", " ")
	res, body = post(t, h, "/profiles/attic", bad, sameOriginHeader)
	if res.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("invalid importance = %d", res.StatusCode)
	}
	checkBody(t, "invalid", body, []string{
		`<div class="errors" role="alert">`, `<p class="field-error">invalid importance &#34;huge&#34;</p>`,
		`<option value="huge" selected>huge</option>`, `<p class="field-error">a label is required</p>`,
		`name="label_skylights" value="Big skylights"`, `name="label_basement" value=" "`,
		`<input id="f-name" name="name" value="Bright attic">`,
	}, nil)
	res, body = post(t, h, "/profiles/attic", with(bad, "remove_basement", "1"), sameOriginHeader)
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, `<input type="checkbox" name="remove_basement" value="1" checked> remove`) {
		t.Errorf("remove should be preserved on error: %d", res.StatusCode)
	}
	if again, _ := a.svc.Profile(ctx, "attic"); !reflect.DeepEqual(again, after) {
		t.Error("an invalid edit must not save")
	}

	res, body = post(t, h, "/profiles/attic", with(form, "criteria", "ghost"), sameOriginHeader)
	if res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "criterion &#34;ghost&#34; is no longer part of this profile") {
		t.Errorf("stale criterion = %d", res.StatusCode)
	}
	if res, _ := post(t, h, "/profiles/ghost", form, sameOriginHeader); res.StatusCode != http.StatusNotFound {
		t.Errorf("POST unknown = %d", res.StatusCode)
	}
}

func TestRedraftProfile(t *testing.T) {
	a := profileApp(t)
	h := a.Handler()
	for _, tc := range []struct {
		name string
		form url.Values
		want jobs.DraftProfileParams
	}{
		{"unchanged notes", url.Values{"notes": {"skylights\r\ntop floor\r\n"}, "model": {""}}, jobs.DraftProfileParams{ProfileID: "attic", Model: profile.DefaultDraftModel}},
		{"new notes", url.Values{"notes": {"skylights\r\nnear the T"}, "model": {"test/x"}}, jobs.DraftProfileParams{ProfileID: "attic", Model: "test/x", Notes: []string{"skylights", "near the T"}}},
		{"cleared notes", url.Values{"notes": {"  "}}, jobs.DraftProfileParams{ProfileID: "attic", Model: profile.DefaultDraftModel, Notes: []string{}}},
	} {
		res, _ := post(t, h, "/profiles/attic/draft", tc.form, sameOriginHeader)
		if res.StatusCode != http.StatusSeeOther {
			t.Errorf("%s: status %d", tc.name, res.StatusCode)
			continue
		}
		var got jobs.DraftProfileParams
		j := jobParams(t, a, res.Header.Get("Location"), &got)
		if j.Kind != string(jobs.KindDraftProfile) || j.ProfileID != "attic" || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: job %+v params %#v, want %#v", tc.name, j, got, tc.want)
		}
	}
	if res, body := post(t, h, "/profiles/corp/draft", url.Values{}, sameOriginHeader); res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "no reference listings") {
		t.Errorf("draft without references = %d", res.StatusCode)
	}
	if res, _ := post(t, h, "/profiles/ghost/draft", url.Values{}, sameOriginHeader); res.StatusCode != http.StatusNotFound {
		t.Errorf("draft unknown = %d", res.StatusCode)
	}
}

func TestProfileFormGuards(t *testing.T) {
	a := profileApp(t)
	h := a.Handler()
	before, _ := a.svc.Profile(context.Background(), "attic")
	evil := map[string]string{"Origin": "https://evil.example"}
	for _, tc := range []struct {
		target string
		form   url.Values
	}{
		{"/profiles", validProfileForm()},
		{"/profiles/attic", url.Values{"name": {"pwned"}}},
		{"/profiles/attic/draft", url.Values{"notes": {"pwned"}}},
	} {
		if res, _ := post(t, h, tc.target, tc.form, evil); res.StatusCode != http.StatusForbidden {
			t.Errorf("cross-site POST %s = %d", tc.target, res.StatusCode)
		}
	}
	big := with(validProfileForm(), "notes", strings.Repeat("x", maxFormBytes))
	if res, _ := post(t, h, "/profiles", big, sameOriginHeader); res.StatusCode != http.StatusBadRequest {
		t.Errorf("oversized form = %d", res.StatusCode)
	}
	if js, _ := a.jobs.Jobs(context.Background(), jobs.Filter{}); len(js) != 0 {
		t.Errorf("rejected requests must not enqueue: %d", len(js))
	}
	if after, _ := a.svc.Profile(context.Background(), "attic"); !reflect.DeepEqual(after, before) {
		t.Error("rejected requests must not save")
	}
}
