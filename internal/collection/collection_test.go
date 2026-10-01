package collection

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func valid() Collection {
	return Collection{
		ID:       "somerville",
		Mode:     profile.ModeRent,
		Sources:  []listing.Source{listing.SourceZillow},
		Profiles: []string{"somerville-attic"},
		Search:   profile.Search{Location: "Somerville, MA", Limit: 50},
	}
}

func TestStore(t *testing.T) {
	s := Store{Root: t.TempDir()}
	if _, err := s.Load("somerville"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing collection: %v", err)
	}
	c := valid()
	if err := s.Save(c); err != nil {
		t.Fatal(err)
	}
	got, err := s.Load("somerville")
	if err != nil || got.Search.Location != "Somerville, MA" || got.Profiles[0] != "somerville-attic" {
		t.Fatalf("got %+v %v", got, err)
	}
	if all, err := s.List(); err != nil || len(all) != 1 {
		t.Errorf("list = %+v %v", all, err)
	}
	os.WriteFile(filepath.Join(s.Root, "other.json"), []byte(`{"id":"somerville"}`), 0o644)
	if _, err := s.Load("other"); err == nil {
		t.Error("a file whose id does not match its name should be rejected")
	}
}

func TestValidate(t *testing.T) {
	c := valid()
	c.Mode, c.Sources, c.Profiles, c.Search.Location = "lease", nil, nil, ""
	err := c.Validate()
	for _, want := range []string{"invalid mode", "source", "profile", "location"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}
