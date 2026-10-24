package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestSplitTopic(t *testing.T) {
	for in, want := range map[string][2]string{
		"https://ntfy.sh/housing":             {"https://ntfy.sh/", "housing"},
		"https://ntfy.sh/housing/":            {"https://ntfy.sh/", "housing"},
		"http://box.lan:8080/ntfy/housing":    {"http://box.lan:8080/ntfy/", "housing"},
		"https://u:p@ntfy.example/t?auth=tok": {"https://u:p@ntfy.example/?auth=tok", "t"},
	} {
		base, topic, err := SplitTopic(in)
		if err != nil || base != want[0] || topic != want[1] {
			t.Errorf("SplitTopic(%q) = %q, %q, %v; want %v", in, base, topic, err, want)
		}
	}
	for _, in := range []string{"", "ntfy.sh/housing", "ftp://ntfy.sh/housing", "https://ntfy.sh", "https://ntfy.sh/", "https://:pw@ntfy.sh"} {
		if _, _, err := SplitTopic(in); err == nil {
			t.Errorf("SplitTopic(%q) should fail", in)
		}
	}
}

func TestNtfyNotify(t *testing.T) {
	var (
		got  ntfyMessage
		path string
		user string
		pass string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		user, pass, _ = r.BasicAuth()
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()

	n := NewNtfy(strings.Replace(srv.URL, "http://", "http://me:secret@", 1) + "/sub/housing")
	m := Message{Title: "92% match · 7 Windom St", Body: "$3,000/mo", Click: "https://example.com/l", Attach: "https://example.com/1.jpg", Tags: []string{"house"}, Priority: 4}
	if err := n.Notify(t.Context(), m); err != nil {
		t.Fatal(err)
	}
	want := ntfyMessage{Topic: "housing", Title: m.Title, Message: m.Body, Click: m.Click, Attach: m.Attach, Tags: m.Tags, Priority: 4}
	if path != "/sub/" || user != "me" || pass != "secret" || !reflect.DeepEqual(got, want) {
		t.Errorf("posted %s as %s:%s\n%+v\nwant\n%+v", path, user, pass, got, want)
	}
}

func TestNtfyNotifyError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"limit reached"}`, http.StatusTooManyRequests)
	}))
	defer srv.Close()
	err := NewNtfy(srv.URL+"/housing").Notify(t.Context(), Message{Body: "x"})
	if err == nil || !strings.Contains(err.Error(), "429") || !strings.Contains(err.Error(), "limit reached") {
		t.Errorf("err = %v", err)
	}
}
