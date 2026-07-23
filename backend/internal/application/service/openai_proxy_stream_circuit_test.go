package service

import (
	"sync"
	"testing"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestOpenAIProxyStreamCircuitThresholdTTLAndSuccessReset(t *testing.T) {
	base := time.Unix(1_800_000_000, 0)
	circuit := newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		failureThreshold: 2,
		failureWindow:    time.Minute,
		quarantineTTL:    10 * time.Minute,
		maxEntries:       16,
	})

	tripped, _ := circuit.recordFailure(1, base, base)
	require.False(t, tripped)
	require.False(t, circuit.isBlocked(1, base))
	require.True(t, circuit.recordSuccess(1, base.Add(time.Second), base.Add(time.Second)))

	tripped, _ = circuit.recordFailure(1, base.Add(10*time.Second), base.Add(10*time.Second))
	require.False(t, tripped, "success must clear the previous failure observation")
	tripped, until := circuit.recordFailure(1, base.Add(20*time.Second), base.Add(20*time.Second))
	require.True(t, tripped)
	require.Equal(t, base.Add(20*time.Second+10*time.Minute), until)
	require.True(t, circuit.isBlocked(1, until.Add(-time.Nanosecond)))
	require.False(t, circuit.isBlocked(1, until), "TTL expiry must re-admit the proxy")

	tripped, _ = circuit.recordFailure(2, base, base)
	require.False(t, tripped)
	tripped, _ = circuit.recordFailure(2, base.Add(2*time.Minute), base.Add(2*time.Minute))
	require.False(t, tripped, "failures outside the window must not accumulate")
}

func TestOpenAIProxyStreamCircuitBoundsEntries(t *testing.T) {
	base := time.Unix(1_800_000_000, 0)
	circuit := newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		failureThreshold: 1,
		failureWindow:    time.Minute,
		quarantineTTL:    10 * time.Minute,
		maxEntries:       2,
	})

	circuit.recordFailure(1, base, base)
	circuit.recordFailure(2, base.Add(time.Second), base.Add(time.Second))
	circuit.recordFailure(3, base.Add(2*time.Second), base.Add(2*time.Second))

	circuit.mu.Lock()
	defer circuit.mu.Unlock()
	require.Len(t, circuit.entries, 2)
	_, oldestRetained := circuit.entries[1]
	require.False(t, oldestRetained, "the oldest entry must be evicted at the bound")
}

func TestOpenAIProxyStreamCircuitIgnoresOutOfOrderOutcomes(t *testing.T) {
	base := time.Unix(1_800_000_000, 0)
	circuit := newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		failureThreshold: 1,
		failureWindow:    time.Minute,
		quarantineTTL:    10 * time.Minute,
		maxEntries:       16,
	})

	olderRequest := base
	newerRequest := base.Add(time.Second)
	tripped, _ := circuit.recordFailure(1, newerRequest, base.Add(2*time.Second))
	require.True(t, tripped)
	require.True(t, circuit.isBlocked(1, base.Add(2*time.Second)))
	require.False(t, circuit.recordSuccess(1, olderRequest, base.Add(3*time.Second)))
	require.True(t, circuit.isBlocked(1, base.Add(3*time.Second)), "an older success must not clear a newer quarantine")

	require.True(t, circuit.recordSuccess(1, newerRequest.Add(time.Second), base.Add(4*time.Second)))
	require.False(t, circuit.isBlocked(1, base.Add(4*time.Second)))
	tripped, _ = circuit.recordFailure(1, olderRequest, base.Add(5*time.Second))
	require.False(t, tripped, "an older failure must not override a newer success")
	require.False(t, circuit.isBlocked(1, base.Add(5*time.Second)))
}

func TestOpenAIProxyStreamCircuitConcurrentSnapshotAccess(t *testing.T) {
	base := time.Unix(1_800_000_000, 0)
	circuit := newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
		failureThreshold: 1,
		failureWindow:    time.Minute,
		quarantineTTL:    10 * time.Minute,
		maxEntries:       64,
	})

	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 1_000; i++ {
				proxyID := int64((worker+i)%32 + 1)
				startedAt := base.Add(time.Duration(worker*1_000+i) * time.Nanosecond)
				observedAt := startedAt.Add(time.Microsecond)
				if i%3 == 0 {
					circuit.recordSuccess(proxyID, startedAt, observedAt)
				} else {
					circuit.recordFailure(proxyID, startedAt, observedAt)
				}
			}
		}()
	}
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 10_000; i++ {
				_ = circuit.isBlocked(int64(i%32+1), base.Add(time.Second))
			}
		}()
	}
	wg.Wait()
}

func BenchmarkOpenAIProxyStreamCircuitSchedulerLookup(b *testing.B) {
	proxyID := int64(4698)
	account := &Account{Platform: PlatformOpenAI, ProxyID: &proxyID}

	b.Run("disabled", func(b *testing.B) {
		svc := &OpenAIGatewayService{cfg: &config.Config{}}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_ = svc.isOpenAIProxyStreamQuarantined(account)
		}
	})

	b.Run("enabled_blocked", func(b *testing.B) {
		cfg := &config.Config{}
		cfg.Gateway.OpenAIProxyStreamCircuit.Enabled = true
		circuit := newOpenAIProxyStreamCircuit(openAIProxyStreamCircuitSettings{
			failureThreshold: 1,
			failureWindow:    time.Minute,
			quarantineTTL:    10 * time.Minute,
			maxEntries:       16,
		})
		now := time.Now()
		circuit.recordFailure(proxyID, now, now)
		svc := &OpenAIGatewayService{cfg: cfg, openaiProxyStreamCircuit: circuit}
		require.True(b, svc.isOpenAIProxyStreamQuarantined(account))
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = svc.isOpenAIProxyStreamQuarantined(account)
		}
	})
}
