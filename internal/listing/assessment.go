package listing

import (
	"fmt"
	"time"
)

type Verdict string

const (
	VerdictPresent Verdict = "present"
	VerdictPartial Verdict = "partial"
	VerdictAbsent  Verdict = "absent"
	VerdictUnknown Verdict = "unknown"
)

type Confidence string

const (
	ConfidenceLow    Confidence = "low"
	ConfidenceMedium Confidence = "medium"
	ConfidenceHigh   Confidence = "high"
)

type CriterionResult struct {
	ID         string     `json:"id"`
	Verdict    Verdict    `json:"verdict" jsonschema:"enum=present,enum=partial,enum=absent,enum=unknown"`
	Confidence Confidence `json:"confidence" jsonschema:"enum=low,enum=medium,enum=high"`
	Photos     []int      `json:"photos"`
	Evidence   string     `json:"evidence"`
}

type Assessment struct {
	ProfileHash       string            `json:"profile_hash"`
	InputHash         string            `json:"input_hash"`
	Model             string            `json:"model"`
	Score             float64           `json:"score"`
	Coverage          float64           `json:"coverage"`
	MissingEssentials []string          `json:"missing_essentials,omitempty"`
	AvoidsHit         []string          `json:"avoids_hit,omitempty"`
	Dealbreakers      []string          `json:"dealbreakers,omitempty"`
	Summary           string            `json:"summary"`
	Want              []CriterionResult `json:"want"`
	Avoid             []CriterionResult `json:"avoid"`
	AssessedAt        time.Time         `json:"assessed_at"`
	CostUSD           float64           `json:"cost_usd"`
	Tokens            TokenUsage        `json:"tokens"`
	ImagePx           int               `json:"image_px,omitempty"`
}

func (l Listing) Assessment(profileID, model string) (Assessment, bool) {
	a, ok := l.Assessments[profileID][model]
	return a, ok
}

func (l Listing) WithAssessment(profileID string, a Assessment) Listing {
	byProfile := make(map[string]map[string]Assessment, len(l.Assessments)+1)
	for id, byModel := range l.Assessments {
		byProfile[id] = byModel
	}
	byModel := make(map[string]Assessment, len(byProfile[profileID])+1)
	for m, prev := range byProfile[profileID] {
		byModel[m] = prev
	}
	byModel[a.Model] = a
	byProfile[profileID] = byModel
	l.Assessments = byProfile
	return l
}

type TokenUsage struct {
	Prompt     int64 `json:"prompt"`
	Completion int64 `json:"completion"`
	Reasoning  int64 `json:"reasoning"`
	Cached     int64 `json:"cached"`
}

func (t TokenUsage) Add(o TokenUsage) TokenUsage {
	return TokenUsage{
		Prompt:     t.Prompt + o.Prompt,
		Completion: t.Completion + o.Completion,
		Reasoning:  t.Reasoning + o.Reasoning,
		Cached:     t.Cached + o.Cached,
	}
}

func (t TokenUsage) Total() int64 {
	return t.Prompt + t.Completion
}

func (t TokenUsage) String() string {
	out := Count(t.Prompt) + " in"
	if t.Cached > 0 {
		out += " (" + Count(t.Cached) + " cached)"
	}
	out += " / " + Count(t.Completion) + " out"
	if t.Reasoning > 0 {
		out += " (" + Count(t.Reasoning) + " reasoning)"
	}
	return out + " tokens"
}

func Count(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	case n >= 10_000:
		return fmt.Sprintf("%.0fk", float64(n)/1_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fk", float64(n)/1_000)
	}
	return fmt.Sprint(n)
}
