package migrationhttp

import (
	"bytes"
	"crypto/rand"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPinnedHTTPRPC(t *testing.T) {
	root := make([]byte, 32)
	_, err := rand.Read(root)
	require.NoError(t, err)
	key, err := PinKey("admin-standalone-fixture-credential-for-migration", root)
	require.NoError(t, err)
	now := time.Unix(1800000000, 0)
	r := httptest.NewRequest(http.MethodPost, "http://peer.example/api/v1/admin/site-migration/export/probe?id=fake-job&n=0&scope=clone", nil)
	r.Header.Set(CloneHeader, "ordinary-users-v2")
	plain := []byte("synthetic block metadata")
	wire, reply, err := SealRequest(r, key, plain, now)
	require.NoError(t, err)
	require.False(t, bytes.Contains(wire, plain))
	require.False(t, bytes.Contains(wire, []byte(key)))
	opened, err := OpenRequest(r, root, wire, now)
	require.NoError(t, err)
	require.Equal(t, key, opened.Key)
	require.Equal(t, plain, opened.Body)
	cloned := r.Clone(r.Context())
	cloned.URL.RawQuery += "0"
	_, err = OpenRequest(cloned, root, wire, now)
	require.ErrorIs(t, err, ErrEnvelope)
	_, err = OpenRequest(r, root, wire, now.Add(MaxClockSkew+time.Second))
	require.ErrorIs(t, err, ErrEnvelope)
	altered := bytes.Clone(wire)
	altered[len(altered)-1] ^= 1
	_, err = OpenRequest(r, root, altered, now)
	require.ErrorIs(t, err, ErrEnvelope)
	_, err = OpenRequest(r, bytes.Repeat([]byte{9}, 32), wire, now)
	require.Error(t, err)
	body := []byte("synthetic status")
	sealed := opened.Reply.Seal(200, body, "v2")
	require.False(t, bytes.Contains(sealed, body))
	out, err := reply.Open(200, sealed, "v2")
	require.NoError(t, err)
	require.Equal(t, body, out)
	_, err = reply.Open(403, sealed, "v2")
	require.ErrorIs(t, err, ErrEnvelope)
	_, err = reply.Open(200, sealed, "")
	require.ErrorIs(t, err, ErrEnvelope)
	otherWire, otherReply, err := SealRequest(r, key, plain, now)
	require.NoError(t, err)
	require.NotEqual(t, wire, otherWire)
	_, err = otherReply.Open(200, sealed, "v2")
	require.ErrorIs(t, err, ErrEnvelope)
	require.ErrorIs(t, CheckKey("admin-legacy-test-secret"), ErrKey)
	require.ErrorIs(t, CheckKey("https://not-a-key"), ErrKey)
}
