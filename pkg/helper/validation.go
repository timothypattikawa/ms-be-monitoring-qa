package helper

import "strings"

// IsEmpty will checks is value of argument s was empty.
func IsEmpty[T Validateable](s T) bool {
	switch v := any(s).(type) {
	default:
		return true
	case string:
		return len(strings.TrimSpace(v)) == 0
	case int:
		return v == 0
	case int8:
		return v == 0
	case int16:
		return v == 0
	case int32:
		return v == 0
	case int64:
		return v == 0
	case uint:
		return v == 0
	case uint8:
		return v == 0
	case uint16:
		return v == 0
	case uint32:
		return v == 0
	case uint64:
		return v == 0
	case float32:
		return v == 0
	case float64:
		return v == 0
	case []any:
		return len(v) == 0
	case []string:
		return len(v) == 0
	case map[string]string:
		return len(v) == 0
	case map[string]any:
		return len(v) == 0
	case map[any]any:
		return len(v) == 0
	case any:
		return v == nil
	}
}
