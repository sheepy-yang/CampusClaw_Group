package mcp

import (
	"fmt"
	"math"
	"reflect"
	"unicode/utf8"
)

// ValidateSchema validates the JSON-Schema subset emitted by this server.
// Unsupported schema keywords are not silently treated as authorization.
func ValidateSchema(schema map[string]any, value any) error {
	return validateValue("$", schema, value)
}

func validateValue(path string, schema map[string]any, value any) error {
	schemaType, _ := schema["type"].(string)
	switch schemaType {
	case "object":
		object, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("%s must be an object", path)
		}
		properties, _ := schema["properties"].(map[string]any)
		for _, name := range schemaStrings(schema["required"]) {
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
			rawProperty, ok := properties[name]
			if !ok {
				continue
			}
			property, ok := rawProperty.(map[string]any)
			if !ok {
				return fmt.Errorf("schema for %s.%s is invalid", path, name)
			}
			if err := validateValue(path+"."+name, property, item); err != nil {
				return err
			}
		}
	case "array":
		array, ok := value.([]any)
		if !ok {
			return fmt.Errorf("%s must be an array", path)
		}
		if minimum, ok := schemaNumber(schema["minItems"]); ok && float64(len(array)) < minimum {
			return fmt.Errorf("%s must contain at least %d items", path, int(minimum))
		}
		if maximum, ok := schemaNumber(schema["maxItems"]); ok && float64(len(array)) > maximum {
			return fmt.Errorf("%s must contain at most %d items", path, int(maximum))
		}
		if rawItems, ok := schema["items"]; ok {
			items, ok := rawItems.(map[string]any)
			if !ok {
				return fmt.Errorf("schema for %s items is invalid", path)
			}
			for index, item := range array {
				if err := validateValue(fmt.Sprintf("%s[%d]", path, index), items, item); err != nil {
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
		if minimum, ok := schemaNumber(schema["minLength"]); ok && float64(length) < minimum {
			return fmt.Errorf("%s is too short", path)
		}
		if maximum, ok := schemaNumber(schema["maxLength"]); ok && float64(length) > maximum {
			return fmt.Errorf("%s is too long", path)
		}
	case "number":
		number, ok := jsonNumber(value)
		if !ok || math.IsNaN(number) || math.IsInf(number, 0) {
			return fmt.Errorf("%s must be a finite number", path)
		}
		if minimum, ok := schemaNumber(schema["minimum"]); ok && number < minimum {
			return fmt.Errorf("%s must be at least %v", path, minimum)
		}
		if maximum, ok := schemaNumber(schema["maximum"]); ok && number > maximum {
			return fmt.Errorf("%s must be at most %v", path, maximum)
		}
	case "":
		return fmt.Errorf("schema for %s has no type", path)
	default:
		return fmt.Errorf("schema for %s uses unsupported type %q", path, schemaType)
	}
	return nil
}

func schemaStrings(value any) []string {
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

func schemaNumber(value any) (float64, bool) {
	if value == nil {
		return 0, false
	}
	return jsonNumber(value)
}

func jsonNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int8:
		return float64(number), true
	case int16:
		return float64(number), true
	case int32:
		return float64(number), true
	case int64:
		return float64(number), true
	case uint:
		return float64(number), true
	case uint8:
		return float64(number), true
	case uint16:
		return float64(number), true
	case uint32:
		return float64(number), true
	case uint64:
		if number > 1<<53 {
			return 0, false
		}
		return float64(number), true
	default:
		value := reflect.ValueOf(value)
		if value.IsValid() && value.Kind() == reflect.Float64 {
			return value.Float(), true
		}
		return 0, false
	}
}
