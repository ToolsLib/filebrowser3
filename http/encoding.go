package fbhttp

import (
	"strconv"
	"strings"
)

// chooseEncoding respects explicit exclusions and wildcard quality values.
// The empty result selects the original representation.
func chooseEncoding(header string, available ...string) string {
	qualities := make(map[string]float64)
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(strings.TrimSpace(part), ";")
		name := strings.ToLower(strings.TrimSpace(fields[0]))
		if name == "" {
			continue
		}
		q := 1.0
		for _, parameter := range fields[1:] {
			kv := strings.SplitN(strings.TrimSpace(parameter), "=", 2)
			if len(kv) == 2 && strings.EqualFold(kv[0], "q") {
				value, err := strconv.ParseFloat(kv[1], 64)
				if err != nil || value < 0 || value > 1 {
					q = 0
				} else {
					q = value
				}
			}
		}
		qualities[name] = q
	}
	best := ""
	score := 0.0
	for _, coding := range available {
		q, ok := qualities[coding]
		if !ok {
			q = qualities["*"]
		}
		if q > score {
			best = coding
			score = q
		}
	}
	if identity, ok := qualities["identity"]; ok && identity > score {
		return ""
	}
	if best != "" {
		return best
	}
	if q, ok := qualities["identity"]; ok && q == 0 {
		return "not-acceptable"
	}
	if q, ok := qualities["*"]; ok && q == 0 {
		if _, explicit := qualities["identity"]; !explicit {
			return "not-acceptable"
		}
	}
	return ""
}
