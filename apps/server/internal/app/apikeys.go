package app

import (
	"errors"
	"net/http"
	"strconv"

	"git.kwkaiser.io/kwkaiser/housing/internal/app/view"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

type apiKeyRow struct {
	store.APIKey
	Created  string
	LastUsed string
}

type apiKeysData struct {
	Keys      []apiKeyRow
	Created   *apiKeyRow
	Token     string
	Name      string
	NameError string
}

func (a *App) apiKeysPage(w http.ResponseWriter, r *http.Request) {
	a.renderAPIKeys(w, r, http.StatusOK, apiKeysData{})
}

func (a *App) createAPIKey(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxFormBytes)
	if err := r.ParseForm(); err != nil {
		a.renderMessage(w, r, http.StatusBadRequest, "Bad request", "The form could not be read.")
		return
	}
	name := r.PostForm.Get("name")
	k, token, err := a.svc.CreateAPIKey(r.Context(), name)
	var fe profile.FieldError
	if errors.As(err, &fe) {
		a.renderAPIKeys(w, r, http.StatusUnprocessableEntity, apiKeysData{Name: name, NameError: fe.Error()})
		return
	}
	if err != nil {
		a.fail(w, r, err)
		return
	}
	row := a.apiKeyRow(k)
	w.Header().Set("Cache-Control", "no-store")
	a.renderAPIKeys(w, r, http.StatusCreated, apiKeysData{Created: &row, Token: token})
}

func (a *App) deleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		a.notFound(w, r)
		return
	}
	if err := a.svc.DeleteAPIKey(r.Context(), id); err != nil {
		a.fail(w, r, err)
		return
	}
	http.Redirect(w, r, "/keys", http.StatusSeeOther)
}

func (a *App) renderAPIKeys(w http.ResponseWriter, r *http.Request, status int, d apiKeysData) {
	ks, err := a.svc.APIKeys(r.Context())
	if err != nil {
		a.fail(w, r, err)
		return
	}
	for _, k := range ks {
		d.Keys = append(d.Keys, a.apiKeyRow(k))
	}
	a.render(w, r, status, "keys", d)
}

func (a *App) apiKeyRow(k store.APIKey) apiKeyRow {
	now := a.now()
	row := apiKeyRow{APIKey: k, Created: k.CreatedAt.Local().Format("2006-01-02 15:04"), LastUsed: "never"}
	if !k.LastUsedAt.IsZero() {
		row.LastUsed = view.Ago(k.LastUsedAt, now)
	}
	return row
}
