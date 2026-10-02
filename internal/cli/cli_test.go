package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
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

func seedCollection(t *testing.T, id string) string {
	t.Helper()
	dir := t.TempDir()
	ctx := t.Context()
	svc, err := service.Open(ctx, service.Config{DataDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	if err := svc.SaveProfile(ctx, profile.Profile{ID: "attic", Ignore: profile.DefaultIgnore}); err != nil {
		t.Fatal(err)
	}
	c := collection.Collection{ID: id, Mode: profile.ModeRent, Sources: []listing.Source{listing.SourceZillow}, Profiles: []string{"attic"},
		Search: profile.Search{Location: "Somerville, MA", Limit: 10}}
	if err := svc.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestMigrate(t *testing.T) {
	out, err := run(t, "migrate", "--data-dir", t.TempDir())
	if err != nil || !strings.HasPrefix(out, "schema version ") || strings.TrimSpace(out) == "schema version 0" {
		t.Errorf("migrate: %v %q", err, out)
	}
}

func TestRun(t *testing.T) {
	data := seedCollection(t, "somerville")
	t.Setenv("OPENROUTER_API_KEY", "")
	if _, err := run(t, "run", "--data-dir", data); err == nil {
		t.Error("run without a collection should be rejected")
	}
	held, err := service.Open(t.Context(), service.Config{DataDir: data, Exclusive: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, "run", "somerville", "--data-dir", data); !errors.Is(err, service.ErrLocked) {
		t.Errorf("a locked data dir should fail fast: %v", err)
	}
	held.Close()
	if _, err := run(t, "run", "somerville", "--data-dir", data); err == nil || !strings.Contains(err.Error(), "OPENROUTER_API_KEY") {
		t.Errorf("a missing key should fail before any work: %v", err)
	}
}

func TestImport(t *testing.T) {
	root := t.TempDir()
	dirs := []string{"--data-dir", filepath.Join(root, "data"), "--profiles-dir", filepath.Join(root, "profiles"), "--collections-dir", filepath.Join(root, "collections")}
	if _, err := run(t, append([]string{"import"}, dirs...)...); err == nil || !strings.Contains(err.Error(), "nothing to import") {
		t.Errorf("empty import: %v", err)
	}
	b, _ := json.Marshal(profile.Profile{ID: "attic", Name: "Attic", Ignore: profile.DefaultIgnore})
	os.MkdirAll(filepath.Join(root, "profiles", "attic"), 0o755)
	if err := os.WriteFile(filepath.Join(root, "profiles", "attic", "profile.json"), b, 0o644); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		out, err := run(t, append([]string{"import"}, dirs...)...)
		if err != nil || !strings.Contains(out, `msg="import finished" stage=import profiles=1 references=0 media_copied=0 collections=0`) {
			t.Fatalf("import: %v\n%s", err, out)
		}
	}
	svc, err := service.Open(t.Context(), service.Config{DataDir: filepath.Join(root, "data")})
	if err != nil {
		t.Fatal(err)
	}
	defer svc.Close()
	ps, err := svc.Profiles(t.Context())
	if err != nil || len(ps) != 1 || ps[0].Name != "Attic" {
		t.Errorf("imported profiles: %v %+v", err, ps)
	}
}
