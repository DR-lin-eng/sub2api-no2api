package apicompat

import "strings"

func isGPT61CompatModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndexByte(m, '/'); i >= 0 {
		m = m[i+1:]
	}
	m = strings.ReplaceAll(m, "_", "-")
	return m == "gpt-6.1-sol" || m == "gpt-6.1-terra"
}
