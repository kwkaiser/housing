package openrouter

import (
	"encoding/json"
	"slices"

	"github.com/invopop/jsonschema"
)

func SchemaFor[T any](name string) (*Schema, error) {
	r := jsonschema.Reflector{DoNotReference: true, ExpandedStruct: true, Anonymous: true}
	raw, err := json.Marshal(r.Reflect(new(T)))
	if err != nil {
		return nil, err
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	delete(m, "$schema")
	delete(m, "$id")
	strictify(m)
	return &Schema{Name: name, Schema: m}, nil
}

func strictify(node map[string]any) {
	if props, ok := node["properties"].(map[string]any); ok {
		required := make([]string, 0, len(props))
		for key, child := range props {
			required = append(required, key)
			if c, ok := child.(map[string]any); ok {
				strictify(c)
			}
		}
		slices.Sort(required)
		node["required"] = required
		node["additionalProperties"] = false
	}
	if items, ok := node["items"].(map[string]any); ok {
		strictify(items)
	}
}

func MustSchemaFor[T any](name string) *Schema {
	s, err := SchemaFor[T](name)
	if err != nil {
		panic(err)
	}
	if err := s.ProviderSafe(); err != nil {
		panic(err)
	}
	return s
}
