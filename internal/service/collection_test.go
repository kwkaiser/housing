package service

import (
	"errors"
	"slices"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func TestSupportedSources(t *testing.T) {
	want := []listing.Source{listing.SourceZillow, listing.SourceRedfin, listing.SourceCraigslist, listing.SourceFacebook}
	if got := SupportedSources(); !slices.Equal(got, want) {
		t.Errorf("SupportedSources() = %v", got)
	}
}

func TestCreateAndSaveCollection(t *testing.T) {
	ctx := t.Context()
	svc := openService(t, Config{DataDir: t.TempDir()})
	for _, p := range []profile.Profile{
		{ID: "attic", Ignore: profile.DefaultIgnore},
		{ID: "corp", Kind: profile.KindAvoid, Ignore: profile.DefaultIgnore},
	} {
		if err := svc.SaveProfile(ctx, p); err != nil {
			t.Fatal(err)
		}
	}
	c := collection.Collection{ID: "somerville", Mode: profile.ModeRent, Sources: []listing.Source{listing.SourceZillow}, Profiles: []string{"attic"}, Search: profile.Search{Location: "Somerville, MA"}}
	if err := svc.CreateCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	changed := c
	changed.Search.Location = "Boston, MA"
	if err := svc.CreateCollection(ctx, changed); !errors.Is(err, collection.ErrExists) {
		t.Errorf("create over an existing id = %v", err)
	}
	if got, _ := svc.Collection(ctx, "somerville"); got.Search.Location != "Somerville, MA" {
		t.Errorf("create overwrote: %+v", got)
	}
	if err := svc.SaveCollection(ctx, changed); err != nil {
		t.Fatal(err)
	}

	bad := c
	bad.Sources, bad.Profiles, bad.Search.Location = []listing.Source{"myspace"}, []string{"corp", "ghost"}, ""
	err := svc.SaveCollection(ctx, bad)
	fields := map[string]int{}
	var walk func(error)
	walk = func(err error) {
		var fe profile.FieldError
		if e, ok := err.(profile.FieldError); ok {
			fields[e.Field]++
			return
		}
		if m, ok := err.(interface{ Unwrap() []error }); ok {
			for _, e := range m.Unwrap() {
				walk(e)
			}
			return
		}
		if errors.As(err, &fe) {
			walk(errors.Unwrap(err))
		}
	}
	walk(err)
	if fields["sources"] != 1 || fields["profiles"] != 2 || fields["location"] != 1 || !errors.Is(err, profile.ErrNotFound) {
		t.Errorf("field errors = %v from %v", fields, err)
	}
}
