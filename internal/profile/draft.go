package profile

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"git.kwkaiser.io/kwkaiser/housing/internal/listing"
	"git.kwkaiser.io/kwkaiser/housing/internal/openrouter"
)

const DefaultDraftModel = "anthropic/claude-sonnet-5.5"

var DefaultIgnore = []string{
	"furniture and decor",
	"wall paint colors",
	"tenant belongings",
	"photo quality and staging",
}

type ReferenceInput struct {
	Listing  listing.Listing
	Collages [][]byte
}

type Draft struct {
	Name    string      `json:"name"`
	Summary string      `json:"summary"`
	Want    []Criterion `json:"want"`
	Avoid   []Criterion `json:"avoid"`
	Ignore  []string    `json:"ignore"`
}

type Drafter struct {
	Client openrouter.Completer
	Model  string
}

func (d Drafter) Draft(ctx context.Context, notes []string, refs []ReferenceInput) (Draft, Drafted, error) {
	if len(refs) == 0 {
		return Draft{}, Drafted{}, fmt.Errorf("drafting needs at least one reference listing")
	}

	content := []openrouter.Part{openrouter.TextPart(draftRequestText(notes))}
	for i, ref := range refs {
		content = append(content, openrouter.TextPart(referenceText(i+1, ref.Listing)))
		for j, c := range ref.Collages {
			content = append(content,
				openrouter.TextPart(fmt.Sprintf("Reference %d, collage %d:", i+1, j+1)),
				openrouter.ImagePart("image/jpeg", c),
			)
		}
	}

	temperature := 0.2
	resp, err := d.Client.Complete(ctx, openrouter.Request{
		Model: d.Model,
		Messages: []openrouter.Message{
			{Role: "system", Content: []openrouter.Part{openrouter.TextPart(draftSystemPrompt)}},
			{Role: "user", Content: content},
		},
		ResponseFormat: openrouter.JSONSchemaFormat("housing_profile", draftSchema),
		Temperature:    &temperature,
	})
	if err != nil {
		return Draft{}, Drafted{}, err
	}
	text, err := resp.Text()
	if err != nil {
		return Draft{}, Drafted{}, err
	}

	var draft Draft
	if err := json.Unmarshal([]byte(text), &draft); err != nil {
		return Draft{}, Drafted{}, fmt.Errorf("decode draft: %w", err)
	}
	meta := Drafted{Model: resp.Model, At: time.Now().UTC(), CostUSD: resp.Usage.Cost}
	if meta.Model == "" {
		meta.Model = d.Model
	}
	return draft, meta, nil
}

func (p *Profile) Apply(d Draft, meta Drafted) error {
	next := *p
	if next.Name == "" {
		next.Name = d.Name
	}
	next.Summary = d.Summary
	next.Want = d.Want
	next.Avoid = d.Avoid
	next.Ignore = mergeIgnore(DefaultIgnore, d.Ignore)
	next.Drafted = &meta
	if err := next.Validate(); err != nil {
		return fmt.Errorf("drafted profile is invalid: %w", err)
	}
	*p = next
	return nil
}

func mergeIgnore(base, extra []string) []string {
	out := append([]string{}, base...)
	seen := map[string]bool{}
	for _, s := range out {
		seen[strings.ToLower(s)] = true
	}
	for _, s := range extra {
		if k := strings.ToLower(strings.TrimSpace(s)); k != "" && !seen[k] {
			seen[k] = true
			out = append(out, strings.TrimSpace(s))
		}
	}
	return out
}

func draftRequestText(notes []string) string {
	var b strings.Builder
	b.WriteString("Build a housing preference profile from the reference listing(s) below.\n\n")
	b.WriteString("What I love about the reference listing(s), in my own words:\n")
	if len(notes) == 0 {
		b.WriteString("- (no notes given; infer from the photos and description)\n")
	}
	for _, n := range notes {
		b.WriteString("- " + n + "\n")
	}
	return b.String()
}

func referenceText(n int, l listing.Listing) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Reference %d: %s\n", n, l.Address.Formatted)
	if l.Beds != nil {
		fmt.Fprintf(&b, "Bedrooms: %d\n", *l.Beds)
	}
	if l.Baths != nil {
		fmt.Fprintf(&b, "Bathrooms: %g\n", *l.Baths)
	}
	if l.SqFt != nil {
		fmt.Fprintf(&b, "Square feet: %d\n", *l.SqFt)
	}
	if l.Description != "" {
		fmt.Fprintf(&b, "Description:\n%s\n", l.Description)
	}
	b.WriteString("Photos follow as numbered collages; photo numbers are drawn in each cell's corner.")
	return b.String()
}

const draftSystemPrompt = `You turn a person's favorite apartment into a reusable search profile. The profile will later be used by a vision model to judge OTHER listings from their photo collages and descriptions, so every criterion must be concrete, checkable, and phrased generally (never "like photo 4").

Rules:
- Every note the person wrote must become a "want" criterion. Emphasis such as "!!" or "really" means importance "essential"; other notes are "high".
- You may add up to 4 extra "want" criteria that you clearly see in the reference and that fit the person's taste. Mark them "medium" or "low". Do not add things the person did not ask for if they are merely amenities (appliances, laundry, parking). An extra criterion must not overlap an existing one: if a trait is already covered (e.g. dormers are part of sloped ceilings), fold it into that criterion's "look_for" instead.
- Every criterion, want or avoid, must judge a distinct trait so no single feature of a listing is counted twice.
- Criteria describe the dwelling itself: architecture, materials, light, layout, outdoor space, building age and character, location within the building. Never furniture, decor, paint colors, plants, or tenant belongings.
- "look_for": what a reviewer should see in photos or read in text to mark it present.
- "not_this": common lookalikes that should NOT count (e.g. vinyl plank that imitates hardwood). Use an empty string if none.
- "keywords": short lowercase phrases that would distinguish a matching listing from a typical one. Exclude generic marketing words that appear in most listings regardless of the trait (e.g. "charming", "character", "classic", "bright", "stunning", "must see"). Empty array if the criterion is only visual or no distinctive phrase exists.
- "evidence": "photos" if only visible, "description" if only stated in text, "either" otherwise.
- "avoid": 2 to 5 traits that would clearly clash with this profile, with importance reflecting how badly they clash. An avoid criterion must NOT be the absence or opposite of a want criterion (e.g. if "natural light" is a want, "dark rooms" is not an avoid; if "solid wood floors" is a want, "vinyl plank" belongs in its "not_this", not in an avoid). Avoid criteria cover independent dealbreakers such as basement units, drop ceilings, or a unit facing a highway.
- "ignore": things a reviewer should disregard when judging listings for this person. Never list anything that a want or avoid criterion depends on (e.g. do not ignore kitchen fixtures when butcher block counters are wanted). Do not repeat furniture, decor, paint colors, tenant belongings, or photo staging; those are already ignored.
- ids are short unique snake_case strings.
- "name" is a short title for the profile; "summary" is one or two sentences describing the overall vibe.`

var draftSchema = json.RawMessage(`{
  "type": "object",
  "additionalProperties": false,
  "required": ["name", "summary", "want", "avoid", "ignore"],
  "properties": {
    "name": {"type": "string"},
    "summary": {"type": "string"},
    "want": {"type": "array", "items": {"$ref": "#/$defs/criterion"}},
    "avoid": {"type": "array", "items": {"$ref": "#/$defs/criterion"}},
    "ignore": {"type": "array", "items": {"type": "string"}}
  },
  "$defs": {
    "criterion": {
      "type": "object",
      "additionalProperties": false,
      "required": ["id", "label", "look_for", "not_this", "keywords", "importance", "evidence"],
      "properties": {
        "id": {"type": "string"},
        "label": {"type": "string"},
        "look_for": {"type": "string"},
        "not_this": {"type": "string"},
        "keywords": {"type": "array", "items": {"type": "string"}},
        "importance": {"type": "string", "enum": ["essential", "high", "medium", "low"]},
        "evidence": {"type": "string", "enum": ["photos", "description", "either"]}
      }
    }
  }
}`)
