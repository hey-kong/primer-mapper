package common

import "strconv"

// Itos converts an interface to a string.
func Itos(value interface{}) (s string) {
	switch t := value.(type) {
	case nil:
		s = ""
	case int:
		s = strconv.Itoa(t)
	case float64:
		s = strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			s = "true"
		} else {
			s = "false"
		}
	case string:
		s = t
	default:
		s = ""
	}

	return s
}
