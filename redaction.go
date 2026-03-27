package aegis

import (
	"encoding/json"
	"strings"
)

func applyRedactions(output any, rules []RedactionRule) (any, error) {
	if len(rules) == 0 || output == nil {
		return output, nil
	}

	payload, err := json.Marshal(output)
	if err != nil {
		return output, err
	}

	var decoded any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		return output, err
	}

	for _, rule := range rules {
		applyRedactionRule(&decoded, rule)
	}

	return decoded, nil
}

func applyRedactionRule(target *any, rule RedactionRule) {
	segments := strings.Split(strings.TrimPrefix(rule.Path, "$."), ".")
	redactSegment(target, segments, rule.Mode)
}

func redactSegment(target *any, segments []string, mode string) {
	if len(segments) == 0 || target == nil || *target == nil {
		return
	}

	switch current := (*target).(type) {
	case map[string]any:
		key := segments[0]
		value, ok := current[key]
		if !ok {
			return
		}

		if len(segments) == 1 {
			switch mode {
			case "remove":
				delete(current, key)
			default:
				current[key] = "[redacted]"
			}
			return
		}

		redactSegment(&value, segments[1:], mode)
		current[key] = value
	case []any:
		for index := range current {
			value := current[index]
			redactSegment(&value, segments, mode)
			current[index] = value
		}
	}
}
