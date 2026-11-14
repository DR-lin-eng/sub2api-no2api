package chatgptimages

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"unicode/utf16"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func encodeIntegrityChallenge(t testing.TB, key string, program any) string {
	t.Helper()
	data, err := json.Marshal(program)
	require.NoError(t, err)
	units := utf16.Encode([]rune(string(data)))
	data = data[:0]
	for n, unit := range units {
		value := unit ^ uint16(key[n%len(key)])
		require.LessOrEqual(t, value, uint16(255))
		data = append(data, byte(value))
	}
	return base64.StdEncoding.EncodeToString(data)
}

func TestIntegrityEnvironmentAndIdentity(t *testing.T) {
	a, err := NewIntegrity("fixture-agent", "zh-CN,zh;q=0.9", "persisted-account-device")
	require.NoError(t, err)
	b, err := NewIntegrity("fixture-agent", "zh-CN", "persisted-account-device")
	require.NoError(t, err)
	var ids []string
	for _, i := range []*Integrity{a, b} {
		data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(i.RequirementsKey(), "gAAAAAC"))
		require.NoError(t, err)
		var env []any
		require.NoError(t, json.Unmarshal(data, &env))
		require.Len(t, env, 25)
		require.Equal(t, float64(1), env[3])
		require.Equal(t, "fixture-agent", env[4])
		require.Nil(t, env[5])
		require.Nil(t, env[6])
		require.Equal(t, "zh-CN", env[7])
		navigatorProperty, ok := env[10].(string)
		require.True(t, ok)
		require.Contains(t, navigatorProperty, "−")
		instanceID, ok := env[14].(string)
		require.True(t, ok)
		id, err := uuid.Parse(instanceID)
		require.NoError(t, err)
		require.Equal(t, uuid.Version(4), id.Version())
		require.NotEqual(t, i.profile.DeviceID, env[14])
		ids = append(ids, id.String())
	}
	require.NotEqual(t, ids[0], ids[1])
	program := [][]any{{2, 40, "localStorage"}, {6, 41, 10, 40}, {2, 42, "getItem"}, {24, 43, 41, 42}, {2, 44, "codex.chatgpt-conversations.device-id"}, {17, 45, 43, 44}, {7, 3, 45}}
	token, err := a.solveVM(t.Context(), encodeIntegrityChallenge(t, a.key, program))
	require.NoError(t, err)
	require.Equal(t, base64.StdEncoding.EncodeToString([]byte("persisted-account-device")), token)
}

func TestIntegrityJSONPreservesLiteralEscapes(t *testing.T) {
	input := "<环境>\u2028\u2029 literal: \\u2028 \\u2029 \\\u2028"
	data, err := integrityJSON(input)
	require.NoError(t, err)
	var decoded string
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, input, decoded)
	require.Contains(t, string(data), "<环境>\u2028\u2029")
	require.Contains(t, string(data), `\\u2028`)
}

func TestIntegrityVMReferenceGolden(t *testing.T) {
	data, err := os.ReadFile("testdata/integrity_vm_golden.json")
	require.NoError(t, err)
	var golden struct {
		Fixtures []struct {
			Name     string
			Program  json.RawMessage
			Token    string
			Rejected bool
		}
		Hashes []struct {
			Input string
			Hex   string
		}
	}
	require.NoError(t, json.Unmarshal(data, &golden))
	for _, fixture := range golden.Fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			i, err := NewIntegrity("fixture-agent", "en", "fixture-device")
			require.NoError(t, err)
			token, err := i.solveVM(t.Context(), encodeIntegrityChallenge(t, i.key, fixture.Program))
			if fixture.Rejected {
				require.ErrorIs(t, err, ErrVMRejected)
				return
			}
			require.NoError(t, err)
			require.Equal(t, fixture.Token, token)
		})
	}
	for _, fixture := range golden.Hashes {
		h := uint32(0x811c9dc5)
		for _, c := range utf16.Encode([]rune(fixture.Input)) {
			h = (h ^ uint32(c)) * 0x1000193
		}
		require.Equal(t, fixture.Hex, fmt.Sprintf("%08x", finishProofHash(h)))
	}
}

func TestIntegrityProofAndHeaderBinding(t *testing.T) {
	i, err := NewIntegrity("fixture-agent", "en", "fixture-device")
	require.NoError(t, err)
	for _, difficulty := range []string{"f", "00", "0fff", "fffffffff"} {
		body, err := json.Marshal(map[string]any{"token": "final-token", "prepare_token": "prepare-token", "proofofwork": map[string]any{"required": true, "seed": "seed🌈漢字", "difficulty": difficulty}, "turnstile": map[string]any{"required": true, "dx": encodeIntegrityChallenge(t, i.key, [][]any{{3, "fixture-vm"}})}})
		require.NoError(t, err)
		headers, err := i.Headers(t.Context(), body)
		require.NoError(t, err)
		require.Equal(t, "final-token", headers.Get("OpenAI-Sentinel-Chat-Requirements-Token"))
		require.Empty(t, headers.Get("OpenAI-Sentinel-Chat-Requirements-Prepare-Token"))
		require.Equal(t, "Zml4dHVyZS12bQ==", headers.Get("OpenAI-Sentinel-Turnstile-Token"))
		proof := headers.Get("OpenAI-Sentinel-Proof-Token")
		require.True(t, strings.HasPrefix(proof, "gAAAAAB"))
		require.True(t, strings.HasSuffix(proof, "~S"))
		candidate := strings.TrimSuffix(strings.TrimPrefix(proof, "gAAAAAB"), "~S")
		data, err := base64.StdEncoding.DecodeString(candidate)
		require.NoError(t, err)
		var env []any
		require.NoError(t, json.Unmarshal(data, &env))
		require.Len(t, env, 25)
		nonce, ok := env[3].(float64)
		require.True(t, ok)
		elapsed, ok := env[9].(float64)
		require.True(t, ok)
		require.GreaterOrEqual(t, nonce, float64(0))
		require.Less(t, nonce, float64(500000))
		require.GreaterOrEqual(t, elapsed, float64(0))
		h := uint32(0x811c9dc5)
		for _, c := range utf16.Encode([]rune("seed🌈漢字" + candidate)) {
			h = (h ^ uint32(c)) * 0x1000193
		}
		require.LessOrEqual(t, fmt.Sprintf("%08x", finishProofHash(h))[:min(len(difficulty), 8)], difficulty)
	}
	prepare, err := i.Headers(t.Context(), []byte(`{"prepare_token":"prepare-only"}`))
	require.NoError(t, err)
	require.Equal(t, "prepare-only", prepare.Get("OpenAI-Sentinel-Chat-Requirements-Prepare-Token"))
	require.Len(t, prepare, 1)
	_, err = i.proof(t.Context(), "fixture", "00000000", 0)
	require.ErrorIs(t, err, ErrProofExhausted)
}

func TestIntegrityRejectionAndBudgets(t *testing.T) {
	i, err := NewIntegrity("fixture-agent", "en", "fixture-device")
	require.NoError(t, err)
	for _, body := range []string{`null`, `[]`, `{`, `{"token":"bad\r\nheader"}`, `{"proofofwork":{"required":true}}`, `{"turnstile":{"required":true}}`} {
		_, err := i.Headers(t.Context(), []byte(body))
		require.Error(t, err, body)
	}
	_, err = i.Headers(t.Context(), []byte(`{"force_login":true,"turnstile":{"required":true}}`))
	require.ErrorIs(t, err, ErrForceLogin)
	for name, program := range map[string]any{
		"constructor":      [][]any{{2, 40, "constructor"}, {6, 41, 10, 40}},
		"largeString":      [][]any{{2, 40, "a"}, {2, 41, "repeat"}, {24, 42, 40, 41}, {2, 43, 1000000000}, {17, 44, 42, 43}},
		"recursiveClosure": [][]any{{30, 40, 41, [][]any{{7, 40}}}, {7, 40}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := i.solveVM(t.Context(), encodeIntegrityChallenge(t, i.key, program))
			require.ErrorIs(t, err, ErrVMBudget)
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = i.Headers(ctx, []byte(`{"proofofwork":{"required":true,"seed":"fixture","difficulty":"00000000"}}`))
	require.ErrorIs(t, err, context.Canceled)
	_, err = i.proof(ctx, "fixture", "00000000", 500000)
	require.ErrorIs(t, err, context.Canceled)
}

func TestIntegrityVMUsesBrowserBase64Rules(t *testing.T) {
	i, err := NewIntegrity("fixture-agent", "en", "fixture-device")
	require.NoError(t, err)
	dx := encodeIntegrityChallenge(t, i.key, [][]any{{3, "fixture-base64"}})
	for _, encoded := range []string{dx, strings.TrimRight(dx, "="), " \t" + dx + "\r\n"} {
		token, err := i.solveVM(t.Context(), encoded)
		require.NoError(t, err)
		require.Equal(t, base64.StdEncoding.EncodeToString([]byte("fixture-base64")), token)
	}
}

func TestIntegrityConcurrentChallengesDoNotShareRegisters(t *testing.T) {
	var wg sync.WaitGroup
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			i, err := NewIntegrity("fixture-agent", "en", fmt.Sprint(n))
			require.NoError(t, err)
			text := fmt.Sprintf("account-%d", n)
			body, err := json.Marshal(map[string]any{"turnstile": map[string]any{"required": true, "dx": encodeIntegrityChallenge(t, i.key, [][]any{{3, text}})}})
			require.NoError(t, err)
			headers, err := i.Headers(t.Context(), body)
			require.NoError(t, err)
			require.Equal(t, base64.StdEncoding.EncodeToString([]byte(text)), headers.Get("OpenAI-Sentinel-Turnstile-Token"))
		}(n)
	}
	wg.Wait()
}

func BenchmarkIntegrityPrepare(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		_, err := NewIntegrity("fixture-agent", "en", "fixture-device")
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIntegrityVM(b *testing.B) {
	i, err := NewIntegrity("fixture-agent", "en", "fixture-device")
	require.NoError(b, err)
	dx := encodeIntegrityChallenge(b, i.key, [][]any{{3, "fixture-token"}})
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := i.solveVM(context.Background(), dx); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkIntegrityProof(b *testing.B) {
	i, err := NewIntegrity("fixture-agent", "en", "fixture-device")
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := i.proof(context.Background(), "fixture", "00", 500000); err != nil {
			b.Fatal(err)
		}
	}
}
