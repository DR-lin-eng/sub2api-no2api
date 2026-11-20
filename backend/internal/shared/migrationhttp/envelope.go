// Package migrationhttp protects individual migration RPCs to an HTTP-only
// peer with RFC 9180 HPKE. The issuing site's public key travels out of band
// inside the operator's scoped credential, never via unauthenticated discovery.
package migrationhttp

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/hpke"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net/http"
	"strings"
	"time"
)

const (
	Header       = "X-Sub2API-Migration-Transport"
	Version      = "hpke-v1"
	CloneHeader  = "X-Sub2API-Clone-Policy"
	publicSize   = 32
	MaxPayload   = (1 << 20) + 64
	MaxFrame     = MaxPayload + 512
	MaxClockSkew = 2 * time.Minute
	keySeparator = ".m1."
)

var (
	ErrKey      = errors.New("HTTP migration requires a migration key created or rotated on the upgraded peer")
	ErrEnvelope = errors.New("invalid migration envelope or incompatible peer")
)
var kem = hpke.DHKEM(ecdh.X25519())

func PinKey(secret string, root []byte) (string, error) {
	if len(root) != 32 {
		return "", ErrKey
	}
	private, err := ecdh.X25519().NewPrivateKey(root)
	if err != nil {
		return "", ErrKey
	}
	if len(secret) < 16 {
		return "", ErrKey
	}
	// Keep the normal key prefix and a random last-four suffix for the admin key list.
	return secret[:len(secret)-4] + keySeparator + base64.RawURLEncoding.EncodeToString(private.PublicKey().Bytes()) + "." + secret[len(secret)-4:], nil
}
func publicKey(secret string) ([]byte, error) {
	raw, pin, ok := strings.Cut(secret, keySeparator)
	if !ok || len(raw) < 16 || len(secret) > 256 {
		return nil, ErrKey
	}
	pub, last, found := strings.Cut(pin, ".")
	if !found || len(last) != 4 {
		return nil, ErrKey
	}
	b, err := base64.RawURLEncoding.DecodeString(pub)
	if err != nil || len(b) != publicSize {
		return nil, ErrKey
	}
	return b, nil
}
func CheckKey(secret string) error { _, err := publicKey(secret); return err }

type Reply struct {
	aead cipher.AEAD
	aad  []byte
}
type Opened struct {
	Key     string
	Body    []byte
	Nonce   []byte
	Expires time.Time
	Reply   Reply
}

func info(req *http.Request) []byte {
	return []byte("sub2api/migration-http/v1\x00" + req.Method + "\x00" + req.URL.RequestURI() + "\x00" + req.Header.Get(CloneHeader))
}

type exporter interface {
	Export(string, int) ([]byte, error)
}

func reply(c exporter, enc []byte) (Reply, error) {
	key, e := c.Export("sub2api/migration-http/v1/response", 32)
	if e != nil {
		return Reply{}, e
	}
	b, e := aes.NewCipher(key)
	if e != nil {
		return Reply{}, e
	}
	g, e := cipher.NewGCMWithRandomNonce(b)
	return Reply{aead: g, aad: enc}, e
}
func (r Reply) context(status int, clone string) []byte {
	aad := binary.BigEndian.AppendUint16(nil, uint16(status))
	aad = append(aad, r.aad...)
	return append(aad, clone...)
}
func (r Reply) Seal(status int, b []byte, clone string) []byte {
	return r.aead.Seal(nil, nil, b, r.context(status, clone))
}
func (r Reply) Open(status int, b []byte, clone string) ([]byte, error) {
	if len(b) < r.aead.Overhead() || len(b) > MaxFrame {
		return nil, ErrEnvelope
	}
	out, e := r.aead.Open(nil, nil, b, r.context(status, clone))
	if e != nil {
		return nil, ErrEnvelope
	}
	return out, nil
}
func SealRequest(req *http.Request, key string, body []byte, now time.Time) ([]byte, Reply, error) {
	pub, e := publicKey(key)
	if e != nil {
		return nil, Reply{}, e
	}
	if len(body) > MaxPayload {
		return nil, Reply{}, ErrEnvelope
	}
	k, e := kem.NewPublicKey(pub)
	if e != nil {
		return nil, Reply{}, ErrKey
	}
	enc, s, e := hpke.NewSender(k, hpke.HKDFSHA256(), hpke.AES256GCM(), info(req))
	if e != nil {
		return nil, Reply{}, e
	}
	plain := binary.BigEndian.AppendUint64(nil, uint64(now.Unix()))
	plain = binary.BigEndian.AppendUint16(plain, uint16(len(key)))
	plain = append(plain, key...)
	plain = append(plain, body...)
	sealed, e := s.Seal(nil, plain)
	clear(plain)
	if e != nil {
		return nil, Reply{}, e
	}
	response, e := reply(s, enc)
	return append(enc, sealed...), response, e
}
func OpenRequest(req *http.Request, root, frame []byte, now time.Time) (Opened, error) {
	if len(frame) <= publicSize || len(frame) > MaxFrame {
		return Opened{}, ErrEnvelope
	}
	k, e := ecdh.X25519().NewPrivateKey(root)
	if e != nil {
		return Opened{}, ErrKey
	}
	private, e := hpke.NewDHKEMPrivateKey(k)
	if e != nil {
		return Opened{}, ErrKey
	}
	enc := frame[:publicSize]
	recipient, e := hpke.NewRecipient(enc, private, hpke.HKDFSHA256(), hpke.AES256GCM(), info(req))
	if e != nil {
		return Opened{}, ErrEnvelope
	}
	plain, e := recipient.Open(nil, frame[publicSize:])
	if e != nil || len(plain) < 10 {
		return Opened{}, ErrEnvelope
	}
	ts := int64(binary.BigEndian.Uint64(plain))
	issued := time.Unix(ts, 0)
	if issued.Before(now.Add(-MaxClockSkew)) || issued.After(now.Add(MaxClockSkew)) {
		return Opened{}, ErrEnvelope
	}
	n := int(binary.BigEndian.Uint16(plain[8:]))
	if n < 16 || n > 256 || len(plain) < 10+n || len(plain)-(10+n) > MaxPayload {
		return Opened{}, ErrEnvelope
	}
	key := string(plain[10 : 10+n])
	clear(plain[:10+n])
	pub, e := publicKey(key)
	if e != nil || !bytes.Equal(pub, k.PublicKey().Bytes()) {
		return Opened{}, ErrKey
	}
	response, e := reply(recipient, enc)
	if e != nil {
		return Opened{}, ErrEnvelope
	}
	return Opened{Key: key, Body: plain[10+n:], Nonce: enc, Expires: issued.Add(MaxClockSkew + time.Second), Reply: response}, nil
}
