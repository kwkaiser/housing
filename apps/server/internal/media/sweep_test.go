package media

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestSweep(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	old := now.Add(-48 * time.Hour)
	write := func(key string, mtime time.Time) {
		t.Helper()
		p := filepath.Join(root, filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("12345"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mtime, mtime); err != nil {
			t.Fatal(err)
		}
	}

	kept := PhotoKey("https://example.com/kept.jpg")
	gone := PhotoKey("https://example.com/gone.jpg")
	fresh := PhotoKey("https://example.com/fresh.jpg")
	files := map[string]time.Time{
		kept:                              old,
		kept + ".s320.jpg":                old,
		gone:                              old,
		gone + ".s640.jpg":                old,
		fresh:                             now,
		"collages/aa/keep/0.jpg":          old,
		"collages/aa/keep/0.jpg.s320.jpg": old,
		"collages/aa/keep/index.json":     old,
		"collages/bb/drop/0.jpg":          old,
		"collages/bb/drop/index.json":     old,
		"tiles/0/0/0.png":                 old,
	}
	for k, mt := range files {
		write(k, mt)
	}
	keep := Keep{}
	keep.Add([]string{"https://example.com/kept.jpg"}, []string{"collages/aa/keep/0.jpg"})
	o := SweepOptions{Before: now.Add(-24 * time.Hour), DryRun: true}

	res, err := Sweep(t.Context(), root, keep, o)
	if err != nil {
		t.Fatal(err)
	}
	want := SweepResult{Scanned: 10, Kept: 5, Recent: 1, Deleted: 4, Bytes: 20}
	if res != want {
		t.Errorf("dry run = %+v, want %+v", res, want)
	}
	for k := range files {
		if _, err := os.Stat(filepath.Join(root, k)); err != nil {
			t.Errorf("dry run removed %s", k)
		}
	}

	o.DryRun = false
	if res, err = Sweep(t.Context(), root, keep, o); err != nil || res != want {
		t.Errorf("sweep = %+v %v, want %+v", res, err, want)
	}
	var left []string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			left = append(left, filepath.ToSlash(rel))
		}
		return nil
	})
	slices.Sort(left)
	wantLeft := []string{"collages/aa/keep/0.jpg", "collages/aa/keep/0.jpg.s320.jpg", "collages/aa/keep/index.json", kept, kept + ".s320.jpg", fresh, "tiles/0/0/0.png"}
	slices.Sort(wantLeft)
	if !slices.Equal(left, wantLeft) {
		t.Errorf("left = %v, want %v", left, wantLeft)
	}
	if _, err := os.Stat(filepath.Join(root, "collages/bb")); !os.IsNotExist(err) {
		t.Errorf("emptied collage directories should be removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "collages")); err != nil {
		t.Errorf("top-level media directories should stay: %v", err)
	}
}

func TestSweepMissingRoot(t *testing.T) {
	res, err := Sweep(t.Context(), t.TempDir(), Keep{}, SweepOptions{Before: time.Now()})
	if err != nil || res != (SweepResult{}) {
		t.Errorf("sweep of an empty data dir = %+v %v", res, err)
	}
}
