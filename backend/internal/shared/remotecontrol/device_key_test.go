package remotecontrol

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type goldenSigner struct{ private *ecdsa.PrivateKey }

func (s goldenSigner) SignDeviceKey(_ context.Context, _ string, payload []byte) (DeviceSignature, error) {
	digest := sha256.Sum256(payload)
	signature, err := ecdsa.SignASN1(rand.Reader, s.private, digest[:])
	return DeviceSignature{Algorithm: DeviceKeyAlgorithm, SignatureDERBase64: base64.StdEncoding.EncodeToString(signature)}, err
}

func TestDeviceKeyPayloadMatchesActualDesktopJavaScript(t *testing.T) {
	data, err := os.ReadFile("testdata/desktop_device_key_golden.json")
	require.NoError(t, err)
	var fixture struct {
		Key          DeviceKeyPublic `json:"key"`
		IdentityHash string          `json:"identityHash"`
		Enrollment   struct {
			Nonce string `json:"nonce"`
		} `json:"enrollment"`
		Connection struct {
			Nonce     string `json:"nonce"`
			TokenHash string `json:"tokenSha256Base64url"`
		} `json:"connection"`
		EnrollmentPayload string `json:"enrollmentPayload"`
		ConnectionPayload string `json:"connectionPayload"`
	}
	require.NoError(t, json.Unmarshal(data, &fixture))
	hash, err := DeviceIdentityHash(fixture.Key)
	require.NoError(t, err)
	require.Equal(t, fixture.IdentityHash, hash)
	// The public fixture is P-256's generator point; private scalar 1 is used
	// only to sign this deterministic test identity, never by production code.
	x, y := elliptic.P256().ScalarBaseMult([]byte{1})
	signer := goldenSigner{private: &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, D: big.NewInt(1)}}
	now := time.Unix(1800000000, 0)
	origin := "https://chatgpt.com"
	path := "/backend-api/codex/remote/control/client"
	challenge := EnrollmentDeviceChallenge{Purpose: DeviceEnrollmentAudience, Audience: DeviceEnrollmentAudience, AccountUserID: "account-user-fixture", ClientID: "client-fixture", ChallengeID: "challenge-fixture", ChallengeToken: "challenge-token-fixture", Nonce: fixture.Enrollment.Nonce, TargetOrigin: origin, TargetPath: path + "/enroll/finish", ChallengeExpiresAt: json.RawMessage("4102444800")}
	proof, err := SignEnrollmentChallenge(context.Background(), signer, fixture.Key, challenge, "account-user-fixture", "client-fixture", origin, path+"/enroll/finish", false, now)
	require.NoError(t, err)
	payload, err := base64.StdEncoding.DecodeString(proof.SignedPayloadBase64)
	require.NoError(t, err)
	require.Equal(t, fixture.EnrollmentPayload, string(payload))
	connection := ConnectionDeviceChallenge{Type: "device_key_challenge", Purpose: DeviceConnectionAudience, Audience: DeviceConnectionAudience, AccountUserID: "account-user-fixture", ClientID: "client-fixture", SessionID: "session-fixture", Nonce: fixture.Connection.Nonce, TargetOrigin: origin, TargetPath: path, TokenSHA256Base64URL: fixture.Connection.TokenHash, TokenExpiresAt: 4102444800, Scopes: []string{DeviceControllerScope}}
	websocketProof, err := SignConnectionChallenge(context.Background(), signer, fixture.Key, connection, "account-user-fixture", "client-fixture", origin, path, "session-token-fixture", 4102444800, now)
	require.NoError(t, err)
	payload, err = base64.StdEncoding.DecodeString(websocketProof.SignedPayloadBase64)
	require.NoError(t, err)
	require.Equal(t, fixture.ConnectionPayload, string(payload))
	t.Log("DEVICE_IDENTITY_HASH_JS_MATCH=true ENROLLMENT_BYTES_JS_MATCH=true CONNECTION_BYTES_JS_MATCH=true")
}

func TestDeviceSignatureRejectsPayloadSubstitutionAndRawFormat(t *testing.T) {
	data, err := os.ReadFile("testdata/desktop_device_key_golden.json")
	require.NoError(t, err)
	var fixture struct {
		Key DeviceKeyPublic `json:"key"`
	}
	require.NoError(t, json.Unmarshal(data, &fixture))
	x, y := elliptic.P256().ScalarBaseMult([]byte{1})
	signer := goldenSigner{private: &ecdsa.PrivateKey{PublicKey: ecdsa.PublicKey{Curve: elliptic.P256(), X: x, Y: y}, D: big.NewInt(1)}}
	signature, err := signer.SignDeviceKey(context.Background(), fixture.Key.KeyID, []byte("original"))
	require.NoError(t, err)
	require.NoError(t, VerifyDeviceSignature(fixture.Key, []byte("original"), signature.SignatureDERBase64))
	require.Error(t, VerifyDeviceSignature(fixture.Key, []byte("changed"), signature.SignatureDERBase64))
	require.Error(t, VerifyDeviceSignature(fixture.Key, []byte("original"), base64.StdEncoding.EncodeToString(make([]byte, 64))))
	fixture.Key.Algorithm = "rsa_sha256"
	_, err = ParseDevicePublicKey(fixture.Key)
	require.Error(t, err)
}
