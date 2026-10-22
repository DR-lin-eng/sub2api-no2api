package codexremotecontrol

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/remotecontrol"
	"github.com/coder/websocket"
	"github.com/stretchr/testify/require"
)

// Software keys exist only in this local fixture. Production uses devicekey's
// native provider; these tests assert wire signatures, not hardware provenance.
type fixtureKeys struct {
	private *ecdsa.PrivateKey
	public  remotecontrol.DeviceKeyPublic
	signs   atomic.Int64
	creates atomic.Int64
	deletes atomic.Int64
}

func newFixtureKeys(t *testing.T) *fixtureKeys {
	t.Helper()
	private, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	der, err := x509.MarshalPKIXPublicKey(&private.PublicKey)
	require.NoError(t, err)
	return &fixtureKeys{private: private, public: remotecontrol.DeviceKeyPublic{KeyID: "dk_htp_linux_v1_fixture", Algorithm: remotecontrol.DeviceKeyAlgorithm, ProtectionClass: "hardware_tpm", PublicKeySPKIDERBase64: base64.StdEncoding.EncodeToString(der)}}
}
func (k *fixtureKeys) CreateDeviceKey(context.Context, string) (remotecontrol.DeviceKeyPublic, error) {
	k.creates.Add(1)
	return k.public, nil
}
func (k *fixtureKeys) GetDeviceKeyPublic(context.Context, string) (remotecontrol.DeviceKeyPublic, error) {
	return k.public, nil
}
func (k *fixtureKeys) DeleteDeviceKey(context.Context, string) error { k.deletes.Add(1); return nil }
func (k *fixtureKeys) SignDeviceKey(_ context.Context, _ string, payload []byte) (remotecontrol.DeviceSignature, error) {
	k.signs.Add(1)
	digest := sha256.Sum256(payload)
	signature, err := ecdsa.SignASN1(rand.Reader, k.private, digest[:])
	return remotecontrol.DeviceSignature{Algorithm: remotecontrol.DeviceKeyAlgorithm, SignatureDERBase64: base64.StdEncoding.EncodeToString(signature)}, err
}

func fixtureJWT(t *testing.T, stepUp bool) string {
	t.Helper()
	claims := map[string]any{"https://api.openai.com/auth": map[string]string{"chatgpt_account_user_id": "account-user-fixture", "chatgpt_account_id": "account-fixture", "user_id": "legacy-user-fixture"}}
	if stepUp {
		claims["iat"] = time.Now().Unix()
		claims["pwd_auth_time"] = time.Now().UnixMilli()
		claims["scope"] = "codex.remote_control.enroll"
	}
	data, err := json.Marshal(claims)
	require.NoError(t, err)
	return "fixture." + base64.RawURLEncoding.EncodeToString(data) + ".signature"
}

func fixtureChallenge(origin, path, identityHash string) remotecontrol.EnrollmentDeviceChallenge {
	return remotecontrol.EnrollmentDeviceChallenge{Purpose: remotecontrol.DeviceEnrollmentAudience, Audience: remotecontrol.DeviceEnrollmentAudience, AccountUserID: "account-user-fixture", ClientID: "client-fixture", ChallengeID: "challenge-fixture", ChallengeToken: "challenge-token-fixture", Nonce: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), TargetOrigin: origin, TargetPath: path, ChallengeExpiresAt: json.RawMessage(fmt.Sprint(time.Now().Add(time.Minute).Unix())), DeviceIdentityHash: identityHash}
}

func TestClientEnrollmentRefreshAndWebSocketProof(t *testing.T) {
	keys := newFixtureKeys(t)
	oauth := fixtureJWT(t, false)
	stepUp := fixtureJWT(t, true)
	var accepted atomic.Int64
	var refresh atomic.Int64
	expires := time.Now().Add(time.Hour).Truncate(time.Second)
	const token = "local-session-token-fixture"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+oauth || r.Header.Get("ChatGPT-Account-ID") != "account-fixture" {
			t.Error("wrong account authentication")
			w.WriteHeader(401)
			return
		}
		base := "/backend-api/" + clientPath
		switch r.URL.Path {
		case base + "/enroll/start", base + "/refresh/start":
			hash := ""
			finish := "/enroll/finish"
			if strings.HasSuffix(r.URL.Path, "/refresh/start") {
				refresh.Add(1)
				finish = "/refresh/finish"
				var err error
				hash, err = remotecontrol.DeviceIdentityHash(keys.public)
				if err != nil {
					t.Error(err)
					w.WriteHeader(500)
					return
				}
			}
			_ = json.NewEncoder(w).Encode(challengeResponse{AccountUserID: "account-user-fixture", ClientID: "client-fixture", Challenge: fixtureChallenge(server.URL, base+finish, hash)})
		case base + "/enroll/finish", base + "/refresh/finish":
			var body struct {
				ClientID string                              `json:"client_id"`
				StepUp   string                              `json:"step_up_token"`
				Proof    remotecontrol.EnrollmentDeviceProof `json:"device_key_proof"`
				Identity struct {
					PublicKey string `json:"public_key_spki_der_base64"`
				} `json:"device_identity"`
			}
			if json.NewDecoder(r.Body).Decode(&body) != nil || body.ClientID != "client-fixture" {
				t.Error("bad finish body")
				w.WriteHeader(400)
				return
			}
			if strings.HasSuffix(r.URL.Path, "/enroll/finish") && (body.StepUp != stepUp || body.Identity.PublicKey != keys.public.PublicKeySPKIDERBase64) {
				t.Error("missing explicit enrollment authorization or identity")
				w.WriteHeader(400)
				return
			}
			payload, err := base64.StdEncoding.DecodeString(body.Proof.SignedPayloadBase64)
			if err != nil || body.Proof.KeyID != keys.public.KeyID || body.Proof.Algorithm != remotecontrol.DeviceKeyAlgorithm || body.Proof.ChallengeToken != "challenge-token-fixture" || remotecontrol.VerifyDeviceSignature(keys.public, payload, body.Proof.SignatureDERBase64) != nil {
				t.Error("bad enrollment signature")
				w.WriteHeader(400)
				return
			}
			var signed struct {
				Domain  string `json:"domain"`
				Payload struct {
					TargetPath string `json:"targetPath"`
					Type       string `json:"type"`
				} `json:"payload"`
			}
			if json.Unmarshal(payload, &signed) != nil || signed.Domain != remotecontrol.DeviceKeyPayloadDomain || signed.Payload.TargetPath != r.URL.Path || signed.Payload.Type != "remoteControlClientEnrollment" {
				t.Error("wrong signed enrollment target")
				w.WriteHeader(400)
				return
			}
			accepted.Add(1)
			_ = json.NewEncoder(w).Encode(sessionResponse{AccountUserID: "account-user-fixture", ClientID: "client-fixture", Token: token, ExpiresAt: expires.Format(time.RFC3339), Scopes: []string{remotecontrol.DeviceControllerScope}})
		case base:
			if r.Header.Get(SessionTokenHeader) != "Bearer "+token || r.Header.Get("x-codex-client-id") != "client-fixture" || r.Header.Get(remotecontrol.ProtocolVersionHeader) != "3" || r.Header.Get(remotecontrol.SubscribeCursorHeader) != "cursor-fixture" {
				t.Error("wrong websocket session headers")
				w.WriteHeader(400)
				return
			}
			conn, err := websocket.Accept(w, r, nil)
			if err != nil {
				t.Error(err)
				return
			}
			defer func() { _ = conn.CloseNow() }()
			ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
			defer cancel()
			digest := sha256.Sum256([]byte(token))
			challenge := remotecontrol.ConnectionDeviceChallenge{Type: "device_key_challenge", Purpose: remotecontrol.DeviceConnectionAudience, Audience: remotecontrol.DeviceConnectionAudience, SessionID: "session-fixture", TargetOrigin: server.URL, TargetPath: base, AccountUserID: "account-user-fixture", ClientID: "client-fixture", Nonce: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), TokenSHA256Base64URL: base64.RawURLEncoding.EncodeToString(digest[:]), TokenExpiresAt: expires.Unix(), Scopes: []string{remotecontrol.DeviceControllerScope}}
			data, err := json.Marshal(challenge)
			if err != nil {
				t.Error(err)
				return
			}
			if err := conn.Write(ctx, websocket.MessageText, data); err != nil {
				t.Error(err)
				return
			}
			_, data, err = conn.Read(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			var proof remotecontrol.ConnectionDeviceProof
			if json.Unmarshal(data, &proof) != nil {
				t.Error("bad websocket proof")
				return
			}
			payload, err := base64.StdEncoding.DecodeString(proof.SignedPayloadBase64)
			if err != nil || proof.Type != "device_key_proof" || proof.KeyID != keys.public.KeyID || remotecontrol.VerifyDeviceSignature(keys.public, payload, proof.SignatureDERBase64) != nil {
				t.Error("bad websocket signature")
				return
			}
			accepted.Add(1)
			_ = conn.Write(ctx, websocket.MessageText, []byte("fixture-ready"))
			_, _, _ = conn.Read(ctx)
		default:
			t.Error("unexpected endpoint")
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	manager, err := New(Config{BaseURL: server.URL + "/backend-api", Auth: Auth{Token: oauth, AccountUserID: "account-user-fixture", AccountID: "account-fixture"}, Keys: keys, Store: &MemoryStore{}, RequestStepUp: func(context.Context, string) (string, error) { return stepUp, nil }})
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	session, err := manager.ConnectSession(ctx)
	require.NoError(t, err)
	require.Equal(t, "client-fixture", session.ClientID)
	session, err = manager.ConnectSession(ctx)
	require.NoError(t, err)
	conn, _, err := manager.Dial(ctx, session, "cursor-fixture")
	require.NoError(t, err)
	defer func() { _ = conn.CloseNow() }()
	_, ready, err := conn.Read(ctx)
	require.NoError(t, err)
	require.Equal(t, "fixture-ready", string(ready))
	require.EqualValues(t, 3, accepted.Load())
	require.EqualValues(t, 1, keys.creates.Load())
	require.EqualValues(t, 1, refresh.Load())
	t.Log("ENROLLMENT_SIGNATURE=true REFRESH_IDENTITY_HASH=true WEBSOCKET_SIGNATURE=true PRIVATE_KEY_EXPORTED=false")
}

func TestClientRequiresStepUpBeforeAnyRequest(t *testing.T) {
	keys := newFixtureKeys(t)
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	manager, err := New(Config{BaseURL: server.URL, Auth: Auth{Token: fixtureJWT(t, false), AccountUserID: "account-user-fixture"}, Keys: keys, Store: &MemoryStore{}})
	require.NoError(t, err)
	_, err = manager.ConnectSession(context.Background())
	require.ErrorContains(t, err, "explicit step-up")
	require.Zero(t, calls.Load())
	require.Zero(t, keys.creates.Load())
}

func TestDeviceChallengesRejectBeforeSigning(t *testing.T) {
	for _, kind := range []string{"account", "client", "origin", "path", "nonce", "expired", "refresh_identity", "purpose"} {
		t.Run(kind, func(t *testing.T) {
			keys := newFixtureKeys(t)
			challenge := fixtureChallenge("https://chatgpt.com", "/backend-api/codex/remote/control/client/refresh/finish", "")
			hash, err := remotecontrol.DeviceIdentityHash(keys.public)
			require.NoError(t, err)
			challenge.DeviceIdentityHash = hash
			switch kind {
			case "account":
				challenge.AccountUserID = "different"
			case "client":
				challenge.ClientID = "different"
			case "origin":
				challenge.TargetOrigin = "https://elsewhere.example"
			case "path":
				challenge.TargetPath = "/different"
			case "nonce":
				challenge.Nonce = "short"
			case "expired":
				challenge.ChallengeExpiresAt = json.RawMessage("1")
			case "refresh_identity":
				challenge.DeviceIdentityHash = ""
			case "purpose":
				challenge.Purpose = "other"
			}
			_, err = remotecontrol.SignEnrollmentChallenge(context.Background(), keys, keys.public, challenge, "account-user-fixture", "client-fixture", "https://chatgpt.com", "/backend-api/codex/remote/control/client/refresh/finish", true, time.Now())
			require.Error(t, err)
			require.Zero(t, keys.signs.Load())
		})
	}
	for _, kind := range []string{"token", "expiry", "scopes", "account", "origin", "path", "nonce"} {
		t.Run("connection_"+kind, func(t *testing.T) {
			keys := newFixtureKeys(t)
			digest := sha256.Sum256([]byte("session-token-fixture"))
			expiry := time.Now().Add(time.Minute).Unix()
			challenge := remotecontrol.ConnectionDeviceChallenge{Type: "device_key_challenge", Purpose: remotecontrol.DeviceConnectionAudience, Audience: remotecontrol.DeviceConnectionAudience, SessionID: "session", AccountUserID: "account-user-fixture", ClientID: "client-fixture", Nonce: base64.RawURLEncoding.EncodeToString(make([]byte, 32)), TargetOrigin: "https://chatgpt.com", TargetPath: "/backend-api/codex/remote/control/client", TokenSHA256Base64URL: base64.RawURLEncoding.EncodeToString(digest[:]), TokenExpiresAt: expiry, Scopes: []string{remotecontrol.DeviceControllerScope}}
			switch kind {
			case "token":
				challenge.TokenSHA256Base64URL = "different"
			case "expiry":
				challenge.TokenExpiresAt--
			case "scopes":
				challenge.Scopes = append(challenge.Scopes, "extra")
			case "account":
				challenge.AccountUserID = "different"
			case "origin":
				challenge.TargetOrigin = "https://elsewhere.example"
			case "path":
				challenge.TargetPath = "/other"
			case "nonce":
				challenge.Nonce = "short"
			}
			_, err := remotecontrol.SignConnectionChallenge(context.Background(), keys, keys.public, challenge, "account-user-fixture", "client-fixture", "https://chatgpt.com", "/backend-api/codex/remote/control/client", "session-token-fixture", expiry, time.Now())
			require.Error(t, err)
			require.Zero(t, keys.signs.Load())
		})
	}
}

func TestStepUpFreshnessAccountAndScope(t *testing.T) {
	valid := fixtureJWT(t, true)
	require.NoError(t, ValidateStepUp(valid, "account-user-fixture", time.Now()))
	require.Error(t, ValidateStepUp(valid, "other", time.Now()))
	require.Error(t, ValidateStepUp(valid, "account-user-fixture", time.Now().Add(6*time.Minute)))
	require.Error(t, ValidateStepUp(fixtureJWT(t, false), "account-user-fixture", time.Now()))
}

func TestFileStoreScopesAndStoresOnlyPublicReferences(t *testing.T) {
	directory := t.TempDir()
	first, err := NewFileStore(directory, "https://chatgpt.com/backend-api", "account-user-fixture", "installation-one")
	require.NoError(t, err)
	second, err := NewFileStore(directory, "https://chatgpt.com/backend-api", "account-user-fixture", "installation-two")
	require.NoError(t, err)
	key := newFixtureKeys(t).public
	record := Record{AccountUserID: "account-user-fixture", ClientID: "client-fixture", Key: key}
	require.NoError(t, first.Save(context.Background(), record))
	loaded, err := first.Load(context.Background())
	require.NoError(t, err)
	require.Equal(t, &record, loaded)
	missing, err := second.Load(context.Background())
	require.NoError(t, err)
	require.Nil(t, missing)
	require.NoError(t, first.Clear(context.Background()))
	missing, err = first.Load(context.Background())
	require.NoError(t, err)
	require.Nil(t, missing)
}
