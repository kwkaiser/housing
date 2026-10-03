package app

import (
	"bytes"
	"fmt"
	"html/template"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/app/view"
)

type navItem struct {
	Label    string
	Href     string
	Active   bool
	prefixes []string
}

type page struct {
	Path string
	Nav  []navItem
	Data any
}

type errorData struct {
	Status  int
	Title   string
	Message string
}

var nav = []navItem{
	{Label: "Listings", Href: "/", prefixes: []string{"/c/", "/listing/"}},
	{Label: "Collections", Href: "/collections"},
	{Label: "Profiles", Href: "/profiles"},
	{Label: "Jobs", Href: "/jobs"},
	{Label: "API keys", Href: "/keys"},
}

func (a *App) funcs() template.FuncMap {
	funcs := view.Funcs()
	funcs["static"] = a.staticURL
	funcs["profileURL"] = profileURL
	return funcs
}

func parsePages(fsys fs.FS, funcs template.FuncMap) (map[string]*template.Template, error) {
	layout, err := template.New("layout").Funcs(funcs).ParseFS(fsys, "layout.tmpl")
	if err != nil {
		return nil, err
	}
	files, err := fs.Glob(fsys, "pages/*.tmpl")
	if err != nil {
		return nil, err
	}
	pages := make(map[string]*template.Template, len(files))
	for _, f := range files {
		t, err := template.Must(layout.Clone()).ParseFS(fsys, f)
		if err != nil {
			return nil, err
		}
		pages[strings.TrimSuffix(path.Base(f), ".tmpl")] = t
	}
	if pages["error"] == nil {
		return nil, fmt.Errorf("missing pages/error.tmpl")
	}
	return pages, nil
}

func navFor(p string) []navItem {
	items := make([]navItem, len(nav))
	for i, n := range nav {
		n.Active = p == n.Href || n.Href != "/" && strings.HasPrefix(p, n.Href+"/") ||
			slices.ContainsFunc(n.prefixes, func(prefix string) bool { return strings.HasPrefix(p, prefix) })
		items[i] = n
	}
	return items
}

func (a *App) execute(name string, r *http.Request, data any) ([]byte, error) {
	t, ok := a.pages[name]
	if !ok {
		return nil, fmt.Errorf("unknown page %q", name)
	}
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "layout", page{Path: r.URL.Path, Nav: navFor(r.URL.Path), Data: data}); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func (a *App) render(w http.ResponseWriter, r *http.Request, status int, name string, data any) {
	body, err := a.execute(name, r, data)
	if err != nil {
		a.log.Error("render", "page", name, "path", r.URL.Path, "err", err)
		a.renderError(w, r, http.StatusInternalServerError)
		return
	}
	write(w, status, body)
}

func (a *App) renderError(w http.ResponseWriter, r *http.Request, status int) {
	switch status {
	case http.StatusNotFound:
		a.renderMessage(w, r, status, "Page not found", "There is nothing at "+r.URL.Path+".")
	case http.StatusInternalServerError:
		a.renderMessage(w, r, status, "Something went wrong", "The server hit an error rendering this page. Details are in the server log.")
	default:
		a.renderMessage(w, r, status, http.StatusText(status), "")
	}
}

func (a *App) renderMessage(w http.ResponseWriter, r *http.Request, status int, title, message string) {
	body, err := a.execute("error", r, errorData{Status: status, Title: title, Message: message})
	if err != nil {
		a.log.Error("render error page", "status", status, "err", err)
		http.Error(w, http.StatusText(status), status)
		return
	}
	write(w, status, body)
}

func write(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if w.Header().Get("Cache-Control") == "" {
		w.Header().Set("Cache-Control", "no-cache")
	}
	w.WriteHeader(status)
	w.Write(body)
}
