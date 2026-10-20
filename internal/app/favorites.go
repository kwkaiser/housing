package app

import (
	"net/http"
	"net/url"
	"strings"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/service"
)

type starButton struct {
	Action string
	Next   string
	On     bool
}

type favoriteRow struct {
	service.FavoriteView
	Href  string
	Added string
	Star  starButton
}

type favoritesData struct {
	Rows []favoriteRow
}

func star(l listing.Listing, on bool, next string) starButton {
	return starButton{Action: favoriteURL(l), Next: next, On: on}
}

func favoriteURL(l listing.Listing) string {
	return "/listing/" + url.PathEscape(string(l.Source)) + "/" + url.PathEscape(l.SourceID) + "/favorite"
}

func (a *App) favoritesPage(w http.ResponseWriter, r *http.Request) {
	fs, err := a.svc.Favorites(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	var d favoritesData
	for _, f := range fs {
		d.Rows = append(d.Rows, favoriteRow{
			FavoriteView: f,
			Href:         listingURL(f.Listing, "", ""),
			Added:        f.CreatedAt.Local().Format("2006-01-02"),
			Star:         star(f.Listing, true, "/favorites"),
		})
	}
	a.render(w, r, http.StatusOK, "favorites", d)
}

func (a *App) setFavorite(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		a.renderMessage(w, r, http.StatusBadRequest, "Bad request", "The form could not be read.")
		return
	}
	l := listing.Listing{Source: listing.Source(r.PathValue("source")), SourceID: r.PathValue("id")}
	if err := a.svc.SetFavorite(r.Context(), l.Source, l.SourceID, r.PostForm.Get("on") == "1"); err != nil {
		a.fail(w, r, err)
		return
	}
	next := r.PostForm.Get("next")
	if !localPath(next) {
		next = listingURL(l, "", "")
	}
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func localPath(s string) bool {
	if !strings.HasPrefix(s, "/") || strings.HasPrefix(s, "//") || strings.ContainsAny(s, "\\\r\n") {
		return false
	}
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "" && u.Host == ""
}
