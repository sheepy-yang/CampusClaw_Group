package tutoring

import (
	"fmt"
	"math"
	"unicode/utf8"
)

func validateJSONSchema(schema map[string]any, value any) error {
	return validateJSONValue("$", schema, value)
}

func validateJSONValue(path string, schema map[string]any, value any) error {
	schemaType, _ := schema["type"].(string)
	switch schemaType {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		properties, ok := schema["properties"].(map[string]any)
		if !ok {
			return fmt.Errorf("schema for %s has invalid properties", path)
		}
		for _, name := range schemaStringList(schema["required"]) {
			if _, ok := object[name]; !ok {
				return fmt.Errorf("%s.%s is required", path, name)
			}
		}
		if additional, ok := schema["additionalProperties"].(bool); ok && !additional {
			for name := range object {
				if _, ok := properties[name]; !ok {
					return fmt.Errorf("%s.%s is not allowed", path, name)
				}
			}
		}
		for name, item := range object {
			rawProperty, exists := properties[name]
			if !exists {
				continue
			}
			property, ok := rawProperty.(map[string]any)
			if !ok {
				return fmt.Errorf("schema for %s.%s is invalid", path, name)
			}
			if err := validateJSONValue(path+"."+name, property, item); err != nil {
				return err
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if minimum, ok := JSONNumber(schema["minItems"]); ok && float64(len(array)) < minimum {
			return fmt.Errorf("%s has too few items", path)
		}
		if maximum, ok := JSONNumber(schema["maxItems"]); ok && float64(len(array)) > maximum {
			return fmt.Errorf("%s has too many items", path)
		}
		if rawItems, ok := schema["items"]; ok {
			items, ok := rawItems.(map[string]any)
			if !ok {
				return fmt.Errorf("schema for %s items is invalid", path)
			}
			for index, item := range array {
				if err := validateJSONValue(fmt.Sprintf("%s[%d]", path, index), items, item); err != nil {
					return err
				}
			}
		}
	case "string":
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s must be a string", path)
		}
		length := utf8.RuneCountInString(text)
		if minimum, ok := JSONNumber(schema["minLength"]); ok && float64(length) < minimum {
			return fmt.Errorf("%s is too short", path)
		}
		if maximum, ok := JSONNumber(schema["maxLength"]); ok && float64(length) > maximum {
			return fmt.Errorf("%s is too long", path)
		}
	case "number":
		number, ok := JSONNumber(value)
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("%s must be a finite number", path)
		}
		if minimum, ok := JSONNumber(schema["minimum"]); ok && number < minimum {
			return fmt.Errorf("%s is below the minimum", path)
		}
		if maximum, ok := JSONNumber(schema["maximum"]); ok && number > maximum {
			return fmt.Errorf("%s exceeds the maximum", path)
		}
	case "":
		return fmt.Errorf("schema for %s has no type", path)
	default:
		return fmt.Errorf("schema for %s uses unsupported type %q", path, schemaType)
	}
	return nil
}

func schemaStringList(value any) []string {
	switch values := value.(type) {
	case []string:
		return values
	case []any:
		result := make([]string, 0, len(values))
		for _, value := range values {
			if text, ok := value.(string); ok {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

// JSONNumber converts the numeric forms used by encoding/json and tests.
func JSONNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		if number > 1<<53 {
			return 0, false
		}
		return float64(number), true
	default:
		return 0, false
	}
}
