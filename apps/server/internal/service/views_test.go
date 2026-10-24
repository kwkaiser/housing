package service

import (
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

func TestPickDay(t *testing.T) {
	days := []store.DayCount{{Day: "2026-09-01"}, {Day: "2026-09-10"}, {Day: "2026-09-20"}}
	for want, i := range map[string]int{"": 2, "junk": 2, "2026-09-10": 1, "2026-09-15": 1, "2026-08-01": 0, "2026-12-01": 2} {
		if got := pickDay(days, want); got != i {
			t.Errorf("pickDay(%q) = %d, want %d", want, got, i)
		}
	}
	if pickDay(nil, "2026-09-10") != -1 {
		t.Error("no days should pick nothing")
	}
}

func TestSightings(t *testing.T) {
	seen := []store.Sighting{
		{Day: "2026-09-01", PriceCents: 300000, InputHash: "a"},
		{Day: "2026-09-10", PriceCents: 290000, InputHash: "a"},
		{Day: "2026-09-20", PriceCents: 280000, InputHash: "a"},
		{Day: "2026-09-21", PriceCents: 280000, InputHash: "a"},
		{Day: "2026-09-22", PriceCents: 280000, InputHash: "b"},
		{Day: "2026-09-23", PriceCents: 280000},
	}
	first, status, prev := sightings("2026-09-20", seen)
	if first != "2026-09-01" || status != StatusChanged || prev == nil || *prev != 290000 {
		t.Errorf("price change got %q %q %v", first, status, prev)
	}
	if first, status, prev := sightings("2026-09-01", seen); first != "2026-09-01" || status != StatusNew || prev != nil {
		t.Errorf("first sighting got %q %q %v", first, status, prev)
	}
	if first, status, prev := sightings("2026-09-05", nil); first != "2026-09-05" || status != StatusNew || prev != nil {
		t.Errorf("unseen got %q %q %v", first, status, prev)
	}
	for day, want := range map[string]Status{"2026-09-21": StatusRepeat, "2026-09-22": StatusChanged, "2026-09-23": StatusRepeat} {
		if _, status, _ := sightings(day, seen); status != want {
			t.Errorf("%s status = %q, want %q", day, status, want)
		}
	}
}

func TestValidMediaKey(t *testing.T) {
	for key, want := range map[string]bool{
		"photos/ab/abc.jpg":       true,
		"collages/ab/abc":         true,
		"photos/../housing.db":    false,
		"photos/ab/../../x":       false,
		"/photos/ab/abc.jpg":      false,
		"other/ab/abc":            false,
		"photos/ab//abc":          false,
		"photos\\..\\x":           false,
		"collages/ab/abc/":        false,
		"housing.sqlite3":         false,
		"collages/ab/abc\x00.jpg": false,
	} {
		if got := ValidMediaKey(key); got != want {
			t.Errorf("ValidMediaKey(%q) = %v, want %v", key, got, want)
		}
	}
}
