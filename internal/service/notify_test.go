package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"git.kwkaiser.io/kwkaiser/housing/internal/collection"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/notify"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

type fakeNotifier struct {
	mu   sync.Mutex
	urls []string
	sent []notify.Message
	err  error
}

func (f *fakeNotifier) client(url string) notify.Notifier {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.urls = append(f.urls, url)
	return f
}

func (f *fakeNotifier) Notify(_ context.Context, m notify.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

func (f *fakeNotifier) titles() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []string
	for _, m := range f.sent {
		out = append(out, m.Title+" | "+strings.Split(m.Body, "\n")[0])
	}
	slices.Sort(out)
	return out
}

func notifyService(t *testing.T, n collection.Notify) (*Service, *fakeNotifier) {
	t.Helper()
	svc := newTestService(t, &fakeCompleter{})
	fn := &fakeNotifier{}
	svc.cfg.Clients.Notifier = fn.client
	ctx := t.Context()
	if _, err := svc.CreateProfileFromURL(ctx, CreateProfileOptions{URL: "https://www.zillow.com/homedetails/ref", ID: "attic", Kind: profile.KindWant, Model: "m"}, nil); err != nil {
		t.Fatal(err)
	}
	c := collection.Collection{
		ID: "somerville", Mode: profile.ModeRent, Profiles: []string{"attic"}, Model: "test/grader", Notify: n,
		Sources: []listing.Source{listing.SourceZillow, listing.SourceRedfin}, Search: profile.Search{Location: "Somerville, MA", Limit: 5},
	}
	if err := svc.SaveCollection(ctx, c); err != nil {
		t.Fatal(err)
	}
	return svc, fn
}

func TestRunNotifiesOncePerListing(t *testing.T) {
	ctx := t.Context()
	svc, fn := notifyService(t, collection.Notify{URL: "https://ntfy.sh/housing"})
	var logs logBuffer
	res, err := svc.RunCollection(ctx, "somerville", logs.logger())
	if err != nil || res.NotifyErr != nil || res.Notified != 2 {
		t.Fatalf("run = %+v, %v", res, err)
	}
	want := []string{
		"100% match in somerville | 31 Fairmount Ave, Somerville, MA 02144",
		"100% match in somerville | 7 Windom St, Somerville, MA 02144",
	}
	if got := fn.titles(); !slices.Equal(got, want) {
		t.Errorf("sent %q", got)
	}
	m := fn.sent[0]
	if !strings.Contains(m.Body, "$3,000/mo · 2 bd") || !strings.HasPrefix(m.Click, "https://example.com/") || !strings.HasSuffix(m.Attach, "/1.jpg") || fn.urls[0] != "https://ntfy.sh/housing" {
		t.Errorf("message = %+v via %v", m, fn.urls)
	}
	if out := logs.String(); !strings.Contains(out, `msg="notifications sent" stage=notify sent=2 min_score=80 failed=0`) {
		t.Errorf("missing summary in\n%s", out)
	}
	notified, err := svc.Store().Notified(ctx, "somerville")
	if err != nil || len(notified) != 3 {
		t.Errorf("recorded %v, %v; want every member of each group", notified, err)
	}

	again, err := svc.RunCollection(ctx, "somerville", nil)
	if err != nil || again.Notified != 0 || len(fn.sent) != 2 {
		t.Errorf("a second run should not resend: %+v %v", again, err)
	}
}

func TestRunNotifyThreshold(t *testing.T) {
	svc, fn := notifyService(t, collection.Notify{URL: "https://ntfy.sh/housing", MinScore: 101})
	res, err := svc.RunCollection(t.Context(), "somerville", nil)
	if err != nil || res.Notified != 0 || len(fn.sent) != 0 {
		t.Errorf("nothing should clear the threshold: %+v %v", res, err)
	}
}

func TestRunWithoutNotifyURL(t *testing.T) {
	svc, fn := notifyService(t, collection.Notify{})
	res, err := svc.RunCollection(t.Context(), "somerville", nil)
	if err != nil || res.Notified != 0 || len(fn.urls) != 0 {
		t.Errorf("no url should not notify: %+v %v %v", res, err, fn.urls)
	}
}

func TestRunNotifyFailureRetries(t *testing.T) {
	ctx := t.Context()
	svc, fn := notifyService(t, collection.Notify{URL: "https://ntfy.sh/housing"})
	fn.err = errors.New("ntfy: 429 Too Many Requests")
	res, err := svc.RunCollection(ctx, "somerville", nil)
	if err != nil || res.Notified != 0 || res.NotifyErr == nil || !strings.Contains(res.NotifyErr.Error(), "429") {
		t.Fatalf("a failed notification should not fail the run: %+v %v", res, err)
	}
	if notified, _ := svc.Store().Notified(ctx, "somerville"); len(notified) != 0 {
		t.Errorf("failed notifications must not be recorded: %v", notified)
	}
	fn.err = nil
	res, err = svc.RunCollection(ctx, "somerville", nil)
	if err != nil || res.Notified != 2 || res.NotifyErr != nil {
		t.Errorf("the next run should retry: %+v %v", res, err)
	}
}
