package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

type bearer struct {
	token string
	next  http.RoundTripper
}

func (b bearer) RoundTrip(r *http.Request) (*http.Response, error) {
	r = r.Clone(r.Context())
	r.Header.Set("Authorization", "Bearer "+b.token)
	return b.next.RoundTrip(r)
}

func mcpSession(t *testing.T, srv *httptest.Server, token string) (*mcp.ClientSession, error) {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	transport := &mcp.StreamableClientTransport{
		Endpoint:   srv.URL + "/api/mcp",
		HTTPClient: &http.Client{Transport: bearer{token: token, next: http.DefaultTransport}},
	}
	cs, err := client.Connect(t.Context(), transport, nil)
	if err == nil {
		t.Cleanup(func() { cs.Close() })
	}
	return cs, err
}

func callTool[T any](t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) (T, *mcp.CallToolResult) {
	t.Helper()
	var out T
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if res.IsError {
		t.Fatalf("%s: tool error %v", name, res.Content)
	}
	b, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("%s: %v\n%s", name, err, b)
	}
	return out, res
}

func TestMCP(t *testing.T) {
	a, h, key := apiApp(t)
	srv := httptest.NewServer(h)
	defer srv.Close()

	if _, err := mcpSession(t, srv, "hk_wrong"); err == nil {
		t.Fatal("connecting with a bad key should fail")
	}
	res, err := http.Post(srv.URL+"/api/mcp", "application/json", strings.NewReader(`{}`))
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized || res.Header.Get("WWW-Authenticate") == "" {
		t.Errorf("no key = %d %v", res.StatusCode, res.Header)
	}

	cs, err := mcpSession(t, srv, key)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cs.InitializeResult().Instructions, "list_collections") {
		t.Errorf("instructions = %q", cs.InitializeResult().Instructions)
	}
	tools, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range tools.Tools {
		names = append(names, tool.Name)
		readOnly := tool.Annotations != nil && tool.Annotations.ReadOnlyHint
		if readOnly != (strings.HasPrefix(tool.Name, "get_") || strings.HasPrefix(tool.Name, "list_")) {
			t.Errorf("%s read-only = %v", tool.Name, readOnly)
		}
	}
	if got := strings.Join(names, ","); got != "create_collection,create_profile,get_collection,get_job,get_listing,get_profile,list_collections,list_listings,list_profiles,redraft_profile,run_collection,update_collection,update_profile" {
		t.Errorf("tools = %s", got)
	}

	cols, _ := callTool[mcpCollections](t, cs, "list_collections", nil)
	if c := cols.Collections; len(c) != 2 || c[0].ID != "somerville" || c[0].Mode != "rent" || c[0].LastDay != day2 || c[0].Days != 2 || c[1].Days != 0 || c[1].LastDay != "" {
		t.Errorf("collections = %+v", cols)
	}

	ls, _ := callTool[mcpListings](t, cs, "list_listings", map[string]any{"collection": "somerville", "limit": 2})
	if ls.Day != day2 || ls.Matching != 3 || ls.Hidden != 1 || len(ls.Listings) != 2 {
		t.Fatalf("listings = %+v", ls)
	}
	z1 := ls.Listings[0]
	if z1.SourceID != "z1" || z1.Price != "$2,900/mo" || z1.PreviousPrice != "$3,000/mo" || z1.Profile != "attic" || z1.Match != 87.5 || len(z1.AlsoListedOn) != 1 || z1.AlsoListedOn[0] != "redfin" {
		t.Errorf("z1 row = %+v", z1)
	}
	if ls, _ := callTool[mcpListings](t, cs, "list_listings", map[string]any{"collection": "somerville", "include_dealbreakers": true}); len(ls.Listings) != 4 || ls.Listings[3].Dealbreakers[0] != "corp.carpet" {
		t.Errorf("with dealbreakers = %+v", ls.Listings)
	}

	l, raw := callTool[mcpListing](t, cs, "get_listing", map[string]any{"source": "zillow", "source_id": "z1", "collection": "somerville"})
	if l.Address != "7 Windom St, Somerville, MA 02144" || len(l.PriceHistory) != 2 || len(l.AlsoListed) != 1 || len(l.Grades) != 2 || l.Photos != 1 {
		t.Errorf("listing = %+v", l)
	}
	if c := l.Grades[0].Models[0].Criteria; len(c) != 3 || c[0].Kind != "want" || c[0].Label != "Skylights" || c[2].Kind != "avoid" {
		t.Errorf("criteria = %+v", c)
	}
	if len(raw.Content) != 2 {
		t.Fatalf("content = %d items", len(raw.Content))
	}
	if img, ok := raw.Content[1].(*mcp.ImageContent); !ok || img.MIMEType != "image/jpeg" || len(img.Data) == 0 {
		t.Errorf("second content item should be the collage: %T", raw.Content[1])
	}
	if _, raw := callTool[mcpListing](t, cs, "get_listing", map[string]any{"source": "zillow", "source_id": "z1", "no_photos": true}); len(raw.Content) != 1 {
		t.Errorf("no_photos content = %d items", len(raw.Content))
	}
	res2, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: "get_listing", Arguments: map[string]any{"source": "zillow", "source_id": "nope"}})
	if err != nil || !res2.IsError {
		t.Errorf("missing listing = %+v %v", res2, err)
	}

	ps, _ := callTool[mcpProfiles](t, cs, "list_profiles", nil)
	if len(ps.Profiles) != 3 {
		t.Errorf("profiles = %+v", ps)
	}
	p, _ := callTool[mcpProfile](t, cs, "get_profile", map[string]any{"id": "attic"})
	if len(p.Wants) != 2 || len(p.Avoids) != 1 || p.Avoids[0].From != "corp" || len(p.Collections) != 2 {
		t.Errorf("profile = %+v", p)
	}

	j, _ := callTool[mcpJob](t, cs, "run_collection", map[string]any{"collection": "somerville"})
	if j.ID == 0 || j.Status != "queued" || j.Collection != "somerville" {
		t.Errorf("run = %+v", j)
	}
	if got, _ := callTool[mcpJob](t, cs, "get_job", map[string]any{"id": j.ID}); got.ID != j.ID || got.Status != "queued" || got.Result != nil {
		t.Errorf("job = %+v", got)
	}

	db := a.svc.Store()
	claimed, ok, err := db.ClaimJob(t.Context(), time.Now())
	if err != nil || !ok || claimed.ID != j.ID {
		t.Fatalf("claim = %+v %v %v", claimed, ok, err)
	}
	claimed.Status, claimed.FinishedAt, claimed.Result = store.JobSucceeded, time.Now(), json.RawMessage(`{"day":"2026-10-04","stats":{"Cached":12}}`)
	if err := db.FinishJob(t.Context(), claimed); err != nil {
		t.Fatal(err)
	}
	done, _ := callTool[mcpJob](t, cs, "get_job", map[string]any{"id": j.ID})
	if r, ok := done.Result.(map[string]any); done.Status != "succeeded" || !ok || r["day"] != "2026-10-04" {
		t.Errorf("finished job = %+v", done)
	}
}

func toolError(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if !res.IsError || len(res.Content) == 0 {
		t.Fatalf("%s should fail: %+v", name, res)
	}
	return res.Content[0].(*mcp.TextContent).Text
}

func TestMCPMutations(t *testing.T) {
	a, h, key := apiApp(t)
	srv := httptest.NewServer(h)
	defer srv.Close()
	cs, err := mcpSession(t, srv, key)
	if err != nil {
		t.Fatal(err)
	}

	c, _ := callTool[mcpCollectionDetail](t, cs, "get_collection", map[string]any{"id": "somerville"})
	if c.Mode != "rent" || c.Search.Location != "Somerville, MA" || len(c.Profiles) != 2 {
		t.Fatalf("collection = %+v", c)
	}

	created, _ := callTool[mcpCollectionDetail](t, cs, "create_collection", map[string]any{
		"id": "cambridge", "mode": "rent", "sources": []string{"zillow"}, "profiles": []string{"loft"},
		"location": "Cambridge, MA", "max_price": 4000, "min_beds": 1, "schedule": "05:30",
	})
	if created.ID != "cambridge" || created.Search.MaxPrice == nil || *created.Search.MaxPrice != 4000 || created.Schedule != "05:30" {
		t.Errorf("created = %+v", created)
	}
	if msg := toolError(t, cs, "create_collection", map[string]any{"id": "cambridge", "mode": "rent", "sources": []string{"zillow"}, "profiles": []string{"loft"}, "location": "x"}); !strings.Contains(msg, "already exists") {
		t.Errorf("duplicate create = %q", msg)
	}
	if msg := toolError(t, cs, "create_collection", map[string]any{"id": "bad", "mode": "rent", "sources": []string{"zillow"}, "profiles": []string{"corp"}, "location": "x"}); msg == "internal error" {
		t.Errorf("avoid profile in a collection should give a validation message, got %q", msg)
	}

	updated, _ := callTool[mcpCollectionDetail](t, cs, "update_collection", map[string]any{
		"id": "cambridge", "profiles": []string{"attic", "loft"}, "max_price": 4500, "clear": []string{"min_beds"}, "schedule": "",
	})
	if len(updated.Profiles) != 2 || *updated.Search.MaxPrice != 4500 || updated.Search.MinBeds != nil || updated.Schedule != "" || updated.Search.Location != "Cambridge, MA" {
		t.Errorf("updated = %+v", updated)
	}
	if msg := toolError(t, cs, "update_collection", map[string]any{"id": "cambridge", "clear": []string{"location"}}); !strings.Contains(msg, "cannot clear") {
		t.Errorf("bad clear = %q", msg)
	}
	if msg := toolError(t, cs, "update_collection", map[string]any{"id": "cambridge", "schedule": "25:00"}); !strings.Contains(msg, "schedule") {
		t.Errorf("bad schedule = %q", msg)
	}
	if msg := toolError(t, cs, "update_collection", map[string]any{"id": "nope"}); !strings.Contains(msg, "not found") {
		t.Errorf("missing collection = %q", msg)
	}

	p, _ := callTool[mcpProfile](t, cs, "update_profile", map[string]any{
		"id": "attic", "summary": "Top floor, lots of light.", "notes": []string{"skylights matter most"},
		"criteria": []map[string]any{{"id": "skylights", "importance": "essential"}, {"id": "floors", "remove": true}},
	})
	if p.Name != "Sunny attic" || p.Summary != "Top floor, lots of light." || len(p.Wants) != 1 || p.Wants[0].Importance != "essential" || p.Wants[0].Label != "Skylights" {
		t.Errorf("updated profile = %+v", p)
	}
	stored, err := a.svc.Profile(t.Context(), "attic")
	if err != nil || len(stored.Notes) != 1 || stored.Notes[0] != "skylights matter most" {
		t.Errorf("stored notes = %+v %v", stored.Notes, err)
	}
	if msg := toolError(t, cs, "update_profile", map[string]any{"id": "attic", "criteria": []map[string]any{{"id": "carpet", "remove": true}}}); !strings.Contains(msg, "no criterion") {
		t.Errorf("inherited criterion edit = %q", msg)
	}
	if msg := toolError(t, cs, "update_profile", map[string]any{"id": "attic", "criteria": []map[string]any{{"id": "skylights", "importance": "huge"}}}); !strings.Contains(msg, "importance") {
		t.Errorf("bad importance = %q", msg)
	}

	j, _ := callTool[mcpJob](t, cs, "redraft_profile", map[string]any{"id": "attic"})
	if j.Kind != "draft_profile" || j.Profile != "attic" || j.Status != "queued" {
		t.Errorf("redraft job = %+v", j)
	}
	if msg := toolError(t, cs, "redraft_profile", map[string]any{"id": "loft"}); !strings.Contains(msg, "no reference listings") {
		t.Errorf("redraft without references = %q", msg)
	}
	if msg := toolError(t, cs, "create_profile", map[string]any{"url": "not a url", "id": "attic"}); !strings.Contains(msg, "URL") || !strings.Contains(msg, "already exists") {
		t.Errorf("bad create_profile = %q", msg)
	}
}
