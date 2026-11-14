package lenovoattribution

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestAppResponseAndRetryContract(t *testing.T) {
	now := time.Now().UTC()
	for _, tc := range []struct {
		name     string
		status   int
		values   []string
		digest   bool
		recorded bool
		delay    time.Duration
		terminal bool
	}{
		{"recorded", 204, []string{"1"}, true, true, 7 * 24 * time.Hour, false},
		{"no digest", 204, []string{"1"}, false, false, 0, true},
		{"not recorded", 204, []string{"0"}, true, false, 6 * time.Hour, false},
		{"missing", 204, nil, true, false, 6 * time.Hour, false},
		{"whitespace", 204, []string{" 1"}, true, false, 6 * time.Hour, false},
		{"duplicate", 204, []string{"1", "1"}, true, false, 6 * time.Hour, false},
		{"200 ignores header", 200, []string{"1"}, true, false, 0, true},
		{"unauthorized", 401, []string{"1"}, true, false, 0, true},
		{"timeout status", 408, nil, true, false, 0, true},
		{"rate limit", 429, nil, true, false, 0, true},
		{"server failure", 503, []string{"1"}, true, false, 0, false},
		{"network failure", 0, nil, true, false, 0, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			if tc.values != nil {
				h[http.CanonicalHeaderKey(RecordedHeader)] = tc.values
			}
			state := (State{LeaseID: "lease", LeaseUntil: now.Add(time.Minute)}).Complete(tc.status, h, tc.digest, now)
			require.Equal(t, tc.recorded, state.DeviceRecorded)
			require.Equal(t, tc.terminal, state.Terminal)
			require.Empty(t, state.LeaseID)
			if !state.Terminal {
				require.Equal(t, now.Add(tc.delay), state.NextRetryAt)
			}
		})
	}
	s := State{}
	for _, delay := range []time.Duration{0, time.Second, 5 * time.Second, 30 * time.Second, 15 * time.Minute, 15 * time.Minute} {
		s = s.Complete(503, nil, false, now)
		require.Equal(t, now.Add(delay), s.NextRetryAt)
	}
}
