package app

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"net/http"
	"time"
)

func fingerprint(fsys fs.FS) (map[string]string, error) {
	hashes := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		hashes[p] = hex.EncodeToString(sum[:])[:12]
		return nil
	})
	return hashes, err
}

func (a *App) staticURL(name string) string {
	if h, ok := a.hashes[name]; ok {
		return "/static/" + name + "?v=" + h
	}
	return "/static/" + name
}

func (a *App) serveStatic(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("path")
	hash, ok := a.hashes[name]
	if !ok {
		a.notFound(w, r)
		return
	}
	b, err := fs.ReadFile(a.static, name)
	if err != nil {
		a.notFound(w, r)
		return
	}
	if r.URL.Query().Get("v") == hash {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.Header().Set("ETag", `"`+hash+`"`)
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(b))
}
