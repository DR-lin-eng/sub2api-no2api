package remotecontrol

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

func TestNormalizeTargetMatchesCodexRemoteControlPaths(t *testing.T) {
	target, err := NormalizeTarget("https://chatgpt.com/backend-api")
	require.NoError(t, err)
	require.Equal(t, "wss://chatgpt.com/backend-api/wham/remote/control/server", target.WebSocketURL)
	require.Equal(t, "https://chatgpt.com/backend-api/wham/remote/control/server/enroll", target.EnrollURL)
	require.Equal(t, "https://chatgpt.com/backend-api/wham/remote/control/server/refresh", target.RefreshURL)
	require.Equal(t, "https://chatgpt.com/backend-api/wham/remote/control/server/pair", target.PairURL)
	require.Equal(t, "https://chatgpt.com/backend-api/wham/remote/control/server/pair/status", target.PairStatusURL)

	local, err := NormalizeTarget("http://localhost:8080/backend-api")
	require.NoError(t, err)
	require.Equal(t, "ws://localhost:8080/backend-api/wham/remote/control/server", local.WebSocketURL)
}

func TestNormalizeTargetRejectsUnsupportedHosts(t *testing.T) {
	for _, raw := range []string{
		"http://chatgpt.com/backend-api",
		"https://example.com/backend-api",
		"https://chatgpt.com.evil.example/backend-api",
		"https://chat.openai.com/backend-api",
	} {
		_, err := NormalizeTarget(raw)
		require.Error(t, err, raw)
	}
}

func TestBuildWebSocketHeadersMatchesProtocolV3Projection(t *testing.T) {
	headers, err := BuildWebSocketHeaders(
		"wss://chatgpt.com/backend-api/wham/remote/control/server",
		Enrollment{ServerID: "server-1", EnvironmentID: "env-1", ServerName: "host-name", RemoteControlToken: "token"},
		"installation-1",
		"mac_mini",
		"cursor-7",
	)
	require.NoError(t, err)
	require.Equal(t, "server-1", headers.Get(ServerIDHeader))
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("host-name")), headers.Get(NameHeader))
	require.Equal(t, ProtocolVersion, headers.Get(ProtocolVersionHeader))
	require.Equal(t, "installation-1", headers.Get(InstallationIDHeader))
	require.Equal(t, "Bearer token", headers.Get("Authorization"))
	require.Equal(t, "mac_mini", headers.Get(HostDeviceKindHeader))
	require.Equal(t, "cursor-7", headers.Get(SubscribeCursorHeader))
}

func TestBuildWebSocketHeadersValidatesEnrollmentAndHost(t *testing.T) {
	_, err := BuildWebSocketHeaders("wss://example.com/server", Enrollment{ServerID: "s", ServerName: "h", RemoteControlToken: "t"}, "i", "", "")
	require.Error(t, err)
	_, err = BuildWebSocketHeaders("wss://chatgpt.com/server", Enrollment{ServerID: "s", ServerName: "h"}, "i", "", "")
	require.Error(t, err)
	_, err = BuildWebSocketHeaders("wss://chatgpt.com/server", Enrollment{ServerID: "s", ServerName: "h", RemoteControlToken: "t"}, "", "", "")
	require.Error(t, err)
}

func TestRemoteControlEnvelopeRoundTrip(t *testing.T) {
	message := []byte(`{"jsonrpc":"2.0","method":"turn/start"}`)
	envelope := ClientEnvelope{Type: "client_message", ClientID: "client-1", StreamID: "stream-1", SeqID: 4, Message: message}
	encoded, err := json.Marshal(envelope)
	require.NoError(t, err)
	var decoded ClientEnvelope
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, envelope.Type, decoded.Type)
	require.Equal(t, envelope.ClientID, decoded.ClientID)
	require.JSONEq(t, string(message), string(decoded.Message))
}

func TestRemoteControlClientEnrollAndPairing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer account-token", r.Header.Get("Authorization"))
		require.Equal(t, "application/json", r.Header.Get("Content-Type"))
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/backend-api/wham/remote/control/server/enroll":
			_, _ = w.Write([]byte(`{"server_id":"server-1","environment_id":"env-1","remote_control_token":"server-token","expires_at":"2030-01-01T00:00:00Z"}`))
		case "/backend-api/wham/remote/control/server/pair":
			_, _ = w.Write([]byte(`{"pairing_code":"pair-1","server_id":"server-1","environment_id":"env-1","expires_at":"2030-01-01T00:00:00Z"}`))
		case "/backend-api/wham/remote/control/server/pair/status":
			_, _ = w.Write([]byte(`{"claimed":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	target, err := NormalizeTarget(server.URL + "/backend-api")
	require.NoError(t, err)
	client := NewClient(server.Client())
	enrolled, err := client.Enroll(context.Background(), target, "account-token", EnrollRequest{Name: "host", OS: "darwin", Arch: "arm64", AppServerVersion: "0.149.0", InstallationID: "install"})
	require.NoError(t, err)
	require.Equal(t, "server-1", enrolled.ServerID)
	pairing, err := client.StartPairing(context.Background(), target, "account-token", PairRequest{ManualCode: true})
	require.NoError(t, err)
	require.Equal(t, "pair-1", pairing.PairingCode)
	status, err := client.PairingStatus(context.Background(), target, "account-token", PairStatusRequest{PairingCode: pairing.PairingCode})
	require.NoError(t, err)
	require.True(t, status.Claimed)
}

func TestDialWebSocketSendsProtocolV3Headers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "server-1", r.Header.Get(ServerIDHeader))
		require.Equal(t, ProtocolVersion, r.Header.Get(ProtocolVersionHeader))
		require.Equal(t, "install", r.Header.Get(InstallationIDHeader))
		require.Equal(t, "Bearer server-token", r.Header.Get("Authorization"))
		conn, err := websocket.Accept(w, r, nil)
		require.NoError(t, err)
		defer func() { _ = conn.Close(websocket.StatusNormalClosure, "") }()
		_, _, err = conn.Read(context.Background())
		if err != nil {
			return
		}
	}))
	defer server.Close()
	base := strings.Replace(server.URL, "http://", "http://", 1)
	target, err := NormalizeTarget(base)
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := DialWebSocket(ctx, target, Enrollment{ServerID: "server-1", ServerName: "host", RemoteControlToken: "server-token"}, "install", "", "", server.Client())
	require.NoError(t, err)
	require.NoError(t, conn.Close(websocket.StatusNormalClosure, "done"))
}

func TestLifecycleManagerEnrollRefreshAndClear(t *testing.T) {
	var enrollCalls, refreshCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/backend-api/wham/remote/control/server/enroll":
			enrollCalls++
			_, _ = w.Write([]byte(`{"server_id":"server-1","environment_id":"env-1","remote_control_token":"token-1","expires_at":"2030-01-01T00:00:00Z"}`))
		case "/backend-api/wham/remote/control/server/refresh":
			refreshCalls++
			_, _ = w.Write([]byte(`{"server_id":"server-1","environment_id":"env-1","remote_control_token":"token-2","expires_at":"2030-01-02T00:00:00Z"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	target, err := NormalizeTarget(server.URL + "/backend-api")
	require.NoError(t, err)
	store := NewMemoryEnrollmentStore()
	manager := NewLifecycleManager(NewClient(server.Client()), store, "account-token")
	record, err := manager.Enroll(context.Background(), target, EnrollRequest{Name: "host", InstallationID: "install"})
	require.NoError(t, err)
	require.Equal(t, "server-1", record.ServerID)
	require.Equal(t, 1, enrollCalls)

	manager.now = func() time.Time { return time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC) }
	refreshed, changed, err := manager.RefreshIfNeeded(context.Background(), target, "install")
	require.NoError(t, err)
	require.True(t, changed)
	require.Equal(t, "token-2", refreshed.RemoteControlToken)
	require.Equal(t, 1, refreshCalls)
	require.NoError(t, manager.Clear(context.Background()))
	current, err := manager.Current(context.Background())
	require.NoError(t, err)
	require.Nil(t, current)
}
