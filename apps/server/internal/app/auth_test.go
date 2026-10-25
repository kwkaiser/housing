package app

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"html"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"golang.org/x/oauth2"

	"git.kwkaiser.io/kwkaiser/housing/internal/auth"
	"git.kwkaiser.io/kwkaiser/housing/internal/config"
)

const (
	testClientID  = "housing"
	testPublicURL = "https://housing.example.com"
)

type fakeIdP struct {
	*httptest.Server
	t      *testing.T
	signer jose.Signer
	mu     sync.Mutex
	codes  map[string]fakeGrant
}

type fakeGrant struct {
	nonce, challenge string
}

func newFakeIdP(t *testing.T) *fakeIdP {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: jose.JSONWebKey{Key: key, KeyID: "k1"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	idp := &fakeIdP{t: t, signer: signer, codes: map[string]fakeGrant{}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"issuer":                                idp.URL,
			"authorization_endpoint":                idp.URL + "/authorize",
			"token_endpoint":                        idp.URL + "/token",
			"jwks_uri":                              idp.URL + "/jwks",
			"end_session_endpoint":                  idp.URL + "/end-session",
			"id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("GET /jwks", func(w http.ResponseWriter, _ *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		idp.mu.Lock()
		g, ok := idp.codes[r.PostForm.Get("code")]
		delete(idp.codes, r.PostForm.Get("code"))
		idp.mu.Unlock()
		sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
		if !ok || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "invalid_grant"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"access_token": idp.token(map[string]any{}),
			"token_type":   "Bearer",
			"expires_in":   300,
			"id_token":     idp.token(map[string]any{"nonce": g.nonce}),
		})
	})
	idp.Server = httptest.NewServer(mux)
	t.Cleanup(idp.Close)
	return idp
}

func (idp *fakeIdP) authorize(t *testing.T, location string) (code, state string) {
	t.Helper()
	u, err := url.Parse(location)
	if err != nil || !strings.HasPrefix(location, idp.URL+"/authorize") {
		t.Fatalf("login redirect = %q", location)
	}
	q := u.Query()
	if q.Get("client_id") != testClientID || q.Get("redirect_uri") != testPublicURL+auth.CallbackPath || q.Get("code_challenge_method") != "S256" || q.Get("nonce") == "" {
		t.Fatalf("authorize params = %v", q)
	}
	code = rand.Text()
	idp.mu.Lock()
	idp.codes[code] = fakeGrant{nonce: q.Get("nonce"), challenge: q.Get("code_challenge")}
	idp.mu.Unlock()
	return code, q.Get("state")
}

func (idp *fakeIdP) token(extra map[string]any) string {
	idp.t.Helper()
	claims := map[string]any{
		"iss": idp.URL, "aud": testClientID, "sub": "user-1", "name": "Karl", "email": "karl@example.com",
		"iat": time.Now().Unix(), "exp": time.Now().Add(5 * time.Minute).Unix(),
	}
	for k, v := range extra {
		claims[k] = v
	}
	b, _ := json.Marshal(claims)
	obj, err := idp.signer.Sign(b)
	if err != nil {
		idp.t.Fatal(err)
	}
	s, err := obj.CompactSerialize()
	if err != nil {
		idp.t.Fatal(err)
	}
	return s
}

func oidcApp(t *testing.T) (*App, *fakeIdP) {
	t.Helper()
	idp := newFakeIdP(t)
	a := seededApp(t)
	o, err := auth.New(t.Context(), config.OIDC{Issuer: idp.URL, ClientID: testClientID, ClientSecret: "secret", PublicURL: testPublicURL})
	if err != nil {
		t.Fatal(err)
	}
	a.OIDC = o
	return a, idp
}

type authReq struct {
	method, target, body string
	cookies              []*http.Cookie
	header               map[string]string
}

func authDo(t *testing.T, h http.Handler, ar authReq) *http.Response {
	t.Helper()
	req := httptest.NewRequest(ar.method, ar.target, strings.NewReader(ar.body))
	req.Host = "housing.example.com"
	if ar.body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	for _, c := range ar.cookies {
		req.AddCookie(c)
	}
	for k, v := range ar.header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Result()
}

func cookieNamed(t *testing.T, res *http.Response, name string) *http.Cookie {
	t.Helper()
	for _, c := range res.Cookies() {
		if c.Name == name {
			if !c.HttpOnly || !c.Secure || c.SameSite != http.SameSiteLaxMode || c.Path != "/" {
				t.Errorf("cookie %s attributes = %+v", name, c)
			}
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", name, res.Header["Set-Cookie"])
	return nil
}

func signIn(t *testing.T, h http.Handler, idp *fakeIdP, rd string) *http.Response {
	t.Helper()
	res := authDo(t, h, authReq{method: http.MethodGet, target: "/auth/login?rd=" + url.QueryEscape(rd)})
	if res.StatusCode != http.StatusFound {
		t.Fatalf("login = %d", res.StatusCode)
	}
	login := cookieNamed(t, res, "__Host-"+loginCookie)
	code, state := idp.authorize(t, res.Header.Get("Location"))
	return authDo(t, h, authReq{method: http.MethodGet, target: "/auth/callback?code=" + code + "&state=" + state, cookies: []*http.Cookie{login}})
}

func me(t *testing.T, h http.Handler, ar authReq) (APIUser, int) {
	t.Helper()
	ar.method, ar.target = http.MethodGet, "/api/v1/me"
	res := authDo(t, h, ar)
	var u APIUser
	json.NewDecoder(res.Body).Decode(&u)
	return u, res.StatusCode
}

func TestSignIn(t *testing.T) {
	a, idp := oidcApp(t)
	h := a.Handler()

	if _, status := me(t, h, authReq{}); status != http.StatusUnauthorized {
		t.Fatalf("me before sign-in = %d", status)
	}

	res := signIn(t, h, idp, "/favorites?x=1")
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/favorites?x=1" {
		t.Fatalf("callback = %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	session := cookieNamed(t, res, "__Host-"+sessionCookie)

	if u, status := me(t, h, authReq{cookies: []*http.Cookie{session}}); status != http.StatusOK || u.Auth != "session" || u.Name != "Karl" || u.Email != "karl@example.com" {
		t.Fatalf("me = %d %+v", status, u)
	}

	create := authReq{method: http.MethodPut, target: "/api/v1/listings/zillow/z1/favorite", cookies: []*http.Cookie{session}}
	for _, header := range []map[string]string{
		{"Sec-Fetch-Site": "cross-site"},
		{"Sec-Fetch-Site": "same-site"},
		{"Origin": "https://food.example.com"},
	} {
		create.header = header
		if res := authDo(t, h, create); res.StatusCode != http.StatusForbidden {
			t.Errorf("write with %v = %d, want 403", header, res.StatusCode)
		}
	}
	create.header = map[string]string{"Sec-Fetch-Site": "same-origin"}
	if res := authDo(t, h, create); res.StatusCode != http.StatusNoContent {
		t.Errorf("same-origin write = %d, want 204", res.StatusCode)
	}

	res = authDo(t, h, authReq{method: http.MethodPost, target: "/auth/logout", cookies: []*http.Cookie{session}, header: map[string]string{"Sec-Fetch-Site": "same-origin"}})
	if res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != idp.URL+"/end-session" {
		t.Fatalf("logout = %d %q", res.StatusCode, res.Header.Get("Location"))
	}
	if c := cookieNamed(t, res, "__Host-"+sessionCookie); c.MaxAge >= 0 {
		t.Errorf("logout cookie = %+v", c)
	}
	if _, status := me(t, h, authReq{cookies: []*http.Cookie{session}}); status != http.StatusUnauthorized {
		t.Errorf("me after logout = %d", status)
	}
}

func TestSignInRejects(t *testing.T) {
	a, idp := oidcApp(t)
	h := a.Handler()

	res := authDo(t, h, authReq{method: http.MethodGet, target: "/auth/login?rd=https://evil.example.com/"})
	login := cookieNamed(t, res, "__Host-"+loginCookie)
	code, state := idp.authorize(t, res.Header.Get("Location"))

	for name, ar := range map[string]authReq{
		"no login cookie": {target: "/auth/callback?code=" + code + "&state=" + state},
		"wrong state":     {target: "/auth/callback?code=" + code + "&state=x" + state, cookies: []*http.Cookie{login}},
		"provider error":  {target: "/auth/callback?error=access_denied&state=" + state, cookies: []*http.Cookie{login}},
	} {
		ar.method = http.MethodGet
		if res := authDo(t, h, ar); res.StatusCode < 400 || len(res.Cookies()) != 1 || res.Cookies()[0].Name != "__Host-"+loginCookie {
			t.Errorf("%s: %d %v", name, res.StatusCode, res.Header["Set-Cookie"])
		}
	}

	res = authDo(t, h, authReq{method: http.MethodGet, target: "/auth/callback?code=" + code + "&state=" + state, cookies: []*http.Cookie{login}})
	if res.StatusCode != http.StatusFound || res.Header.Get("Location") != "/" {
		t.Errorf("offsite rd: %d %q", res.StatusCode, res.Header.Get("Location"))
	}

	if res := authDo(t, h, authReq{method: http.MethodPost, target: "/auth/logout", header: map[string]string{"Sec-Fetch-Site": "cross-site"}}); res.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site logout = %d", res.StatusCode)
	}
}

func TestBearerAccessToken(t *testing.T) {
	a, idp := oidcApp(t)
	h := a.Handler()
	bearer := func(tok string) authReq {
		return authReq{header: map[string]string{"Authorization": "Bearer " + tok}}
	}

	if u, status := me(t, h, bearer(idp.token(nil))); status != http.StatusOK || u.Auth != "token" || u.Name != "Karl" {
		t.Errorf("valid token = %d %+v", status, u)
	}
	for name, tok := range map[string]string{
		"other audience": idp.token(map[string]any{"aud": "other"}),
		"other issuer":   idp.token(map[string]any{"iss": "https://elsewhere.example.com"}),
		"expired":        idp.token(map[string]any{"exp": time.Now().Add(-time.Minute).Unix()}),
		"garbage":        "not-a-jwt",
	} {
		if _, status := me(t, h, bearer(tok)); status != http.StatusUnauthorized {
			t.Errorf("%s = %d", name, status)
		}
	}

	write := bearer(idp.token(nil))
	write.method, write.target = http.MethodPut, "/api/v1/listings/zillow/z1/favorite"
	write.header["Sec-Fetch-Site"] = "cross-site"
	if res := authDo(t, h, write); res.StatusCode != http.StatusNoContent {
		t.Errorf("bearer write with cross-site fetch metadata = %d, want 204", res.StatusCode)
	}
}

func TestNoAuthUser(t *testing.T) {
	a := seededApp(t)
	a.NoAuth = true
	if u, status := me(t, a.Handler(), authReq{}); status != http.StatusOK || u.Auth != "none" {
		t.Errorf("me = %d %+v", status, u)
	}
	if res := authDo(t, a.Handler(), authReq{method: http.MethodGet, target: "/auth/login"}); res.StatusCode != http.StatusNotFound {
		t.Errorf("login without oidc = %d", res.StatusCode)
	}
}

func TestSafeReturn(t *testing.T) {
	for in, want := range map[string]string{
		"":                     "/",
		"/":                    "/",
		"/c/somerville?day=1":  "/c/somerville?day=1",
		"//evil.example.com":   "/",
		`/\evil.example.com`:   "/",
		"https://evil.example": "/",
		"javascript:alert(1)":  "/",
		"favorites":            "/",
	} {
		if got := safeReturn(in); got != want {
			t.Errorf("safeReturn(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestAppSignIn(t *testing.T) {
	a, idp := oidcApp(t)
	h := a.Handler()
	verifier := oauth2.GenerateVerifier()
	challenge := oauth2.S256ChallengeFromVerifier(verifier)

	if res := authDo(t, h, authReq{method: http.MethodGet, target: "/auth/login?app=short"}); res.StatusCode != http.StatusBadRequest {
		t.Errorf("login with bad challenge = %d", res.StatusCode)
	}

	res := authDo(t, h, authReq{method: http.MethodGet, target: "/auth/login?rd=%2Ffavorites&app=" + challenge})
	login := cookieNamed(t, res, "__Host-"+loginCookie)
	code, state := idp.authorize(t, res.Header.Get("Location"))
	res = authDo(t, h, authReq{method: http.MethodGet, target: "/auth/callback?code=" + code + "&state=" + state, cookies: []*http.Cookie{login}})
	if res.StatusCode != http.StatusOK {
		t.Fatalf("callback = %d", res.StatusCode)
	}
	for _, c := range res.Cookies() {
		if c.Name == "__Host-"+sessionCookie {
			t.Errorf("app callback set a browser session cookie")
		}
	}
	body, _ := io.ReadAll(res.Body)
	m := regexp.MustCompile(`href="(io\.kwkaiser\.housing://auth\?[^"]+)"`).FindSubmatch(body)
	if m == nil {
		t.Fatalf("handoff page has no app link:\n%s", body)
	}
	target, err := url.Parse(html.UnescapeString(string(m[1])))
	if err != nil || target.Query().Get("rd") != "/favorites" || target.Query().Get("code") == "" {
		t.Fatalf("app link = %q", m[1])
	}
	handoff := func(code, verifier string, header map[string]string) *http.Response {
		b, _ := json.Marshal(handoffRequest{Code: code, Verifier: verifier})
		return authDo(t, h, authReq{method: http.MethodPost, target: "/auth/handoff", body: string(b), header: header})
	}
	sameOrigin := map[string]string{"Sec-Fetch-Site": "same-origin"}

	if res := handoff(target.Query().Get("code"), verifier, map[string]string{"Sec-Fetch-Site": "cross-site"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site handoff = %d", res.StatusCode)
	}
	res = handoff(target.Query().Get("code"), verifier, sameOrigin)
	if res.StatusCode != http.StatusNoContent {
		t.Fatalf("handoff = %d", res.StatusCode)
	}
	session := cookieNamed(t, res, "__Host-"+sessionCookie)
	if u, status := me(t, h, authReq{cookies: []*http.Cookie{session}}); status != http.StatusOK || u.Name != "Karl" {
		t.Errorf("me after handoff = %d %+v", status, u)
	}
	if res := handoff(target.Query().Get("code"), verifier, sameOrigin); res.StatusCode != http.StatusBadRequest {
		t.Errorf("second handoff = %d", res.StatusCode)
	}
}
