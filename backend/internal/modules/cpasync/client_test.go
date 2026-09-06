package cpasync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestFileEligibility(t *testing.T) {
	now := time.Now()
	future, past := now.Add(time.Minute), now.Add(-time.Minute)
	tests := []struct {
		name   string
		change func(*File)
		reason string
	}{
		{"healthy", func(*File) {}, ""},
		{"disabled", func(f *File) { f.Disabled = true }, "disabled"},
		{"disabled status", func(f *File) { f.Status = "disabled" }, "disabled"},
		{"error", func(f *File) { f.Status = "error" }, "abnormal"},
		{"unknown", func(f *File) { f.Status = "new-state" }, "abnormal"},
		{"missing status", func(f *File) { f.Status = "" }, "abnormal"},
		{"unavailable", func(f *File) { f.Unavailable = true }, "abnormal"},
		{"cooldown", func(f *File) { f.NextRetryAfter = &future }, "abnormal"},
		{"elapsed cooldown", func(f *File) { f.NextRetryAfter = &past }, ""},
		{"runtime only", func(f *File) { f.RuntimeOnly = true }, "runtime_only"},
		{"unsupported", func(f *File) { f.Provider = "qwen" }, "unsupported_provider"},
		{"path", func(f *File) { f.Name = "../token.json" }, "invalid_file_name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := File{Name: "one.json", Provider: "codex", Status: "active"}
			tt.change(&f)
			require.Equal(t, tt.reason, f.SkipReason(now))
		})
	}
}

func TestCPAEndpoint(t *testing.T) {
	for _, value := range []string{"https://CPA.example:443", "https://cpa.example/v1/", "https://cpa.example/v0/management", "https://cpa.example/v0/management/auth-files"} {
		endpoint, err := Endpoint(value)
		require.NoError(t, err)
		require.Equal(t, "https://cpa.example/v0/management/auth-files", endpoint)
	}
	endpoint, err := Endpoint("https://cpa.example/prefix/v1")
	require.NoError(t, err)
	require.Equal(t, "https://cpa.example/prefix/v0/management/auth-files", endpoint)
	for _, value := range []string{"file:///tmp/auth", "http://user:pass@example.com", "https://example.com?key=secret", "https://example.com/#token"} {
		_, err := Endpoint(value)
		require.Error(t, err)
		require.NotContains(t, err.Error(), "secret")
	}
}

func TestCPAClientContractAndRedaction(t *testing.T) {
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Equal(t, http.MethodGet, r.Method)
		require.Equal(t, "Bearer synthetic-password", r.Header.Get("Authorization"))
		if strings.HasSuffix(r.URL.Path, "/download") {
			require.Equal(t, "a&b +.json", r.URL.Query().Get("name"))
			_, _ = w.Write([]byte(`{"type":"codex","access_token":"synthetic-access"}`))
			return
		}
		_, _ = w.Write([]byte(`{"files":[{"name":"a&b +.json","provider":"codex","status":"active","disabled":false,"unavailable":false}]}`))
	}))
	defer server.Close()
	client := NewClient(server.URL+"/v0/management/auth-files", "synthetic-password", server.Client())
	files, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, files, 1)
	data, err := client.Download(context.Background(), files[0].Name)
	require.NoError(t, err)
	require.Equal(t, "synthetic-access", data["access_token"])
	_, err = client.Download(context.Background(), "../private.json")
	require.Error(t, err)
	require.Equal(t, int64(2), calls.Load())
}

func TestCPAClientRejectsInvalidAndOversizedResponses(t *testing.T) {
	for _, body := range []string{`{}`, `{"files":null}`, `{"files":[]}{}`, `{"files":[`, strings.Repeat(" ", maxListBytes+1)} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		_, err := NewClient(server.URL, "password", server.Client()).List(context.Background())
		require.Error(t, err)
		server.Close()
	}
}

func TestCPAClientDoesNotFollowRedirectOrLeakBody(t *testing.T) {
	var followed atomic.Int64
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { followed.Add(1) }))
	defer destination.Close()
	for _, status := range []int{302, 401, 500} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Location", destination.URL)
			w.WriteHeader(status)
			_ = json.NewEncoder(w).Encode(map[string]string{"secret": "password access refresh"})
		}))
		_, err := NewClient(server.URL, "password", server.Client()).List(context.Background())
		require.Error(t, err)
		require.NotContains(t, err.Error(), "password")
		require.NotContains(t, err.Error(), "refresh")
		server.Close()
	}
	require.Zero(t, followed.Load())
}

func TestCPAClientCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := NewClient("https://cpa.invalid", "password", &http.Client{Timeout: time.Second}).List(ctx)
	require.EqualError(t, err, "CPA request failed or timed out")
}
