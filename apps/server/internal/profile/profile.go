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

type Kind string

const (
	KindWant  Kind = "want"
	KindAvoid Kind = "avoid"
)

func ParseKind(s string) (Kind, error) {
	switch k := Kind(s); k {
	case KindWant, KindAvoid:
		return k, nil
	}
	return "", fmt.Errorf("invalid kind %q: use want or avoid", s)
}

type Criterion struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	LookFor    string     `json:"look_for"`
	NotThis    string     `json:"not_this,omitempty"`
	Keywords   []string   `json:"keywords,omitempty"`
	Importance Importance `json:"importance" jsonschema:"enum=essential,enum=high,enum=medium,enum=low"`
	Evidence   Evidence   `json:"evidence" jsonschema:"enum=photos,enum=description,enum=either"`
}

type Reference struct {
	Profile  string         `json:"profile,omitempty"`
	Avoid    bool           `json:"avoid,omitempty"`
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
	ID         string          `json:"id"`
	Kind       Kind            `json:"kind,omitempty"`
	Name       string          `json:"name"`
	Summary    string          `json:"summary"`
	Notes      []string        `json:"notes,omitempty"`
	Want       []Criterion     `json:"want"`
	Avoid      []Criterion     `json:"avoid,omitempty"`
	Ignore     []string        `json:"ignore"`
	References []Reference     `json:"references"`
	Searches   map[Mode]Search `json:"searches,omitempty"`
	Drafted    *Drafted        `json:"drafted,omitempty"`
}

const DefaultRoot = "profiles"

var (
	ErrNotFound = errors.New("profile not found")
	ErrExists   = errors.New("profile already exists")
)

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func ValidID(id string) error {
	if !idPattern.MatchString(id) {
		return fmt.Errorf("invalid profile id %q: use lowercase letters, digits and dashes", id)
	}
	return nil
}

func (p Profile) IsAvoid() bool {
	return p.Kind == KindAvoid
}

func (p Profile) Validate() error {
	var errs []error
	if err := ValidID(p.ID); err != nil {
		errs = append(errs, err)
	}
	if p.Kind != "" {
		if _, err := ParseKind(string(p.Kind)); err != nil {
			errs = append(errs, err)
		}
	}
	if p.IsAvoid() && len(p.Want) > 0 {
		errs = append(errs, fmt.Errorf("avoid profile %q cannot have want criteria", p.ID))
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
	for mode, search := range p.Searches {
		if _, err := ParseMode(string(mode)); err != nil {
			errs = append(errs, err)
		}
		if err := search.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("%s search: %w", mode, err))
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

func Effective(p Profile, all []Profile) (Profile, error) {
	if p.IsAvoid() {
		return Profile{}, fmt.Errorf("%q is an avoid profile; it is applied automatically when assessing against a want profile", p.ID)
	}
	p.Avoid = append([]Criterion{}, p.Avoid...)
	p.References = append([]Reference{}, p.References...)
	for _, ap := range all {
		if !ap.IsAvoid() || len(ap.Avoid) == 0 {
			continue
		}
		for _, c := range ap.Avoid {
			c.ID = ap.ID + "." + c.ID
			p.Avoid = append(p.Avoid, c)
		}
		for _, r := range ap.References {
			r.Profile, r.Avoid = ap.ID, true
			p.References = append(p.References, r)
		}
	}
	return p, p.Validate()
}
