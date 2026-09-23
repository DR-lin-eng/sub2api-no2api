//go:build unit

package service

import "testing"

func TestIsCloudflareChallengeResponse(t *testing.T) {
	for _, tc := range []struct {
		name, header, body string
		want               bool
	}{
		{name: "explicit header", header: "challenge", want: true},
		{name: "header case and whitespace", header: " Challenge ", want: true},
		{name: "legacy body marker", body: "<html>Just a moment...</html>", want: true},
		{name: "ordinary provider error", body: `{"error":"forbidden"}`, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := isCloudflareChallengeResponse(tc.header, tc.body); got != tc.want {
				t.Fatalf("isCloudflareChallengeResponse() = %v, want %v", got, tc.want)
			}
		})
	}
}
