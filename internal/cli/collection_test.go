package cli

import (
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func TestCollectionSet(t *testing.T) {
	profiles, collections := t.TempDir(), t.TempDir()
	for _, id := range []string{"attic", "loft"} {
		if err := (profile.Store{Root: profiles}).Save(profile.Profile{ID: id, Ignore: profile.DefaultIgnore}); err != nil {
			t.Fatal(err)
		}
	}
	dirs := []string{"--profiles-dir", profiles, "--collections-dir", collections}

	if _, err := run(t, append([]string{"collection", "set", "somerville", "--profile", "attic,loft", "--source", "zillow,redfin",
		"--location", "Somerville, MA", "--max-price", "4000", "--max-run-cost-usd", "2"}, dirs...)...); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, append([]string{"collection", "set", "somerville", "--max-price", "4500"}, dirs...)...); err != nil {
		t.Fatal(err)
	}
	c, err := collection.Store{Root: collections}.Load("somerville")
	if err != nil {
		t.Fatal(err)
	}
	if *c.Search.MaxPrice != 4500 || c.Search.Location != "Somerville, MA" || len(c.Profiles) != 2 || len(c.Sources) != 2 ||
		c.Sources[1] != listing.SourceRedfin || c.MaxRunCostUSD != 2 || c.Mode != profile.ModeRent || c.Search.Limit != 50 {
		t.Errorf("updates should merge into the collection: %+v", c)
	}

	if out, err := run(t, append([]string{"collection", "list"}, dirs...)...); err != nil || !strings.Contains(out, "somerville  rent  Somerville, MA  zillow,redfin  attic,loft") {
		t.Errorf("list: %v\n%s", err, out)
	}
	if _, err := run(t, append([]string{"collection", "set", "bad", "--profile", "missing", "--location", "x"}, dirs...)...); err == nil {
		t.Error("unknown profile should be rejected")
	}
	if _, err := run(t, append([]string{"collection", "set", "bad", "--profile", "attic", "--location", "x", "--source", "myspace"}, dirs...)...); err == nil || !strings.Contains(err.Error(), "unsupported source") {
		t.Errorf("unknown source should be rejected: %v", err)
	}
}

func TestScopeRequiresProfiles(t *testing.T) {
	data := t.TempDir()
	for _, args := range [][]string{{"report"}, {"run"}} {
		if _, err := run(t, append(args, "--data-dir", data, "--collections-dir", t.TempDir())...); err == nil || !strings.Contains(err.Error(), "--profile or --collection") {
			t.Errorf("%v: %v", args, err)
		}
	}
	if _, err := run(t, "report", "--collection", "nope", "--data-dir", data, "--collections-dir", t.TempDir()); err == nil || !strings.Contains(err.Error(), "collection not found") {
		t.Errorf("missing collection: %v", err)
	}
}
