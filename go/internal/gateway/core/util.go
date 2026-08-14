package core

import (
	"net/http"
	"strconv"
)

func intFrom(v any, def int) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case int64:
		return int(t)
	case string:
		n, err := strconv.Atoi(t)
		if err == nil {
			return n
		}
	}
	return def
}

// queryInt returns a positive query int clamped to [min, max], or 0 if absent/invalid.
func queryInt(r *http.Request, name string, min, max int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		return 0
	}
	return n
}
