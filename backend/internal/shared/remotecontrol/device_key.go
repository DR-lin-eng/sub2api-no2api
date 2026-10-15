package remotecontrol

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const (
	DeviceKeyAlgorithm       = "ecdsa_p256_sha256"
	DeviceKeyPayloadDomain   = "codex-device-key-sign-payload/v1"
	DeviceEnrollmentAudience = "remote_control_client_enrollment"
	DeviceConnectionAudience = "remote_control_client_websocket"
	DeviceControllerScope    = "remote_control_controller_websocket"
)

// DeviceKeyPublic is public enrollment material. Native providers keep the
// private key in TPM, Secure Enclave, or the nonextractable OS key store.
type DeviceKeyPublic struct {
	KeyID                  string `json:"keyId"`
	Algorithm              string `json:"algorithm"`
	ProtectionClass        string `json:"protectionClass"`
	PublicKeySPKIDERBase64 string `json:"publicKeySpkiDerBase64"`
}

func DeviceIdentityHash(key DeviceKeyPublic) (string, error) {
	if _, err := ParseDevicePublicKey(key); err != nil {
		return "", err
	}
	// The desktop hashes JSON in this exact field order; map sorting changes it.
	data, err := encodeDeviceJSON(struct {
		Algorithm              string `json:"algorithm"`
		KeyID                  string `json:"keyId"`
		ProtectionClass        string `json:"protectionClass"`
		PublicKeySPKIDERBase64 string `json:"publicKeySpkiDerBase64"`
	}{key.Algorithm, key.KeyID, key.ProtectionClass, key.PublicKeySPKIDERBase64})
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(digest[:]), nil
}

func ParseDevicePublicKey(key DeviceKeyPublic) (*ecdsa.PublicKey, error) {
	if key.Algorithm != DeviceKeyAlgorithm || key.KeyID == "" {
		return nil, errors.New("invalid device key identity")
	}
	prefix := map[string]string{"hardware_tpm": "dk_htp_linux_v1_", "hardware_secure_enclave": "dk_hse_", "os_protected_nonextractable": "dk_osn_"}[key.ProtectionClass]
	if prefix == "" || !strings.HasPrefix(key.KeyID, prefix) || len(key.KeyID) <= len(prefix) || len(key.KeyID) > 256 || strings.ContainsAny(key.KeyID, "/\\\r\n\x00") {
		return nil, errors.New("invalid device key protection class")
	}
	der, err := base64.StdEncoding.Strict().DecodeString(key.PublicKeySPKIDERBase64)
	if err != nil || len(der) > 1024 {
		return nil, errors.New("invalid device public key encoding")
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, errors.New("invalid device public key")
	}
	public, ok := parsed.(*ecdsa.PublicKey)
	if !ok || public.Curve != elliptic.P256() {
		return nil, errors.New("device key must be ECDSA P-256")
	}
	return public, nil
}

type DeviceSignature struct {
	Algorithm          string `json:"algorithm"`
	SignatureDERBase64 string `json:"signatureDerBase64"`
}

type DeviceSigner interface {
	SignDeviceKey(context.Context, string, []byte) (DeviceSignature, error)
}

type EnrollmentDeviceChallenge struct {
	Purpose            string          `json:"purpose"`
	Audience           string          `json:"audience"`
	AccountUserID      string          `json:"account_user_id"`
	ClientID           string          `json:"client_id"`
	ChallengeID        string          `json:"challenge_id"`
	ChallengeToken     string          `json:"challenge_token"`
	Nonce              string          `json:"nonce"`
	TargetOrigin       string          `json:"target_origin"`
	TargetPath         string          `json:"target_path"`
	ChallengeExpiresAt json.RawMessage `json:"challenge_expires_at"`
	DeviceIdentityHash string          `json:"device_identity_hash,omitempty"`
}

type EnrollmentDeviceProof struct {
	ChallengeToken      string `json:"challenge_token"`
	KeyID               string `json:"key_id"`
	SignatureDERBase64  string `json:"signature_der_base64"`
	SignedPayloadBase64 string `json:"signed_payload_base64"`
	Algorithm           string `json:"algorithm"`
}

type ConnectionDeviceChallenge struct {
	Type                 string   `json:"type"`
	Purpose              string   `json:"purpose"`
	Audience             string   `json:"audience"`
	SessionID            string   `json:"sessionId"`
	TargetOrigin         string   `json:"targetOrigin"`
	TargetPath           string   `json:"targetPath"`
	AccountUserID        string   `json:"accountUserId"`
	ClientID             string   `json:"clientId"`
	Nonce                string   `json:"nonce"`
	TokenSHA256Base64URL string   `json:"tokenSha256Base64url"`
	TokenExpiresAt       int64    `json:"tokenExpiresAt"`
	Scopes               []string `json:"scopes"`
}

type ConnectionDeviceProof struct {
	Type                string `json:"type"`
	KeyID               string `json:"keyId"`
	SignatureDERBase64  string `json:"signatureDerBase64"`
	SignedPayloadBase64 string `json:"signedPayloadBase64"`
	Algorithm           string `json:"algorithm"`
}

func SignEnrollmentChallenge(ctx context.Context, signer DeviceSigner, key DeviceKeyPublic, challenge EnrollmentDeviceChallenge, accountUserID, clientID, origin, path string, requireIdentityHash bool, now time.Time) (EnrollmentDeviceProof, error) {
	invalid := errors.New("remote control enrollment challenge does not match local identity")
	if signer == nil || accountUserID == "" || clientID == "" || challenge.Purpose != DeviceEnrollmentAudience || challenge.Audience != DeviceEnrollmentAudience || challenge.AccountUserID != accountUserID || challenge.ClientID != clientID || challenge.TargetOrigin != origin || challenge.TargetPath != path || challenge.ChallengeID == "" || challenge.ChallengeToken == "" || !validDeviceNonce(challenge.Nonce) {
		return EnrollmentDeviceProof{}, invalid
	}
	if !validChallengeExpiry(challenge.ChallengeExpiresAt, now) {
		return EnrollmentDeviceProof{}, errors.New("remote control enrollment challenge expired or malformed")
	}
	hash, err := DeviceIdentityHash(key)
	if err != nil {
		return EnrollmentDeviceProof{}, err
	}
	if (requireIdentityHash && challenge.DeviceIdentityHash == "") || (challenge.DeviceIdentityHash != "" && challenge.DeviceIdentityHash != hash) {
		return EnrollmentDeviceProof{}, invalid
	}
	// Field ordering and the domain wrapper are taken from CKe/wKe in the desktop bundle.
	payload, err := encodeDeviceJSON(struct {
		Domain  string `json:"domain"`
		Payload any    `json:"payload"`
	}{DeviceKeyPayloadDomain, struct {
		AccountUserID                 string          `json:"accountUserId"`
		Audience                      string          `json:"audience"`
		ChallengeExpiresAt            json.RawMessage `json:"challengeExpiresAt"`
		ChallengeID                   string          `json:"challengeId"`
		ClientID                      string          `json:"clientId"`
		DeviceIdentitySHA256Base64URL string          `json:"deviceIdentitySha256Base64url"`
		Nonce                         string          `json:"nonce"`
		TargetOrigin                  string          `json:"targetOrigin"`
		TargetPath                    string          `json:"targetPath"`
		Type                          string          `json:"type"`
	}{accountUserID, DeviceEnrollmentAudience, challenge.ChallengeExpiresAt, challenge.ChallengeID, clientID, hash, challenge.Nonce, origin, path, "remoteControlClientEnrollment"}})
	if err != nil {
		return EnrollmentDeviceProof{}, err
	}
	signature, err := signAndCheck(ctx, signer, key, payload)
	if err != nil {
		return EnrollmentDeviceProof{}, err
	}
	return EnrollmentDeviceProof{challenge.ChallengeToken, key.KeyID, signature.SignatureDERBase64, base64.StdEncoding.EncodeToString(payload), signature.Algorithm}, nil
}

func SignConnectionChallenge(ctx context.Context, signer DeviceSigner, key DeviceKeyPublic, challenge ConnectionDeviceChallenge, accountUserID, clientID, origin, path, sessionToken string, tokenExpiresAt int64, now time.Time) (ConnectionDeviceProof, error) {
	invalid := errors.New("remote control websocket challenge does not match local session")
	digest := sha256.Sum256([]byte(sessionToken))
	if signer == nil || sessionToken == "" || tokenExpiresAt <= now.Unix() || challenge.Type != "device_key_challenge" || challenge.Purpose != DeviceConnectionAudience || challenge.Audience != DeviceConnectionAudience || challenge.AccountUserID != accountUserID || challenge.ClientID != clientID || accountUserID == "" || clientID == "" || challenge.SessionID == "" || challenge.TargetOrigin != origin || challenge.TargetPath != path || challenge.TokenExpiresAt != tokenExpiresAt || challenge.TokenSHA256Base64URL != base64.RawURLEncoding.EncodeToString(digest[:]) || !validDeviceNonce(challenge.Nonce) || len(challenge.Scopes) != 1 || challenge.Scopes[0] != DeviceControllerScope {
		return ConnectionDeviceProof{}, invalid
	}
	payload, err := encodeDeviceJSON(struct {
		Domain  string `json:"domain"`
		Payload any    `json:"payload"`
	}{DeviceKeyPayloadDomain, struct {
		AccountUserID        string   `json:"accountUserId"`
		Audience             string   `json:"audience"`
		ClientID             string   `json:"clientId"`
		Nonce                string   `json:"nonce"`
		Scopes               []string `json:"scopes"`
		SessionID            string   `json:"sessionId"`
		TargetOrigin         string   `json:"targetOrigin"`
		TargetPath           string   `json:"targetPath"`
		TokenExpiresAt       int64    `json:"tokenExpiresAt"`
		TokenSHA256Base64URL string   `json:"tokenSha256Base64url"`
		Type                 string   `json:"type"`
	}{accountUserID, DeviceConnectionAudience, clientID, challenge.Nonce, challenge.Scopes, challenge.SessionID, origin, path, tokenExpiresAt, challenge.TokenSHA256Base64URL, "remoteControlClientConnection"}})
	if err != nil {
		return ConnectionDeviceProof{}, err
	}
	signature, err := signAndCheck(ctx, signer, key, payload)
	if err != nil {
		return ConnectionDeviceProof{}, err
	}
	return ConnectionDeviceProof{"device_key_proof", key.KeyID, signature.SignatureDERBase64, base64.StdEncoding.EncodeToString(payload), signature.Algorithm}, nil
}

func signAndCheck(ctx context.Context, signer DeviceSigner, key DeviceKeyPublic, payload []byte) (DeviceSignature, error) {
	if _, err := ParseDevicePublicKey(key); err != nil {
		return DeviceSignature{}, err
	}
	if err := ctx.Err(); err != nil {
		return DeviceSignature{}, err
	}
	signature, err := signer.SignDeviceKey(ctx, key.KeyID, payload)
	if err != nil {
		return DeviceSignature{}, err
	}
	if err := ctx.Err(); err != nil {
		return DeviceSignature{}, err
	}
	if signature.Algorithm != DeviceKeyAlgorithm {
		return DeviceSignature{}, errors.New("native device signer returned unexpected algorithm")
	}
	if err := VerifyDeviceSignature(key, payload, signature.SignatureDERBase64); err != nil {
		return DeviceSignature{}, err
	}
	return signature, nil
}

func validDeviceNonce(nonce string) bool {
	if len(nonce) > 4096 {
		return false
	}
	decoded, err := base64.RawURLEncoding.Strict().DecodeString(nonce)
	return err == nil && len(decoded) >= 32 && base64.RawURLEncoding.EncodeToString(decoded) == nonce
}

func validChallengeExpiry(raw json.RawMessage, now time.Time) bool {
	var numeric int64
	if json.Unmarshal(raw, &numeric) == nil {
		return numeric > now.Unix() && numeric <= 9007199254740991
	}
	var text string
	if json.Unmarshal(raw, &text) != nil {
		return false
	}
	value, err := time.Parse(time.RFC3339Nano, text)
	return err == nil && value.After(now)
}

func VerifyDeviceSignature(key DeviceKeyPublic, payload []byte, signatureDERBase64 string) error {
	public, err := ParseDevicePublicKey(key)
	if err != nil {
		return err
	}
	signature, err := base64.StdEncoding.Strict().DecodeString(signatureDERBase64)
	if err != nil || len(signature) < 8 || len(signature) > 72 {
		return errors.New("invalid device signature encoding")
	}
	digest := sha256.Sum256(payload)
	if !ecdsa.VerifyASN1(public, digest[:], signature) {
		return errors.New("device signature does not match payload")
	}
	return nil
}

func encodeDeviceJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}
