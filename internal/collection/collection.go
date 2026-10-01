package collection

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/profile"
)

const DefaultRoot = "collections"

var (
	ErrNotFound = errors.New("collection not found")
	ErrExists   = errors.New("collection already exists")
)

type Collection struct {
	ID            string           `json:"id"`
	Mode          profile.Mode     `json:"mode"`
	Sources       []listing.Source `json:"sources"`
	Profiles      []string         `json:"profiles"`
	Search        profile.Search   `json:"search"`
	Model         string           `json:"model,omitempty"`
	MaxRunCostUSD float64          `json:"max_run_cost_usd,omitempty"`
	Schedule      string           `json:"schedule,omitempty"`
}

const ScheduleLayout = "15:04"

func ParseSchedule(s string) (hour, minute int, err error) {
	t, err := time.Parse(ScheduleLayout, s)
	if err != nil || len(s) != len(ScheduleLayout) {
		return 0, 0, fmt.Errorf("invalid schedule %q: want a 24-hour HH:MM time", s)
	}
	return t.Hour(), t.Minute(), nil
}

func (c Collection) ScheduledAt(day time.Time) (time.Time, bool) {
	if c.Schedule == "" {
		return time.Time{}, false
	}
	h, m, err := ParseSchedule(c.Schedule)
	if err != nil {
		return time.Time{}, false
	}
	y, mo, d := day.Date()
	return time.Date(y, mo, d, h, m, 0, 0, day.Location()), true
}

func (c Collection) Validate() error {
	var errs []error
	field := func(name string, err error) {
		errs = append(errs, profile.FieldError{Field: name, Err: err})
	}
	if profile.ValidID(c.ID) != nil {
		field("id", fmt.Errorf("invalid collection id %q: use lowercase letters, digits and dashes", c.ID))
	}
	if _, err := profile.ParseMode(string(c.Mode)); err != nil {
		field("mode", err)
	}
	if len(c.Sources) == 0 {
		field("sources", fmt.Errorf("at least one source is required"))
	}
	if len(c.Profiles) == 0 {
		field("profiles", fmt.Errorf("at least one profile is required"))
	}
	for i, id := range c.Profiles {
		if slices.Contains(c.Profiles[:i], id) {
			field("profiles", fmt.Errorf("profile %q is listed more than once", id))
		}
	}
	if c.Search.Location == "" {
		field("location", fmt.Errorf("a search location is required"))
	}
	if c.MaxRunCostUSD < 0 {
		field("max_run_cost_usd", fmt.Errorf("max run cost must not be negative"))
	}
	if c.Schedule != "" {
		if _, _, err := ParseSchedule(c.Schedule); err != nil {
			field("schedule", err)
		}
	}
	if err := c.Search.Validate(); err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("collection %q: %w", c.ID, err)
	}
	return nil
}
