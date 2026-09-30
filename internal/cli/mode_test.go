package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), err
}

func TestProfileSearch(t *testing.T) {
	dir := t.TempDir()
	store := profile.Store{Root: dir}
	if err := store.Save(profile.Profile{ID: "attic", Ignore: profile.DefaultIgnore}); err != nil {
		t.Fatal(err)
	}

	if _, err := run(t, "profile", "search", "attic", "--profiles-dir", dir, "--mode", "rent",
		"--location", "Somerville, MA", "--max-price", "3500", "--min-beds", "2", "--amenity", "dishwasher", "--max-age", "36h"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "profile", "search", "attic", "--profiles-dir", dir, "--mode", "buy",
		"--location", "Somerville, MA", "--max-price", "850000"); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "profile", "search", "attic", "--profiles-dir", dir, "--mode", "rent", "--max-price", "3800"); err != nil {
		t.Fatal(err)
	}
	out, err := run(t, "profile", "search", "attic", "--profiles-dir", dir, "--mode", "rent")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"max_price": 3800`) || !strings.Contains(out, `"location": "Somerville, MA"`) {
		t.Errorf("show should print the merged rent search:\n%s", out)
	}

	p, err := store.Load("attic")
	if err != nil {
		t.Fatal(err)
	}
	rent, buy := p.Searches[profile.ModeRent], p.Searches[profile.ModeBuy]
	if *rent.MaxPrice != 3800 || *rent.MinBeds != 2 || rent.MaxAgeDays != 2 || rent.Amenities[0] != listing.AmenityDishwasher {
		b, _ := json.Marshal(rent)
		t.Errorf("rent search = %s", b)
	}
	if *buy.MaxPrice != 850000 || buy.MinBeds != nil {
		b, _ := json.Marshal(buy)
		t.Errorf("buy search = %s", b)
	}

	if _, err := run(t, "profile", "search", "attic", "--profiles-dir", dir, "--mode", "buy", "--clear", "--location", "Cambridge, MA"); err != nil {
		t.Fatal(err)
	}
	p, _ = store.Load("attic")
	if buy := p.Searches[profile.ModeBuy]; buy.Location != "Cambridge, MA" || buy.MaxPrice != nil {
		t.Errorf("--clear should replace the saved search: %+v", buy)
	}

	if _, err := run(t, "profile", "search", "attic", "--profiles-dir", dir, "--mode", "lease"); err == nil {
		t.Error("invalid mode should fail")
	}
	if _, err := run(t, "profile", "search", "attic", "--profiles-dir", dir, "--amenity", "pool"); err == nil {
		t.Error("unknown amenity should fail")
	}
}

func TestFetchRequiresSavedSearchForMode(t *testing.T) {
	dir := t.TempDir()
	if err := (profile.Store{Root: dir}).Save(profile.Profile{ID: "attic", Ignore: profile.DefaultIgnore}); err != nil {
		t.Fatal(err)
	}
	_, err := run(t, "fetch", "--profile", "attic", "--profiles-dir", dir, "--mode", "buy")
	if err == nil || !strings.Contains(err.Error(), "no buy search") {
		t.Fatalf("got %v", err)
	}
}

func TestFetchDeferredAmenitiesNeedEnrichment(t *testing.T) {
	t.Setenv("APIFY_TOKEN", "unused")
	_, err := run(t, "fetch", "--mode", "buy", "--location", "02145", "--amenity", "dishwasher", "--enrich=false", "--data-dir", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "enable --enrich") {
		t.Fatalf("got %v", err)
	}
}

func TestSplitAmenities(t *testing.T) {
	filterable, deferred := splitAmenities(
		[]listing.Amenity{listing.AmenityDishwasher, listing.AmenityParking},
		[]listing.Amenity{listing.AmenityParking},
	)
	if len(filterable) != 1 || filterable[0] != listing.AmenityParking || len(deferred) != 1 || deferred[0] != listing.AmenityDishwasher {
		t.Errorf("filterable=%v deferred=%v", filterable, deferred)
	}
}
