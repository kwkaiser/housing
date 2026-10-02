package profile

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"git.kwkaiser.io/kwkaiser/housing/internal/digest"
	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
)

const (
	DefaultAssessModel           = "google/gemini-3.8-flash"
	DefaultAssessReasoningEffort = "low"
)

var importanceWeights = map[Importance]float64{
	Essential: 4,
	High:      3,
	Medium:    2,
	Low:       1,
}

var verdictValues = map[listing.Verdict]float64{
	listing.VerdictPresent: 1,
	listing.VerdictPartial: 0.5,
	listing.VerdictAbsent:  0,
	listing.VerdictUnknown: 0,
}

func (p Profile) Hash() string {
	b, _ := json.Marshal(struct {
		Summary    string
		Want       []Criterion
		Avoid      []Criterion
		Ignore     []string
		References []Reference
	}{p.Summary, p.Want, p.Avoid, p.Ignore, p.References})
	return digest.Hex(b)
}

func InputHash(l listing.Listing) string {
	return digest.String(strings.Join(l.Collages, "\n") + "\n\n" + l.Description)
}

type Candidate struct {
	Listing  listing.Listing
	Collages [][]byte
}

type Assessor struct {
	Client          openrouter.Completer
	Model           string
	Attempts        int
	ImagePx         int
	ReasoningEffort string
}

func usage(u openrouter.Usage) listing.TokenUsage {
	return listing.TokenUsage{Prompt: u.PromptTokens, Completion: u.CompletionTokens, Reasoning: u.ReasoningTokens, Cached: u.CachedTokens}
}

const DefaultAssessAttempts = 3

type modelResponse struct {
	Want    []listing.CriterionResult `json:"want"`
	Avoid   []listing.CriterionResult `json:"avoid"`
	Summary string                    `json:"summary"`
}

func (a Assessor) Assess(ctx context.Context, p Profile, refs []ReferenceInput, c Candidate) (listing.Assessment, error) {
	if p.IsAvoid() {
		return listing.Assessment{}, fmt.Errorf("%q is an avoid profile and cannot be assessed against directly", p.ID)
	}
	if len(p.Want) == 0 {
		return listing.Assessment{}, fmt.Errorf("profile %q has no criteria; re-draft it from its profile page first", p.ID)
	}
	if len(c.Collages) == 0 {
		return listing.Assessment{}, fmt.Errorf("listing %s/%s has no collages", c.Listing.Source, c.Listing.SourceID)
	}

	content := []openrouter.Part{openrouter.TextPart(profileText(p))}
	wantN, avoidN := 0, 0
	for _, ref := range refs {
		label := ""
		if ref.Avoid {
			avoidN++
			label = fmt.Sprintf("AVOID EXAMPLE %d (a listing the person dislikes, shown for calibrating the AVOID criteria; do not grade it):", avoidN)
		} else {
			wantN++
			label = fmt.Sprintf("REFERENCE listing %d (the person's favorite, shown for calibration only; do not grade it):", wantN)
		}
		content = append(content, openrouter.TextPart(label))
		for _, img := range ref.Collages {
			content = append(content, openrouter.ImagePart("image/jpeg", img))
		}
	}
	content = append(content, openrouter.TextPart(candidateText(c.Listing)))
	for i, img := range c.Collages {
		content = append(content,
			openrouter.TextPart(fmt.Sprintf("CANDIDATE collage %d:", i+1)),
			openrouter.ImagePart("image/jpeg", img),
		)
	}

	var (
		resp   openrouter.Response
		mr     modelResponse
		cost   float64
		tokens listing.TokenUsage
		err    error
	)
	for attempt := range max(a.Attempts, 1) {
		if attempt > 0 && ctx.Err() != nil {
			return listing.Assessment{CostUSD: cost, Tokens: tokens}, ctx.Err()
		}
		resp, mr, err = a.complete(ctx, p, content)
		cost += resp.CostUSD
		tokens = tokens.Add(usage(resp.Usage))
		if err == nil || !errors.Is(err, errUnusable) {
			break
		}
	}
	if err != nil {
		return listing.Assessment{CostUSD: cost, Tokens: tokens}, err
	}
	resp.CostUSD = cost

	out := listing.Assessment{
		ProfileHash: p.Hash(),
		InputHash:   InputHash(c.Listing),
		Model:       a.Model,
		Summary:     mr.Summary,
		Want:        align(p.Want, mr.Want),
		Avoid:       align(p.Avoid, mr.Avoid),
		AssessedAt:  time.Now().UTC(),
		CostUSD:     resp.CostUSD,
		Tokens:      tokens,
		ImagePx:     a.ImagePx,
	}
	Score(p, &out)
	return out, nil
}

var errUnusable = errors.New("unusable model output")

func (a Assessor) complete(ctx context.Context, p Profile, content []openrouter.Part) (openrouter.Response, modelResponse, error) {
	temperature := 0.0
	resp, err := a.Client.Complete(ctx, openrouter.Request{
		Model:           a.Model,
		System:          assessSystemPrompt,
		User:            content,
		Schema:          assessSchema,
		Temperature:     &temperature,
		ReasoningEffort: a.ReasoningEffort,
	})
	if err != nil {
		return resp, modelResponse{}, err
	}
	text, err := resp.Text()
	if err != nil {
		return resp, modelResponse{}, fmt.Errorf("%w: %w", errUnusable, err)
	}
	var mr modelResponse
	if err := json.Unmarshal([]byte(text), &mr); err != nil {
		return resp, modelResponse{}, fmt.Errorf("%w: decode assessment: %w", errUnusable, err)
	}
	if err := checkResponse(p, mr); err != nil {
		return resp, modelResponse{}, fmt.Errorf("%w: %s returned an unusable assessment: %w: %.200s", errUnusable, a.Model, err, text)
	}
	return resp, mr, nil
}

func checkResponse(p Profile, mr modelResponse) error {
	ids := map[string]bool{}
	for _, r := range append(append([]listing.CriterionResult{}, mr.Want...), mr.Avoid...) {
		ids[r.ID] = true
	}
	matched := 0
	for _, c := range append(append([]Criterion{}, p.Want...), p.Avoid...) {
		if ids[c.ID] {
			matched++
		}
	}
	if total := len(p.Want) + len(p.Avoid); matched*2 < total {
		return fmt.Errorf("only %d of %d criteria graded", matched, total)
	}
	return nil
}

func align(criteria []Criterion, results []listing.CriterionResult) []listing.CriterionResult {
	byID := make(map[string]listing.CriterionResult, len(results))
	for _, r := range results {
		byID[r.ID] = r
	}
	out := make([]listing.CriterionResult, len(criteria))
	for i, c := range criteria {
		r, ok := byID[c.ID]
		if _, valid := verdictValues[r.Verdict]; !ok || !valid {
			r = listing.CriterionResult{ID: c.ID, Verdict: listing.VerdictUnknown, Confidence: listing.ConfidenceLow, Evidence: "not assessed"}
		}
		if r.Photos == nil {
			r.Photos = []int{}
		}
		out[i] = r
	}
	return out
}

func Score(p Profile, a *listing.Assessment) {
	var total, earned, known float64
	a.MissingEssentials, a.AvoidsHit, a.Dealbreakers = nil, nil, nil
	for i, c := range p.Want {
		w := importanceWeights[c.Importance]
		r := a.Want[i]
		total += w
		earned += w * verdictValues[r.Verdict]
		if r.Verdict != listing.VerdictUnknown {
			known += w
		}
		if c.Importance == Essential && r.Verdict == listing.VerdictAbsent {
			a.MissingEssentials = append(a.MissingEssentials, c.ID)
		}
	}

	var penalty float64
	for i, c := range p.Avoid {
		r := a.Avoid[i]
		v := verdictValues[r.Verdict]
		penalty += importanceWeights[c.Importance] * v
		if v > 0 {
			a.AvoidsHit = append(a.AvoidsHit, c.ID)
		}
		if c.Importance == Essential && r.Verdict == listing.VerdictPresent {
			a.Dealbreakers = append(a.Dealbreakers, c.ID)
		}
	}

	if total == 0 {
		a.Score, a.Coverage = 0, 0
		return
	}
	a.Score = round1(100 * math.Max(0, (earned-penalty)/total))
	a.Coverage = round1(100 * known / total)
}

func round1(f float64) float64 {
	return math.Round(f*10) / 10
}

func profileText(p Profile) string {
	type criterion struct {
		ID       string   `json:"id"`
		Label    string   `json:"label"`
		LookFor  string   `json:"look_for"`
		NotThis  string   `json:"not_this,omitempty"`
		Keywords []string `json:"keywords,omitempty"`
		Evidence Evidence `json:"evidence"`
	}
	conv := func(cs []Criterion) []criterion {
		out := make([]criterion, len(cs))
		for i, c := range cs {
			out[i] = criterion{c.ID, c.Label, c.LookFor, c.NotThis, c.Keywords, c.Evidence}
		}
		return out
	}
	want, _ := json.MarshalIndent(conv(p.Want), "", "  ")
	avoid, _ := json.MarshalIndent(conv(p.Avoid), "", "  ")

	var b strings.Builder
	fmt.Fprintf(&b, "PROFILE: %s\n%s\n\n", p.Name, p.Summary)
	fmt.Fprintf(&b, "WANT criteria:\n%s\n\nAVOID criteria:\n%s\n\n", want, avoid)
	fmt.Fprintf(&b, "IGNORE when judging: %s\n", strings.Join(p.Ignore, "; "))
	return b.String()
}

func candidateText(l listing.Listing) string {
	var b strings.Builder
	b.WriteString("CANDIDATE listing to grade:\n")
	fmt.Fprintf(&b, "Address: %s\n", l.Address.Formatted)
	if l.Offer != "" {
		fmt.Fprintf(&b, "Offer: %s\n", l.Offer)
	}
	if l.Beds != nil {
		fmt.Fprintf(&b, "Bedrooms: %d\n", *l.Beds)
	}
	if l.Baths != nil {
		fmt.Fprintf(&b, "Bathrooms: %g\n", *l.Baths)
	}
	if l.SqFt != nil {
		fmt.Fprintf(&b, "Square feet: %d\n", *l.SqFt)
	}
	if l.Address.Unit != "" {
		fmt.Fprintf(&b, "Unit: %s\n", l.Address.Unit)
	}
	if l.Description != "" {
		fmt.Fprintf(&b, "Description:\n%s\n", l.Description)
	} else {
		b.WriteString("Description: (none available)\n")
	}
	b.WriteString("Candidate photos follow as numbered collages; cite photo numbers from the CANDIDATE collages only.")
	return b.String()
}

const assessSystemPrompt = `You grade a rental or for-sale listing against a person's housing profile. You will receive the profile's criteria, collages of the person's REFERENCE listing (for calibrating what they want, never to be graded), possibly collages of AVOID EXAMPLES (listings the person dislikes, for calibrating the AVOID criteria, never to be graded), and then the CANDIDATE listing's facts, description and numbered photo collages.

Grade only the CANDIDATE, only from its own photos and text. Never credit or penalize the candidate for something you saw in a reference or avoid example.

For every WANT and every AVOID criterion return exactly one result with the criterion's id:
- "present": clearly visible in candidate photos or explicitly stated in its description, and not one of the criterion's "not_this" lookalikes.
- "partial": a weaker or limited version (e.g. one small skylight in a bathroom, a small balcony instead of a porch), or plausible but ambiguous evidence.
- "absent": the relevant spaces are shown or described and the trait is clearly not there, or only a "not_this" lookalike is.
- "unknown": the photos and description do not show enough to tell. Prefer "unknown" over guessing.
Respect each criterion's "evidence" field: "photos" criteria need visual evidence, "description" criteria need text evidence, "either" accepts both.

"confidence" is how sure you are of the verdict. "photos" lists the candidate photo numbers (drawn in each cell's corner) that support the verdict; use an empty list when the evidence is textual or absent. "evidence" is a terse phrase of at most 12 words naming what decided the verdict (e.g. "two skylights over the bed" or "listing says third floor"); use an empty string when the verdict is "unknown".

"summary" is one sentence of at most 30 words.

Disregard everything in the profile's IGNORE list, and never judge furniture, decor, paint colors or tenant belongings.`

var assessSchema = openrouter.MustSchemaFor[modelResponse]("listing_assessment")

type BatchOptions struct {
	Force           bool
	Limit           int
	Concurrency     int
	MaxCostUSD      float64
	Budget          *Budget
	Checkpoint      func([]listing.Listing) error
	CheckpointEvery int
	Observe         func(BatchEvent)
}

type BatchEventKind string

const (
	BatchPlanned    BatchEventKind = "planned"
	BatchStarted    BatchEventKind = "started"
	BatchGraded     BatchEventKind = "graded"
	BatchFailed     BatchEventKind = "failed"
	BatchOverBudget BatchEventKind = "over_budget"
)

type BatchEvent struct {
	Kind       BatchEventKind
	Listing    listing.Listing
	Units      int
	N          int
	Total      int
	Assessment listing.Assessment
	Err        error
	Elapsed    time.Duration
	Stats      BatchStats
}

func (o BatchOptions) observe(e BatchEvent) {
	if o.Observe != nil {
		o.Observe(e)
	}
}

type BatchStats struct {
	Calls      int
	Updated    int
	Cached     int
	NoCollages int
	OverLimit  int
	OverBudget int
	Failed     int
	CostUSD    float64
	Tokens     listing.TokenUsage
}

func (a Assessor) Current(p Profile, l listing.Listing) bool {
	prev, ok := l.Assessment(p.ID, a.Model)
	return ok && prev.ProfileHash == p.Hash() && prev.InputHash == InputHash(l)
}

func (a Assessor) AssessListings(
	ctx context.Context,
	p Profile,
	refs []ReferenceInput,
	listings []listing.Listing,
	readCollages func(keys []string) ([][]byte, error),
	opts BatchOptions,
) ([]listing.Listing, BatchStats, error) {
	var stats BatchStats
	out := make([]listing.Listing, len(listings))
	copy(out, listings)

	var order []string
	groups := map[string][]int{}
	for i, l := range out {
		switch {
		case len(l.Collages) == 0:
			stats.NoCollages++
		case !opts.Force && a.Current(p, l):
			stats.Cached++
		default:
			key := InputHash(l)
			if _, ok := groups[key]; !ok {
				order = append(order, key)
			}
			groups[key] = append(groups[key], i)
		}
	}

	if opts.Limit > 0 && len(order) > opts.Limit {
		for _, key := range order[opts.Limit:] {
			stats.OverLimit += len(groups[key])
		}
		order = order[:opts.Limit]
	}

	type unit struct {
		kind    BatchEventKind
		idx     int
		members []int
		result  listing.Assessment
		err     error
		elapsed time.Duration
	}
	limit := max(opts.Concurrency, 1)
	units := make(chan unit, limit)
	spent := NewBudget(opts.MaxCostUSD)
	go func() {
		defer close(units)
		var g errgroup.Group
		g.SetLimit(limit)
		for idx, key := range order {
			members := groups[key]
			g.Go(func() error {
				if ctx.Err() != nil {
					return nil
				}
				if spent.Exhausted() || opts.Budget.Exhausted() {
					units <- unit{kind: BatchOverBudget, idx: idx, members: members}
					return nil
				}
				units <- unit{kind: BatchStarted, idx: idx, members: members}
				begin := time.Now()
				result, err := a.assessOne(ctx, p, refs, listings[members[0]], readCollages)
				spent.Spend(result.CostUSD)
				opts.Budget.Spend(result.CostUSD)
				opts.Budget.Record(result.Tokens)
				kind := BatchGraded
				if err != nil {
					kind = BatchFailed
				}
				units <- unit{kind: kind, idx: idx, members: members, result: result, err: err, elapsed: time.Since(begin)}
				return nil
			})
		}
		g.Wait()
	}()

	var (
		errs       []error
		started    int
		budgetSeen bool
		ordinal    = map[int]int{}
	)
	opts.observe(BatchEvent{Kind: BatchPlanned, Total: len(order), Stats: stats})
	for u := range units {
		first := listings[u.members[0]]
		switch u.kind {
		case BatchOverBudget:
			stats.OverBudget += len(u.members)
			if !budgetSeen {
				budgetSeen = true
				opts.observe(BatchEvent{Kind: BatchOverBudget, Total: len(order), N: started, Stats: stats})
			}
		case BatchStarted:
			started++
			ordinal[u.idx] = started
			opts.observe(BatchEvent{Kind: BatchStarted, Listing: first, Units: len(u.members), N: started, Total: len(order)})
		case BatchFailed:
			stats.CostUSD += u.result.CostUSD
			stats.Tokens = stats.Tokens.Add(u.result.Tokens)
			stats.Failed += len(u.members)
			errs = append(errs, fmt.Errorf("assess %s/%s: %w", first.Source, first.SourceID, u.err))
			opts.observe(BatchEvent{Kind: BatchFailed, Listing: first, Units: len(u.members), N: ordinal[u.idx], Total: len(order), Err: u.err, Elapsed: u.elapsed, Assessment: u.result, Stats: stats})
		case BatchGraded:
			stats.CostUSD += u.result.CostUSD
			stats.Tokens = stats.Tokens.Add(u.result.Tokens)
			stats.Calls++
			for _, i := range u.members {
				out[i] = out[i].WithAssessment(p.ID, u.result)
				stats.Updated++
			}
			opts.observe(BatchEvent{Kind: BatchGraded, Listing: first, Units: len(u.members), N: ordinal[u.idx], Total: len(order), Assessment: u.result, Elapsed: u.elapsed, Stats: stats})
			if opts.Checkpoint != nil && opts.CheckpointEvery > 0 && stats.Calls%opts.CheckpointEvery == 0 {
				if err := opts.Checkpoint(slices.Clone(out)); err != nil {
					errs = append(errs, fmt.Errorf("checkpoint: %w", err))
				}
			}
		}
	}
	if err := ctx.Err(); err != nil {
		errs = append(errs, err)
	}
	return out, stats, errors.Join(errs...)
}

func (a Assessor) assessOne(ctx context.Context, p Profile, refs []ReferenceInput, l listing.Listing, readCollages func([]string) ([][]byte, error)) (listing.Assessment, error) {
	collages, err := readCollages(l.Collages)
	if err != nil {
		return listing.Assessment{}, err
	}
	return a.Assess(ctx, p, refs, Candidate{Listing: l, Collages: collages})
}
