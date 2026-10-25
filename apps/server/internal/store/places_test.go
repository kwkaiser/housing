package store

import (
	"errors"
	"testing"
	"time"
)

func TestPlaces(t *testing.T) {
	ctx := t.Context()
	s := open(t)
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

	b, err := s.AddPlace(ctx, Place{Name: "Sam", Address: "2 Elm St, Somerville, MA", Lat: 42.39, Lng: -71.1, CreatedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	a, err := s.AddPlace(ctx, Place{Name: "Alex", Address: "1 Oak St, Cambridge, MA", Lat: 42.37, Lng: -71.11, CreatedAt: at})
	if err != nil {
		t.Fatal(err)
	}
	if a.ID == 0 || a.ID == b.ID {
		t.Errorf("ids = %d %d", a.ID, b.ID)
	}
	ps, err := s.Places(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 || ps[0] != a || ps[1] != b {
		t.Errorf("Places = %+v", ps)
	}

	if err := s.RemovePlace(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.RemovePlace(ctx, a.ID); !errors.Is(err, ErrPlaceNotFound) {
		t.Errorf("remove twice err = %v", err)
	}
	if ps, err := s.Places(ctx); err != nil || len(ps) != 1 || ps[0].ID != b.ID {
		t.Errorf("after remove = %+v %v", ps, err)
	}
}
