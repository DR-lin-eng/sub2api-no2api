package service

import "strings"

// geminiTextDelta tracks the text already emitted for one stream block. Only the
// storage differs from concatenating strings: appends are amortized, so
// delta-mode streams no longer copy the whole prefix on every chunk.
type geminiTextDelta struct {
	seen strings.Builder
}

// next returns the part of incoming that has not been emitted yet.
func (d *geminiTextDelta) next(incoming string) string {
	incoming = strings.TrimSuffix(incoming, "\u0000")
	if incoming == "" {
		return ""
	}
	seen := d.seen.String()

	// Cumulative mode: incoming contains full text so far.
	if strings.HasPrefix(incoming, seen) {
		delta := incoming[len(seen):]
		_, _ = d.seen.WriteString(delta)
		return delta
	}
	// Duplicate/rewind: ignore.
	if strings.HasPrefix(seen, incoming) {
		return ""
	}
	// Delta mode: treat incoming as incremental chunk.
	_, _ = d.seen.WriteString(incoming)
	return incoming
}

func (d *geminiTextDelta) reset() {
	d.seen.Reset()
}

func computeGeminiTextDelta(seen, incoming string) (delta, newSeen string) {
	incoming = strings.TrimSuffix(incoming, "\u0000")
	if incoming == "" {
		return "", seen
	}

	// Cumulative mode: incoming contains full text so far.
	if strings.HasPrefix(incoming, seen) {
		return strings.TrimPrefix(incoming, seen), incoming
	}
	// Duplicate/rewind: ignore.
	if strings.HasPrefix(seen, incoming) {
		return "", seen
	}
	// Delta mode: treat incoming as incremental chunk.
	return incoming, seen + incoming
}
