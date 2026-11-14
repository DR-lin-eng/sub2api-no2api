// Isolated protocol fixture. Its rejection modes model an incompatible upstream;
// they do not establish the behavior of the real ChatGPT service.
package main

import (
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"crypto/sha256"
	"strconv"

	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/andybalholm/brotli"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"
)

type capture struct {
	Encoding             string `json:"encoding"`
	WireHash             string `json:"wire_hash"`
	ContentType          string `json:"content_type"`
	AppServerCompression string `json:"app_server_compression"`
	AcceptEncoding       string `json:"accept_encoding"`
	WireBytes            int    `json:"wire_bytes"`
	DecodedBytes         int    `json:"decoded_bytes"`
	BodyHash             string `json:"body_hash"`
	InputHash            string `json:"input_hash"`
	ValidJSON            bool   `json:"valid_json"`
	Stream               bool   `json:"stream"`
	Model                string `json:"model"`
	Path                 string `json:"path"`
	Status               int    `json:"status"`
	ContentLength        int64  `json:"content_length"`
	IdentityHash         string `json:"identity_hash"`
}

var state = struct {
	sync.Mutex
	Mode     string    `json:"mode"`
	Captures []capture `json:"captures"`
	Wires    [][]byte  `json:"-"`
}{Mode: "accept", Captures: []capture{}}

func send(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func hash(value []byte) string {
	sum := sha256.Sum256(value)
	return hex.EncodeToString(sum[:])
}

func serve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/fixture/wire" {
		state.Lock()
		defer state.Unlock()
		index, err := strconv.Atoi(r.URL.Query().Get("index"))
		if err != nil || index < 0 || index >= len(state.Wires) {
			w.WriteHeader(404)
			return
		}
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(state.Wires[index])
		return
	}
	if !strings.HasPrefix(r.URL.Path, "/fixture/") && r.URL.Path != "/backend-api/codex/responses" && r.URL.Path != "/backend-api/codex/responses/compact" {
		w.WriteHeader(404)
		return
	}

	if r.URL.Path == "/fixture/state" {
		state.Lock()
		defer state.Unlock()
		if r.Method == http.MethodPost {
			var control struct{ Mode string }
			if err := json.NewDecoder(r.Body).Decode(&control); err != nil {
				send(w, 400, map[string]string{"error": err.Error()})
				return
			}
			state.Mode, state.Captures, state.Wires = control.Mode, []capture{}, nil
		}
		send(w, 200, &state)
		return
	}
	wire, err := io.ReadAll(io.LimitReader(r.Body, 64<<20))
	if err != nil {
		send(w, 400, map[string]string{"detail": "fixture read failed"})
		return
	}
	body := wire
	encoding := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Encoding")))
	if encoding == "zstd" {
		decoder, err := zstd.NewReader(nil)
		if err != nil {
			panic(err)
		}
		body, err = decoder.DecodeAll(wire, nil)
		decoder.Close()
		if err != nil {
			send(w, 400, map[string]string{"detail": "fixture zstd decode failed"})
			return
		}
	}
	var request struct {
		Model        string          `json:"model"`
		Stream       bool            `json:"stream"`
		Input        json.RawMessage `json:"input"`
		Instructions string          `json:"instructions"`
	}
	parseErr := json.Unmarshal(body, &request)
	state.Lock()
	mode := state.Mode
	status := 200
	if parseErr != nil || mode == "reject_all" || (mode == "reject_zstd" && encoding == "zstd") {
		status = 400
	}
	if mode == "reject_zstd_415" && encoding == "zstd" {
		status = 415
	}
	if mode == "reject_structured" {
		status = 400
	}
	identity, _ := json.Marshal([]string{r.Header.Get("Authorization"), r.Header.Get("Chatgpt-Account-Id"),
		r.Header.Get("session_id"), r.Header.Get("conversation_id"), r.Header.Get("User-Agent"), r.Header.Get("Accept")})
	state.Captures = append(state.Captures, capture{
		Encoding: encoding, WireHash: hash(wire), ContentType: r.Header.Get("Content-Type"),
		AppServerCompression: r.Header.Get("x-codex-app-server-compression"), AcceptEncoding: r.Header.Get("Accept-Encoding"),
		WireBytes: len(wire), DecodedBytes: len(body),
		BodyHash: hash(body), InputHash: hash(request.Input), ValidJSON: parseErr == nil,
		Model: request.Model, Stream: request.Stream, Path: r.URL.Path, Status: status,
		ContentLength: r.ContentLength, IdentityHash: hash(identity),
	})
	state.Wires = append(state.Wires, wire)
	id := fmt.Sprintf("resp_fixture_%d", len(state.Captures))
	state.Unlock()
	w.Header().Set("X-Request-ID", "issue-8636-"+id)
	if status == 400 {
		if mode == "reject_structured" {
			send(w, 400, map[string]any{"error": map[string]string{"type": "invalid_request_error", "param": "input", "message": "Invalid input parameter"}})
			return
		}
		send(w, 400, map[string]string{"detail": "Bad Request"})
		return
	}
	if status == 415 {
		send(w, 415, map[string]string{"detail": "Unsupported Content-Encoding: zstd"})
		return
	}
	w = compressedResponse(w, mode)
	defer func() {
		if closer, ok := w.(io.Closer); ok {
			_ = closer.Close()
		}
	}()
	response := map[string]any{
		"id": id, "object": "response", "status": "completed", "created_at": 1794441600,
		"model": request.Model, "instructions": request.Instructions,
		"output": []any{map[string]any{"id": "msg_fixture", "type": "message", "role": "assistant",
			"content": []any{map[string]string{"type": "output_text", "text": "fixture ok"}}}},
		"usage": map[string]any{"input_tokens": 100, "output_tokens": 10, "total_tokens": 110,
			"input_tokens_details": map[string]int{"cached_tokens": 0}},
	}
	if !request.Stream || strings.HasSuffix(r.URL.Path, "/compact") {
		send(w, 200, response)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	for _, event := range []any{
		map[string]any{"type": "response.created", "response": map[string]any{
			"id": id, "object": "response", "status": "in_progress", "created_at": 1794441600, "output": []any{}}},
		map[string]any{"type": "response.output_text.delta", "delta": "fixture ok", "output_index": 0, "content_index": 0},
		map[string]any{"type": "response.completed", "response": response},
	} {
		data, _ := json.Marshal(event)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
	}
	_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
}

type encodedResponse struct {
	http.ResponseWriter
	writer io.WriteCloser
}

func (w *encodedResponse) Write(body []byte) (int, error) { return w.writer.Write(body) }
func (w *encodedResponse) Close() error                   { return w.writer.Close() }
func compressedResponse(w http.ResponseWriter, mode string) http.ResponseWriter {
	var writer io.WriteCloser
	encoding := ""
	switch mode {
	case "response_zlib":
		encoding = "deflate"
		writer = zlib.NewWriter(w)
	case "response_raw_deflate":
		encoding = "deflate"
		writer, _ = flate.NewWriter(w, flate.DefaultCompression)
	case "response_gzip":
		encoding = "gzip"
		writer = gzip.NewWriter(w)
	case "response_br":
		encoding = "br"
		writer = brotli.NewWriter(w)
	case "response_zstd":
		encoding = "zstd"
		writer, _ = zstd.NewWriter(w)
	}
	if encoding == "" {
		return w
	}
	w.Header().Set("Content-Encoding", encoding)
	return &encodedResponse{ResponseWriter: w, writer: writer}
}

func main() {
	log.Fatal(http.ListenAndServe(":8000", http.HandlerFunc(serve)))
}
