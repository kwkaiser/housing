package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/oauth2"

	"git.kwkaiser.io/kwkaiser/housing/internal/service"
	"git.kwkaiser.io/kwkaiser/housing/internal/store"
)

const (
	sessionCookie = "housing_session"
	loginCookie   = "housing_login"
	loginTTL      = 10 * time.Minute
	appRedirect   = "io.kwkaiser.housing://auth"
)

var crossOrigin http.CrossOriginProtection

var handoffPage = template.Must(template.New("handoff").Parse(`<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<meta http-equiv="refresh" content="0;url={{.}}">
<title>housing</title>
</head>
<body>
<p>Signed in. <a href="{{.}}">Return to the housing app</a>.</p>
</body>
</html>
`))

var (
	errUnauthenticated = errors.New("not signed in")
	errCrossSite       = errors.New("cross-site request")
)

type APIUser struct {
	Auth  string `json:"auth" enum:"none,session,token" doc:"How the request was authenticated: none when the server runs with --no-auth"`
	Name  string `json:"name"`
	Email string `json:"email,omitempty"`
}

type userKey struct{}

func userFrom(ctx context.Context) APIUser {
	u, _ := ctx.Value(userKey{}).(APIUser)
	return u
}

func (a *App) authenticate(r *http.Request) (APIUser, error) {
	if a.NoAuth {
		return APIUser{Auth: "none", Name: "dev"}, nil
	}
	if token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		return a.authenticateBearer(r.Context(), token)
	}
	c, err := r.Cookie(a.cookieName(sessionCookie))
	if err != nil {
		return APIUser{}, errUnauthenticated
	}
	s, ok, err := a.svc.Session(r.Context(), c.Value)
	if err != nil {
		return APIUser{}, err
	}
	if !ok {
		return APIUser{}, errUnauthenticated
	}
	if crossOrigin.Check(r) != nil {
		return APIUser{}, errCrossSite
	}
	return APIUser{Auth: "session", Name: s.Name, Email: s.Email}, nil
}

func (a *App) authenticateBearer(ctx context.Context, token string) (APIUser, error) {
	if a.OIDC == nil {
		return APIUser{}, errUnauthenticated
	}
	id, err := a.OIDC.Verify(ctx, token)
	if err != nil {
		a.log.InfoContext(ctx, "bearer token rejected", "err", err)
		return APIUser{}, errUnauthenticated
	}
	return APIUser{Auth: "token", Name: id.Name, Email: id.Email}, nil
}

type loginState struct {
	State    string `json:"s"`
	Nonce    string `json:"n"`
	Verifier string `json:"v"`
	Return   string `json:"r"`
	App      string `json:"a,omitempty"`
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if a.OIDC == nil {
		a.notFound(w, r)
		return
	}
	app := r.URL.Query().Get("app")
	if app != "" && !validChallenge(app) {
		http.Error(w, "invalid app challenge", http.StatusBadRequest)
		return
	}
	st := loginState{State: rand.Text(), Nonce: rand.Text(), Verifier: oauth2.GenerateVerifier(), Return: safeReturn(r.URL.Query().Get("rd")), App: app}
	b, err := json.Marshal(st)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.SetCookie(w, a.cookie(loginCookie, base64.RawURLEncoding.EncodeToString(b), loginTTL))
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, a.OIDC.AuthCodeURL(st.State, st.Nonce, st.Verifier), http.StatusFound)
}

func (a *App) callback(w http.ResponseWriter, r *http.Request) {
	if a.OIDC == nil {
		a.notFound(w, r)
		return
	}
	st, ok := a.loginState(r)
	http.SetCookie(w, a.cookie(loginCookie, "", -1))
	w.Header().Set("Cache-Control", "no-store")
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		a.log.WarnContext(r.Context(), "sign-in refused", "error", e, "description", q.Get("error_description"))
		http.Error(w, "Sign-in was not completed: "+e, http.StatusForbidden)
		return
	}
	if !ok || subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(st.State)) != 1 {
		http.Error(w, "Sign-in expired or was started in another tab. Go back to the app to try again.", http.StatusBadRequest)
		return
	}
	id, err := a.OIDC.Exchange(r.Context(), q.Get("code"), st.Verifier, st.Nonce)
	if err != nil {
		a.log.WarnContext(r.Context(), "sign-in exchange", "err", err)
		http.Error(w, "Sign-in failed.", http.StatusBadGateway)
		return
	}
	identity := store.Session{Subject: id.Subject, Name: id.Name, Email: id.Email}
	if st.App != "" {
		code, err := a.svc.CreateHandoff(r.Context(), st.App, identity)
		if err != nil {
			a.log.ErrorContext(r.Context(), "create handoff", "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		a.log.InfoContext(r.Context(), "signed in for app", "subject", id.Subject, "name", id.Name)
		target := appRedirect + "?" + url.Values{"code": {code}, "rd": {st.Return}}.Encode()
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		handoffPage.Execute(w, template.URL(target))
		return
	}
	token, err := a.svc.CreateSession(r.Context(), identity)
	if err != nil {
		a.log.ErrorContext(r.Context(), "create session", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	a.log.InfoContext(r.Context(), "signed in", "subject", id.Subject, "name", id.Name)
	http.SetCookie(w, a.cookie(sessionCookie, token, service.SessionTTL))
	http.Redirect(w, r, st.Return, http.StatusFound)
}

type handoffRequest struct {
	Code     string `json:"code"`
	Verifier string `json:"verifier"`
}

func (a *App) handoff(w http.ResponseWriter, r *http.Request) {
	if a.OIDC == nil {
		a.notFound(w, r)
		return
	}
	if crossOrigin.Check(r) != nil {
		http.Error(w, "cross-site request rejected", http.StatusForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	var req handoffRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}
	token, ok, err := a.svc.RedeemHandoff(r.Context(), req.Code, req.Verifier)
	if err != nil {
		a.log.ErrorContext(r.Context(), "redeem handoff", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "Sign-in expired. Try again.", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, a.cookie(sessionCookie, token, service.SessionTTL))
	w.WriteHeader(http.StatusNoContent)
}

func validChallenge(s string) bool {
	b, err := base64.RawURLEncoding.DecodeString(s)
	return err == nil && len(b) == sha256.Size
}

func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if crossOrigin.Check(r) != nil {
		http.Error(w, "cross-site request rejected", http.StatusForbidden)
		return
	}
	if c, err := r.Cookie(a.cookieName(sessionCookie)); err == nil {
		if err := a.svc.DeleteSession(r.Context(), c.Value); err != nil {
			a.log.ErrorContext(r.Context(), "delete session", "err", err)
		}
	}
	http.SetCookie(w, a.cookie(sessionCookie, "", -1))
	target := "/"
	if a.OIDC != nil && a.OIDC.LogoutURL() != "" {
		target = a.OIDC.LogoutURL()
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func (a *App) loginState(r *http.Request) (loginState, bool) {
	c, err := r.Cookie(a.cookieName(loginCookie))
	if err != nil {
		return loginState{}, false
	}
	b, err := base64.RawURLEncoding.DecodeString(c.Value)
	if err != nil {
		return loginState{}, false
	}
	var st loginState
	if err := json.Unmarshal(b, &st); err != nil || st.State == "" {
		return loginState{}, false
	}
	return st, true
}

func (a *App) secureCookies() bool {
	return a.OIDC != nil && a.OIDC.Secure()
}

func (a *App) cookieName(base string) string {
	if a.secureCookies() {
		return "__Host-" + base
	}
	return base
}

func (a *App) cookie(name, value string, ttl time.Duration) *http.Cookie {
	c := &http.Cookie{Name: a.cookieName(name), Value: value, Path: "/", HttpOnly: true, Secure: a.secureCookies(), SameSite: http.SameSiteLaxMode}
	if ttl < 0 {
		c.MaxAge = -1
	} else {
		c.MaxAge = int(ttl.Seconds())
	}
	return c
}

func safeReturn(rd string) string {
	u, err := url.Parse(rd)
	if err != nil || !strings.HasPrefix(rd, "/") || strings.HasPrefix(rd, "//") || strings.HasPrefix(rd, `/\`) || u.IsAbs() || u.Host != "" {
		return "/"
	}
	return rd
}
