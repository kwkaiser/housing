package listing

import "time"

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
	Verdict    Verdict    `json:"verdict"`
	Confidence Confidence `json:"confidence"`
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
	Vibe              int               `json:"vibe"`
	Summary           string            `json:"summary"`
	Want              []CriterionResult `json:"want"`
	Avoid             []CriterionResult `json:"avoid"`
	AssessedAt        time.Time         `json:"assessed_at"`
	CostUSD           float64           `json:"cost_usd"`
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
