package openrouter

import (
	"encoding/json"
	"strings"
	"testing"
)

type schemaInner struct {
	Verdict string `json:"verdict" jsonschema:"enum=present,enum=absent"`
	Photos  []int  `json:"photos,omitempty"`
}

type schemaOuter struct {
	Name  string        `json:"name"`
	Vibe  int           `json:"vibe" jsonschema:"description=1 to 5"`
	Items []schemaInner `json:"items"`
}

func TestSchemaFor(t *testing.T) {
	s, err := SchemaFor[schemaOuter]("out")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(s.Schema)
	got := string(b)
	for _, bad := range []string{"$ref", "$defs", "$schema", "$id"} {
		if strings.Contains(got, bad) {
			t.Errorf("schema contains %s: %s", bad, got)
		}
	}
	for _, want := range []string{
		`"required":["items","name","vibe"]`,
		`"required":["photos","verdict"]`,
		`"additionalProperties":false`,
		`"enum":["present","absent"]`,
		`"description":"1 to 5"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("schema missing %s: %s", want, got)
		}
	}
	if err := s.ProviderSafe(); err != nil {
		t.Error(err)
	}

	bad := &Schema{Name: "bad", Schema: map[string]any{"type": "object", "properties": map[string]any{"vibe": map[string]any{"type": "integer", "enum": []int{1, 2}}}}}
	if bad.ProviderSafe() == nil {
		t.Error("integer enum and non-strict object should be rejected")
	}
}
