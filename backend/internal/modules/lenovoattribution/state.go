// Package lenovoattribution owns the Lenovo preload attribution wire contract
// and durable next-check state. It does not claim a physical device identity.
package lenovoattribution

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/google/uuid"
)

const (
	ExtraKey       = "openai_lenovo_attribution"
	DigestExtraKey = "lenovo_device_serial_digest"
	Endpoint       = "https://chatgpt.com/backend-api/incentives/lenovo/preload-attribution"
	RecordedHeader = "X-Lenovo-Device-Recorded"
	LeaseDuration  = 45 * time.Second
)

var ErrInvalidState = errors.New("unsupported Lenovo attribution state")

// State is stored in accounts.extra. No token, raw serial number or digest is
// saved here; Binding separates account/user/digest changes without disclosure.
type State struct {
	Version        int       `json:"version"`
	Binding        string    `json:"binding"`
	InstallationID string    `json:"installation_id"`
	StatusCode     int       `json:"status_code"`
	HeaderValue    string    `json:"header_value"`
	DigestSent     bool      `json:"digest_sent"`
	DeviceRecorded bool      `json:"device_recorded"`
	Attempts       int       `json:"attempts"`
	NextRetryAt    time.Time `json:"next_retry_at"`
	CheckedAt      time.Time `json:"checked_at"`
	Terminal       bool      `json:"terminal"`
	LeaseID        string    `json:"lease_id"`
	LeaseUntil     time.Time `json:"lease_until"`
}

func Read(extra map[string]any) (*State, error) {
	value := extra[ExtraKey]
	if value == nil {
		return nil, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, ErrInvalidState
	}
	var state State
	if json.Unmarshal(raw, &state) != nil || state.Version != 1 || state.Binding == "" ||
		state.Attempts < 0 || state.Attempts > 5 {
		return nil, ErrInvalidState
	}
	if _, err := uuid.Parse(state.InstallationID); err != nil {
		return nil, ErrInvalidState
	}
	return &state, nil
}

func (s State) Due(binding string, now time.Time) bool {
	if s.Binding != binding {
		return !now.Before(s.LeaseUntil)
	}
	return !s.Terminal && !now.Before(s.NextRetryAt) && !now.Before(s.LeaseUntil)
}

type Payload struct {
	InstallationID     string `json:"installation_id"`
	DeviceSerialDigest string `json:"device_serial_digest,omitempty"`
}

// Complete mirrors the App's response handling. An exact single "1" on 204
// with a submitted digest earns a seven-day check interval. Without a digest,
// 204 ends the attempt; it does not prove that a device was recorded.
func (s State) Complete(status int, headers http.Header, digestSent bool, now time.Time) State {
	s.StatusCode, s.DigestSent, s.CheckedAt = status, digestSent, now.UTC()
	s.HeaderValue, s.DeviceRecorded, s.LeaseID, s.LeaseUntil = "", false, "", time.Time{}
	s.NextRetryAt, s.Terminal = time.Time{}, false
	if status == http.StatusNoContent {
		values := headers.Values(RecordedHeader)
		if len(values) == 1 && (values[0] == "1" || values[0] == "0") {
			s.HeaderValue = values[0]
		}
		s.DeviceRecorded = digestSent && s.HeaderValue == "1"
		s.Attempts = 0
		if digestSent {
			delay := 6 * time.Hour
			if s.DeviceRecorded {
				delay = 7 * 24 * time.Hour
			}
			s.NextRetryAt = now.Add(delay).UTC()
		} else {
			s.Terminal = true
		}
		return s
	}
	if status > 0 && status < 500 {
		s.Attempts, s.Terminal = 0, true
		return s
	}
	delays := [...]time.Duration{0, time.Second, 5 * time.Second, 30 * time.Second, 15 * time.Minute}
	index := min(s.Attempts, len(delays)-1)
	s.Attempts = min(s.Attempts+1, len(delays))
	s.NextRetryAt = now.Add(delays[index]).UTC()
	return s
}
