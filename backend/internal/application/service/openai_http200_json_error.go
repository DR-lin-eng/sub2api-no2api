package service

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// A few compatible relays return a JSON error with HTTP 200 even when SSE was
// requested. Adapt only a complete, bounded generic failure to an SSE event so
// it reaches the same pre-output recovery classifier. Initialize inside Read:
// the existing scanner watchdog/keepalive still owns slow upstream reads.
func (s *OpenAIGatewayService) adaptOpenAIHTTP200JSONError(resp *http.Response) {
	if resp == nil || resp.StatusCode != http.StatusOK || resp.Body == nil ||
		!strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "application/json") {
		return
	}
	resp.Body = &openAIHTTP200JSONErrorBody{ReadCloser: resp.Body, limit: openAIUpstreamErrorBodyReadLimitForConfig(s.cfg)}
}

type openAIHTTP200JSONErrorBody struct {
	io.ReadCloser
	limit  int64
	reader io.Reader
}

func (b *openAIHTTP200JSONErrorBody) Read(p []byte) (int, error) {
	if b.reader == nil {
		prefix, err := io.ReadAll(io.LimitReader(b.ReadCloser, b.limit+1))
		switch {
		case err != nil:
			b.reader = io.MultiReader(bytes.NewReader(prefix), &openAIHTTP200PrefixError{err: err})
		case int64(len(prefix)) <= b.limit && IsOpenAIGenericUpstreamFailureBody(prefix) && !openAIJSONFailureHasSemanticOutput(prefix):
			var compact bytes.Buffer
			if compactErr := json.Compact(&compact, prefix); compactErr == nil {
				b.reader = io.MultiReader(strings.NewReader("data: "), bytes.NewReader(compact.Bytes()), strings.NewReader("\n\n"))
			} else {
				b.reader = bytes.NewReader(prefix)
			}
		default:
			b.reader = io.MultiReader(bytes.NewReader(prefix), b.ReadCloser)
		}
	}
	return b.reader.Read(p)
}

type openAIHTTP200PrefixError struct{ err error }

func (r *openAIHTTP200PrefixError) Read([]byte) (int, error) { return 0, r.err }
