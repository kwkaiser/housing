package app

import (
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var newKey = regexp.MustCompile(`value="(hk_[A-Za-z0-9_-]+)" readonly`)

func TestAPIKeysPage(t *testing.T) {
	a, _ := newApp(t)
	h := a.Handler()

	res, body := get(t, h, "/keys")
	if res.StatusCode != http.StatusOK || !strings.Contains(body, "There are no API keys yet") || !strings.Contains(body, `<a href="/keys" aria-current="page">API keys</a>`) {
		t.Fatalf("empty page = %d\n%s", res.StatusCode, body)
	}

	if res, _ := post(t, h, "/keys", url.Values{"name": {"agent"}}, map[string]string{"Sec-Fetch-Site": "cross-site"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site create = %d", res.StatusCode)
	}
	if res, body := post(t, h, "/keys", url.Values{"name": {"  "}}, sameOriginHeader); res.StatusCode != http.StatusUnprocessableEntity || !strings.Contains(body, "a name is required") {
		t.Errorf("blank name = %d\n%s", res.StatusCode, body)
	}

	res, body = post(t, h, "/keys", url.Values{"name": {"laptop agent"}}, sameOriginHeader)
	m := newKey.FindStringSubmatch(body)
	if res.StatusCode != http.StatusCreated || m == nil || res.Header.Get("Cache-Control") != "no-store" || !strings.Contains(body, "Created <strong>laptop agent</strong>") {
		t.Fatalf("create = %d %v\n%s", res.StatusCode, res.Header, body)
	}
	token := m[1]
	if r, _ := apiDo(t, h, http.MethodGet, "/api/v1/collections", token); r.StatusCode != http.StatusOK {
		t.Errorf("new key rejected: %d", r.StatusCode)
	}

	_, body = get(t, h, "/keys")
	if strings.Contains(body, token) || !strings.Contains(body, "<code>"+token[:9]+"…</code>") || !strings.Contains(body, "just now") {
		t.Errorf("list should show only the hint and last use:\n%s", body)
	}

	ks, err := a.svc.APIKeys(t.Context())
	if err != nil || len(ks) != 1 {
		t.Fatalf("keys = %+v %v", ks, err)
	}
	target := "/keys/" + strconv.FormatInt(ks[0].ID, 10) + "/delete"
	if res, _ := post(t, h, target, nil, map[string]string{"Sec-Fetch-Site": "cross-site"}); res.StatusCode != http.StatusForbidden {
		t.Errorf("cross-site delete = %d", res.StatusCode)
	}
	if res, _ := post(t, h, target, nil, sameOriginHeader); res.StatusCode != http.StatusSeeOther || res.Header.Get("Location") != "/keys" {
		t.Errorf("delete = %d %v", res.StatusCode, res.Header)
	}
	if res, _ := post(t, h, target, nil, sameOriginHeader); res.StatusCode != http.StatusNotFound {
		t.Errorf("delete again = %d, want 404", res.StatusCode)
	}
	if r, _ := apiDo(t, h, http.MethodGet, "/api/v1/collections", token); r.StatusCode != http.StatusUnauthorized {
		t.Errorf("deleted key = %d, want 401", r.StatusCode)
	}
}
