package httputil

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/tidwall/gjson"
)

func TestNormalizeLenientJSONRequestBody_accepts_client_control_chars_in_strings(t *testing.T) {
	tests := []struct {
		name    string
		body    []byte
		path    string
		want    string
		wantRaw string
	}{
		{
			name:    "null byte in message content",
			body:    []byte("{\"messages\":[{\"content\":\"hello\x00world\"}]}"),
			path:    "messages.0.content",
			want:    "hello\x00world",
			wantRaw: `"hello\u0000world"`,
		},
		{
			name:    "ansi escape in message content",
			body:    []byte("{\"messages\":[{\"content\":\"hello\x1b[31mred\x1b[0m\"}]}"),
			path:    "messages.0.content",
			want:    "hello\x1b[31mred\x1b[0m",
			wantRaw: `"hello\u001b[31mred\u001b[0m"`,
		},
		{
			name:    "leading UTF-8 BOM",
			body:    []byte("\xef\xbb\xbf{\"input\":\"hello\"}"),
			path:    "input",
			want:    "hello",
			wantRaw: `"hello"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Given
			if gjson.ValidBytes(tt.body) {
				t.Fatalf("test payload should reproduce strict JSON rejection: %q", tt.body)
			}

			// When
			got, err := NormalizeLenientJSONRequestBody(tt.body, 1024)
			if err != nil {
				t.Fatalf("NormalizeLenientJSONRequestBody: %v", err)
			}

			// Then
			if !gjson.ValidBytes(got) {
				t.Fatalf("normalized body should be valid JSON: %q", got)
			}
			result := gjson.GetBytes(got, tt.path)
			if result.String() != tt.want {
				t.Fatalf("value mismatch: got %q want %q", result.String(), tt.want)
			}
			if result.Raw != tt.wantRaw {
				t.Fatalf("raw value mismatch: got %q want %q", result.Raw, tt.wantRaw)
			}
		})
	}
}

func TestNormalizeLenientJSONRequestBody_keeps_invalid_structure_invalid(t *testing.T) {
	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "truncated JSON",
			body: []byte("{\"messages\":[{\"content\":\"hello\"}]"),
		},
		{
			name: "control character outside string",
			body: []byte("{\"input\":\"hello\"}\x00"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// When
			got, err := NormalizeLenientJSONRequestBody(tt.body, 1024)
			if err != nil {
				t.Fatalf("NormalizeLenientJSONRequestBody: %v", err)
			}

			// Then
			if gjson.ValidBytes(got) {
				t.Fatalf("normalization must not repair invalid JSON structure: %q", got)
			}
		})
	}
}

func TestNormalizeLenientJSONRequestBody_allows_http_requests_with_client_control_chars(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Given
		body, err := ReadLenientJSONRequestBodyWithPrealloc(r, 1024)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// When
		if !gjson.ValidBytes(body) {
			http.Error(w, "Failed to parse request body", http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	tests := []struct {
		name string
		body []byte
		want int
	}{
		{
			name: "null byte in JSON string",
			body: []byte("{\"model\":\"gpt-5.5\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\x00world\"}]}"),
			want: http.StatusAccepted,
		},
		{
			name: "ANSI escape in JSON string",
			body: []byte("{\"model\":\"gpt-5.5\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\x1b[31mred\x1b[0m\"}]}"),
			want: http.StatusAccepted,
		},
		{
			name: "leading UTF-8 BOM",
			body: []byte("\xef\xbb\xbf{\"model\":\"gpt-5.5\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]}"),
			want: http.StatusAccepted,
		},
		{
			name: "truncated JSON",
			body: []byte("{\"model\":\"gpt-5.5\",\"messages\":[{\"role\":\"user\",\"content\":\"hello\"}]"),
			want: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", bytes.NewReader(tt.body))
			if err != nil {
				t.Fatalf("NewRequest: %v", err)
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := server.Client().Do(req)
			if err != nil {
				t.Fatalf("Do: %v", err)
			}
			defer func() { _ = resp.Body.Close() }()

			if resp.StatusCode != tt.want {
				t.Fatalf("status mismatch: got %d want %d", resp.StatusCode, tt.want)
			}
		})
	}
}

func TestNormalizeLenientJSONRequestBody_rejects_expansion_past_limit(t *testing.T) {
	// Given
	body := []byte("{\"input\":\"\x00\x00\"}")

	// When
	_, err := NormalizeLenientJSONRequestBody(body, int64(len(body)+5))

	// Then
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) {
		t.Fatalf("expected MaxBytesError, got %T %v", err, err)
	}
	if maxErr.Limit != int64(len(body)+5) {
		t.Fatalf("limit mismatch: got %d want %d", maxErr.Limit, len(body)+5)
	}
}

// referenceNormalizeLenientJSON is the plain per-byte scan the bulk-skipping
// implementation must stay identical to.
func referenceNormalizeLenientJSON(body []byte, maxNormalizedBytes int64) ([]byte, bool) {
	body = trimUTF8BOM(body)
	if int64(len(body)) > maxNormalizedBytes {
		return nil, false
	}
	out := make([]byte, 0, len(body))
	inString, escaped := false, false
	for _, b := range body {
		if inString && isJSONControlByte(b) {
			out = appendJSONUnicodeEscape(out, b)
			escaped = false
		} else {
			switch {
			case escaped:
				escaped = false
			case inString && b == '\\':
				escaped = true
			case b == '"':
				inString = !inString
			}
			out = append(out, b)
		}
		if int64(len(out)) > maxNormalizedBytes {
			return nil, false
		}
	}
	return out, true
}

func TestNormalizeLenientJSONRequestBody_matches_per_byte_reference(t *testing.T) {
	alphabet := []byte("\"\"\"\\\\ab{}[]:, \n\t\x00\x1f\x7f\x80\xff")
	state := uint64(0x9e3779b97f4a7c15)
	next := func(n int) int {
		state ^= state << 13
		state ^= state >> 7
		state ^= state << 17
		return int(state % uint64(n))
	}
	for round := 0; round < 4000; round++ {
		body := make([]byte, next(96))
		for i := range body {
			if next(4) == 0 {
				body[i] = alphabet[next(len(alphabet))]
			} else {
				body[i] = byte('a' + next(26))
			}
		}
		for _, limit := range []int64{int64(len(body)), int64(len(body)) + 5, int64(len(body)) + 64, 1 << 20} {
			if limit == 0 {
				continue
			}
			want, wantOK := referenceNormalizeLenientJSON(body, limit)
			got, err := NormalizeLenientJSONRequestBody(body, limit)
			if !wantOK {
				var maxErr *http.MaxBytesError
				if !errors.As(err, &maxErr) {
					t.Fatalf("body=%q limit=%d: err=%v, want MaxBytesError", body, limit, err)
				}
				continue
			}
			if err != nil || !bytes.Equal(got, want) {
				t.Fatalf("body=%q limit=%d: got=%q err=%v, want=%q", body, limit, got, err, want)
			}
		}
	}
}

func BenchmarkNormalizeLenientJSONRequestBody(b *testing.B) {
	text := bytes.Repeat([]byte("The quick brown fox jumps over the lazy dog. "), 512<<10/45)
	body := append(append([]byte(`{"model":"gpt-5.5","input":"`), text...), `","stream":true}`...)
	b.SetBytes(int64(len(body)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := NormalizeLenientJSONRequestBody(body, 0); err != nil {
			b.Fatal(err)
		}
	}
}
