package profile

import (
	"errors"
	"fmt"
	"regexp"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
)

type Importance string

const (
	Essential Importance = "essential"
	High      Importance = "high"
	Medium    Importance = "medium"
	Low       Importance = "low"
)

var Importances = []Importance{Essential, High, Medium, Low}

type Evidence string

const (
	EvidencePhotos      Evidence = "photos"
	EvidenceDescription Evidence = "description"
	EvidenceEither      Evidence = "either"
)

var Evidences = []Evidence{EvidencePhotos, EvidenceDescription, EvidenceEither}

type Criterion struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	LookFor    string     `json:"look_for"`
	NotThis    string     `json:"not_this,omitempty"`
	Keywords   []string   `json:"keywords,omitempty"`
	Importance Importance `json:"importance"`
	Evidence   Evidence   `json:"evidence"`
}

type Reference struct {
	Source   listing.Source `json:"source"`
	SourceID string         `json:"source_id"`
	URL      string         `json:"url"`
	Collages []string       `json:"collages"`
}

type Drafted struct {
	Model   string    `json:"model"`
	At      time.Time `json:"at"`
	CostUSD float64   `json:"cost_usd"`
}

type Profile struct {
	ID         string      `json:"id"`
	Name       string      `json:"name"`
	Summary    string      `json:"summary"`
	Notes      []string    `json:"notes,omitempty"`
	Want       []Criterion `json:"want"`
	Avoid      []Criterion `json:"avoid,omitempty"`
	Ignore     []string    `json:"ignore"`
	References []Reference `json:"references"`
	Drafted    *Drafted    `json:"drafted,omitempty"`
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func ValidID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("invalid profile id %q: use lowercase letters, digits and dashes", id)
	}
	return nil
}

func (p Profile) Validate() error {
	var errs []error
	if err := ValidID(p.ID); err != nil {
		errs = append(errs, err)
	}
	seen := map[string]bool{}
	for _, c := range append(append([]Criterion{}, p.Want...), p.Avoid...) {
		if c.ID == "" {
			errs = append(errs, fmt.Errorf("criterion %q has no id", c.Label))
		}
		if seen[c.ID] {
			errs = append(errs, fmt.Errorf("duplicate criterion id %q", c.ID))
		}
		seen[c.ID] = true
		if !valid(Importances, c.Importance) {
			errs = append(errs, fmt.Errorf("criterion %q: invalid importance %q", c.ID, c.Importance))
		}
		if !valid(Evidences, c.Evidence) {
			errs = append(errs, fmt.Errorf("criterion %q: invalid evidence %q", c.ID, c.Evidence))
		}
	}
	return errors.Join(errs...)
}

func valid[T comparable](all []T, v T) bool {
	for _, a := range all {
		if a == v {
			return true
		}
	}
	return false
}
