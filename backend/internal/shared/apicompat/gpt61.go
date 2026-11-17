package apicompat

import "strings"

func isGPT61Or62CompatModel(model string) bool {
	m := strings.ToLower(strings.TrimSpace(model))
	if i := strings.LastIndexByte(m, '/'); i >= 0 {
		m = m[i+1:]
	}
	m = strings.ReplaceAll(m, "_", "-")
	return m == "gpt-6.2-astra" || m == "gpt-6.2-sol" || m == "gpt-6.1-sol" || m == "gpt-6.1-terra"
}
