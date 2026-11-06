package repository

import (
	"context"
	"crypto/tls"
	"net"

	"github.com/Wei-Shaw/sub2api/internal/shared/tlsfingerprint"
	"github.com/imroc/req/v3"
	utls "github.com/refraction-networking/utls"
)

func configureCodexExperimentalTLSHandshake(client *req.Client, profile *tlsfingerprint.Profile) {
	client.SetTLSHandshake(func(ctx context.Context, addr string, plainConn net.Conn) (net.Conn, *tls.ConnectionState, error) {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			host = addr
		}
		config := client.GetTLSClientConfig()
		conn := utls.UClient(plainConn, &utls.Config{
			ServerName:                  host,
			Rand:                        config.Rand,
			Time:                        config.Time,
			RootCAs:                     config.RootCAs,
			NextProtos:                  config.NextProtos,
			ClientCAs:                   config.ClientCAs,
			InsecureSkipVerify:          config.InsecureSkipVerify,
			CipherSuites:                config.CipherSuites,
			SessionTicketsDisabled:      config.SessionTicketsDisabled,
			MinVersion:                  config.MinVersion,
			MaxVersion:                  config.MaxVersion,
			DynamicRecordSizingDisabled: config.DynamicRecordSizingDisabled,
			KeyLogWriter:                config.KeyLogWriter,
		}, utls.HelloCustom)
		// ApplyPreset mutates extension pointers, including generated key shares.
		// Reusing a spec leaves later connections without their own private keys.
		// Build it here so reconnects and concurrent dials each own the full spec.
		if err := conn.ApplyPreset(tlsfingerprint.ClientHelloSpecFromProfile(profile)); err != nil {
			return nil, nil, err
		}
		if err := conn.HandshakeContext(ctx); err != nil {
			return nil, nil, err
		}
		wrapped := &codexExperimentalTLSConn{UConn: conn}
		state := wrapped.ConnectionState()
		return wrapped, &state, nil
	})
}

// req/v3's HTTP/2 transport reads the standard-library TLS state from the
// connection as well as from the handshake callback.
type codexExperimentalTLSConn struct {
	*utls.UConn
}

func (c *codexExperimentalTLSConn) ConnectionState() tls.ConnectionState {
	state := c.UConn.ConnectionState()
	return tls.ConnectionState{
		Version:                     state.Version,
		HandshakeComplete:           state.HandshakeComplete,
		DidResume:                   state.DidResume,
		CipherSuite:                 state.CipherSuite,
		NegotiatedProtocol:          state.NegotiatedProtocol,
		NegotiatedProtocolIsMutual:  true,
		ServerName:                  state.ServerName,
		PeerCertificates:            state.PeerCertificates,
		VerifiedChains:              state.VerifiedChains,
		SignedCertificateTimestamps: state.SignedCertificateTimestamps,
		OCSPResponse:                state.OCSPResponse,
		TLSUnique:                   state.TLSUnique,
	}
}
