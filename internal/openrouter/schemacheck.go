package openrouter

import (
	"errors"
	"fmt"
)

func (s *Schema) ProviderSafe() error {
	var errs []error
	var walk func(path string, node map[string]any)
	walk = func(path string, node map[string]any) {
		if node["type"] == "integer" || node["type"] == "number" {
			if _, ok := node["enum"]; ok {
				errs = append(errs, fmt.Errorf("%s: enum on %v breaks Gemini structured output", path, node["type"]))
			}
		}
		if _, ok := node["$ref"]; ok {
			errs = append(errs, fmt.Errorf("%s: $ref is not supported by every provider", path))
		}
		if props, ok := node["properties"].(map[string]any); ok {
			if node["additionalProperties"] != false {
				errs = append(errs, fmt.Errorf("%s: strict mode needs additionalProperties false", path))
			}
			if req, _ := node["required"].([]string); len(req) != len(props) {
				errs = append(errs, fmt.Errorf("%s: strict mode needs every property required", path))
			}
			for k, v := range props {
				if c, ok := v.(map[string]any); ok {
					walk(path+"."+k, c)
				}
			}
		}
		if items, ok := node["items"].(map[string]any); ok {
			walk(path+"[]", items)
		}
	}
	walk(s.Name, s.Schema)
	return errors.Join(errs...)
}
