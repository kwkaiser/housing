package collection

import (
	"errors"
	"strings"
	"testing"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

func valid() Collection {
	return Collection{
		ID:       "somerville",
		Mode:     profile.ModeRent,
		Sources:  []listing.Source{listing.SourceZillow},
		Profiles: []string{"somerville-attic"},
		Search:   profile.Search{Location: "Somerville, MA", Limit: 50},
	}
}

func TestValidate(t *testing.T) {
	c := valid()
	c.Mode, c.Sources, c.Profiles, c.Search.Location = "lease", nil, nil, ""
	err := c.Validate()
	for _, want := range []string{"invalid mode", "source", "profile", "location"} {
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("missing %q in %v", want, err)
		}
	}
}

func TestValidateFields(t *testing.T) {
	c := valid()
	c.ID, c.Profiles, c.MaxRunCostUSD, c.Schedule = "Bad", []string{"a", "a"}, -1, "7am"
	c.Search.MinPrice, c.Search.MaxPrice = ptr(5), ptr(1)
	c.Notify = Notify{URL: "ntfy.sh/topic", MinScore: -1}
	err := c.Validate()
	got := map[string]bool{}
	for _, e := range err.(interface{ Unwrap() error }).Unwrap().(interface{ Unwrap() []error }).Unwrap() {
		var fe profile.FieldError
		if errors.As(e, &fe) {
			got[fe.Field] = true
		}
	}
	for _, f := range []string{"id", "profiles", "max_run_cost_usd", "schedule", "max_price", "notify_url", "notify_min_score"} {
		if !got[f] {
			t.Errorf("no %s field error in %v", f, err)
		}
	}
}

func ptr(n int) *int {
	return &n
}

func TestSchedule(t *testing.T) {
	for _, s := range []string{"", "00:00", "07:30", "23:59"} {
		c := valid()
		c.Schedule = s
		if err := c.Validate(); err != nil {
			t.Errorf("%q: %v", s, err)
		}
	}
	for _, s := range []string{"7:30", "24:00", "07:60", "7am", "07:30:00", " 07:30"} {
		c := valid()
		c.Schedule = s
		if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "schedule") {
			t.Errorf("%q should be rejected: %v", s, err)
		}
	}
	c := valid()
	day := time.Date(2026, 9, 30, 0, 0, 0, 0, time.Local)
	if _, ok := c.ScheduledAt(day); ok {
		t.Error("an unscheduled collection has no run time")
	}
	c.Schedule = "07:30"
	if at, ok := c.ScheduledAt(day); !ok || !at.Equal(time.Date(2026, 9, 30, 7, 30, 0, 0, time.Local)) {
		t.Errorf("scheduled at %v", at)
	}
}

func TestNotifyThreshold(t *testing.T) {
	if got := (Notify{URL: "https://ntfy.sh/x"}).Threshold(); got != DefaultNotifyMinScore {
		t.Errorf("default threshold = %v", got)
	}
	if got := (Notify{URL: "https://ntfy.sh/x", MinScore: 95}).Threshold(); got != 95 {
		t.Errorf("threshold = %v", got)
	}
	c := valid()
	c.Notify = Notify{URL: "https://ntfy.sh/housing", MinScore: 90}
	if err := c.Validate(); err != nil {
		t.Errorf("valid notify config rejected: %v", err)
	}
}
