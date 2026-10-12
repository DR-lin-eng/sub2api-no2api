package repository

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"time"
)

// Both HTTP and HTTPS proxies receive a CONNECT to the already validated IP.
// TLS to the proxy and TLS to the image server use separate verified identities.
func dialPublicImageProxy(ctx context.Context, network, destination string, proxyURL *url.URL, proxyTLS *tls.Config) (net.Conn, error) {
	port := proxyURL.Port()
	if port == "" {
		port = "80"
		if proxyURL.Scheme == "https" {
			port = "443"
		}
	}
	address := net.JoinHostPort(proxyURL.Hostname(), port)
	var conn net.Conn
	var err error
	if proxyURL.Scheme == "https" {
		cfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if proxyTLS != nil {
			cfg = proxyTLS.Clone()
		}
		cfg.ServerName = proxyURL.Hostname()
		conn, err = (&tls.Dialer{NetDialer: newUpstreamDialer(), Config: cfg}).DialContext(ctx, network, address)
	} else {
		conn, err = newUpstreamDialer().DialContext(ctx, network, address)
	}
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = conn.Close()
		}
	}()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	deadline := time.Now().Add(defaultUpstreamDialTimeout)
	if ctxDeadline, ok := ctx.Deadline(); ok && ctxDeadline.Before(deadline) {
		deadline = ctxDeadline
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return nil, err
	}
	connect := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: destination}, Host: destination, Header: make(http.Header)}
	if proxyURL.User != nil {
		password, _ := proxyURL.User.Password()
		credentials := base64.StdEncoding.EncodeToString([]byte(proxyURL.User.Username() + ":" + password))
		connect.Header.Set("Proxy-Authorization", "Basic "+credentials)
	}
	if err := connect.Write(conn); err != nil {
		return nil, err
	}
	reader := bufio.NewReader(conn)
	response, err := http.ReadResponse(reader, connect)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("public image proxy CONNECT returned status %d", response.StatusCode)
	}
	if err := conn.SetDeadline(time.Time{}); err != nil {
		return nil, err
	}
	success = true
	return &publicImageProxyConn{Conn: conn, reader: reader}, nil
}

type publicImageProxyConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *publicImageProxyConn) Read(p []byte) (int, error) { return c.reader.Read(p) }
