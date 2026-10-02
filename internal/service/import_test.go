package service

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing/jsonfile"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeProfileFiles(t *testing.T, root string, p profile.Profile, ref listing.Listing) {
	t.Helper()
	writeJSON(t, filepath.Join(root, p.ID, "profile.json"), p)
	if err := (jsonfile.Persister{}).Persist(t.Context(), filepath.Join(root, p.ID, "listings"), []listing.Listing{ref}); err != nil {
		t.Fatal(err)
	}
	for _, key := range append(ref.Collages, "photos/"+p.ID+".jpg") {
		path := filepath.Join(root, p.ID, filepath.FromSlash(key))
		os.MkdirAll(filepath.Dir(path), 0o755)
		if err := os.WriteFile(path, []byte(key), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestImport(t *testing.T) {
	ctx := t.Context()
	root := t.TempDir()
	svc := openService(t, Config{DataDir: root + "/data", ProfilesDir: root + "/profiles", CollectionsDir: root + "/collections"})
	if err := svc.Import(ctx, nil); err == nil || !strings.Contains(err.Error(), "nothing to import") {
		t.Fatalf("an empty import should say so: %v", err)
	}

	wantRef := rental(listing.SourceZillow, "w1", "1 Attic Way")
	wantRef.Collages = []string{"collages/aa/w1/0.jpg", "collages/aa/w1/1.jpg"}
	want := profile.Profile{
		ID:      "attic",
		Name:    "Attic",
		Summary: "Sunny",
		Want:    []profile.Criterion{{ID: "skylights", Label: "Skylights", Importance: profile.High, Evidence: profile.EvidenceEither}},
		Ignore:  profile.DefaultIgnore,
		References: []profile.Reference{{
			Source: wantRef.Source, SourceID: wantRef.SourceID, URL: wantRef.URL, Collages: wantRef.Collages,
		}},
		Drafted: &profile.Drafted{Model: "m", At: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)},
	}
	wantRef = wantRef.WithAssessment("attic", listing.Assessment{Model: "g", ProfileHash: "x", InputHash: profile.InputHash(wantRef), Score: 70})
	avoidRef := rental(listing.SourceZillow, "a1", "2 Tower Pl")
	avoidRef.Collages = []string{"collages/bb/a1/0.jpg"}
	avoid := profile.Profile{
		ID:         "corporate",
		Kind:       profile.KindAvoid,
		Avoid:      []profile.Criterion{{ID: "tower", Label: "Tower", Importance: profile.Essential, Evidence: profile.EvidencePhotos}},
		Ignore:     profile.DefaultIgnore,
		References: []profile.Reference{{Source: avoidRef.Source, SourceID: avoidRef.SourceID, URL: avoidRef.URL, Collages: avoidRef.Collages}},
	}
	writeProfileFiles(t, svc.cfg.ProfilesDir, want, wantRef)
	writeProfileFiles(t, svc.cfg.ProfilesDir, avoid, avoidRef)
	c := collection.Collection{ID: "somerville", Mode: profile.ModeRent, Sources: []listing.Source{listing.SourceZillow}, Profiles: []string{"attic"}, Search: profile.Search{Location: "02144", Limit: 5}}
	writeJSON(t, filepath.Join(svc.cfg.CollectionsDir, "somerville.json"), c)

	if _, err := svc.Profiles(ctx); !errors.Is(err, ErrNotImported) || !strings.Contains(err.Error(), "housing import") {
		t.Fatalf("unimported profiles should be reported: %v", err)
	}

	var logs logBuffer
	if err := svc.Import(ctx, logs.logger()); err != nil {
		t.Fatal(err)
	}
	if out := logs.String(); !strings.Contains(out, `msg="import finished" stage=import profiles=2 references=2 media_copied=5 collections=1 db=`) {
		t.Errorf("summary missing:\n%s", out)
	}
	logs = logBuffer{}
	if err := svc.Import(ctx, logs.logger()); err != nil {
		t.Fatalf("import should be safe to repeat: %v", err)
	}
	if out := logs.String(); !strings.Contains(out, `msg="import finished" stage=import profiles=2 references=2 media_copied=0 collections=1`) {
		t.Errorf("a repeat import should copy nothing:\n%s", out)
	}

	profiles, err := svc.Profiles(ctx)
	if err != nil || len(profiles) != 2 || profiles[0].ID != "attic" || profiles[1].ID != "corporate" {
		t.Fatalf("profiles = %+v %v", profiles, err)
	}
	if profiles[0].Hash() != want.Hash() || profiles[1].Hash() != avoid.Hash() {
		t.Error("imported profiles must keep their hashes")
	}

	db, err := svc.catalog(ctx)
	if err != nil {
		t.Fatal(err)
	}
	fromFiles, err := profile.Effective(want, []profile.Profile{want, avoid})
	if err != nil {
		t.Fatal(err)
	}
	fromDB, err := db.EffectiveProfile(ctx, "attic")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fromDB, fromFiles) || fromDB.Hash() != fromFiles.Hash() {
		t.Errorf("effective profile from the database differs:\n%+v\n%+v", fromDB, fromFiles)
	}

	templates, err := svc.templates(ctx, db, []string{"attic"})
	if err != nil {
		t.Fatal(err)
	}
	refs := templates[0].References
	if len(refs) != 1 || refs[0].SourceID != "w1" || !reflect.DeepEqual(refs[0].Assessments, wantRef.Assessments) {
		t.Errorf("own references with their calibration should load: %+v", refs)
	}
	inputs, err := svc.references(templates[0].Profile, templates[0].refs)
	if err != nil || len(inputs) != 2 || !inputs[1].Avoid || string(inputs[1].Collages[0]) != "collages/bb/a1/0.jpg" {
		t.Errorf("reference collages should be read from the data directory: %+v %v", inputs, err)
	}
	if listings, _ := db.Load(ctx); len(listings) != 0 {
		t.Errorf("reference listings must not appear as fetched listings: %d", len(listings))
	}

	got, err := svc.Collection(ctx, "somerville")
	if err != nil || !reflect.DeepEqual(got, c) {
		t.Errorf("collection = %+v %v", got, err)
	}
}
