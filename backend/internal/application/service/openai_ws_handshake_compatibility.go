package service

import "crypto/sha256"

// Handshake identity is immutable for the life of an upstream socket.
type openAIWSHandshakeCompatibilityKey struct {
	targetURL       string
	userAgent       string
	originator      string
	version         string
	installationID  string
	windowID        string
	betaHeader      string
	betaFeatures    string
	routingOverride string
	responsesLite   string
	subagent        string
	guardian        string
	memgen          string
	timingMetrics   string
	authHash        [sha256.Size]byte
	fingerprintKey  string
	turnStateHash   [sha256.Size]byte
	tlsProfileKey   string
	egressRouteKey  string
}
