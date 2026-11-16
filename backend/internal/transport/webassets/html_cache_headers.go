//go:build embed || unit

package web

import "net/http"

// applyHTMLNoCacheHeaders prevents browser and shared caches from storing
// settings-injected HTML. Hashed static assets keep their independent cache policy.
func applyHTMLNoCacheHeaders(header http.Header) {
	header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
	header.Set("Pragma", "no-cache")
	header.Set("Expires", "0")
	header.Set("CDN-Cache-Control", "no-store")
	header.Set("Surrogate-Control", "no-store")
}
