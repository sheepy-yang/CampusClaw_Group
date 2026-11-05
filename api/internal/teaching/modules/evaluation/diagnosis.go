package evaluation

func diagnosisHasEvidence(diagnosis map[string]any) bool {
	if diagnosis == nil {
		return false
	}
	if evidenceCount(diagnosis["evidenceEvents"]) < MinDiagnosisSamples {
		return false
	}
	switch mastery := diagnosis["mastery"].(type) {
	case []map[string]any:
		for _, item := range mastery {
			if evidenceCount(item["evidence"]) == 0 {
				return false
			}
		}
	case []any:
		for _, raw := range mastery {
			item, ok := raw.(map[string]any)
			if !ok || evidenceCount(item["evidence"]) == 0 {
				return false
			}
		}
	default:
		return false
	}
	return true
}

func evidenceCount(value any) int {
	switch values := value.(type) {
	case []string:
		return len(values)
	case []any:
		return len(values)
	case []map[string]any:
		return len(values)
	default:
		return 0
	}
}
