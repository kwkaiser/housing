package media

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"time"
)

var resizedSuffix = regexp.MustCompile(`\.s\d+\.jpg$`)

type Keep map[string]bool

func (k Keep) Add(photos, collages []string) {
	for _, u := range photos {
		k[PhotoKey(u)] = true
	}
	for _, c := range collages {
		k[path.Dir(c)] = true
	}
}

func (k Keep) Has(key string) bool {
	base := resizedSuffix.ReplaceAllString(key, "")
	return k[base] || k[path.Dir(base)]
}

type SweepOptions struct {
	Before time.Time
	DryRun bool
}

type SweepResult struct {
	Scanned int
	Kept    int
	Recent  int
	Deleted int
	Bytes   int64
}

func Sweep(ctx context.Context, root string, keep Keep, o SweepOptions) (SweepResult, error) {
	var res SweepResult
	for _, top := range []string{"photos", "collages"} {
		var dirs []string
		err := filepath.WalkDir(filepath.Join(root, top), func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() {
				dirs = append(dirs, p)
				return nil
			}
			res.Scanned++
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			if keep.Has(filepath.ToSlash(rel)) {
				res.Kept++
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.ModTime().Before(o.Before) {
				res.Recent++
				return nil
			}
			if !o.DryRun {
				if err := os.Remove(p); err != nil {
					return err
				}
			}
			res.Deleted++
			res.Bytes += info.Size()
			return nil
		})
		if errors.Is(err, fs.ErrNotExist) && len(dirs) == 0 {
			continue
		}
		if err != nil {
			return res, err
		}
		if o.DryRun {
			continue
		}
		for _, d := range slices.Backward(dirs[1:]) {
			os.Remove(d)
		}
	}
	return res, nil
}
