package config

import (
	"fmt"
	"golang.org/x/net/http/httpguts"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

// MCPConfig fixes the public resource URL used by OAuth discovery and tokens.
// Setting it is also the explicit opt-in that enables MCP.
type MCPConfig struct {
	PublicURL string `mapstructure:"public_url"`
}

// Enabled reports whether the operator opted in to the management MCP endpoint.
func (c MCPConfig) Enabled() bool { return strings.TrimSpace(c.PublicURL) != "" }

// Validate normalizes the configured resource URL without trusting request headers.
// An empty URL keeps MCP and its OAuth endpoints disabled.
func (c *MCPConfig) Validate() error {
	raw := strings.TrimSpace(c.PublicURL)
	if raw == "" {
		c.PublicURL = ""
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("mcp.public_url invalid: %w", err)
	}
	if !u.IsAbs() || u.Hostname() == "" || !httpguts.ValidHostHeader(u.Host) || strings.ContainsAny(u.Host, "*%") {
		return fmt.Errorf("mcp.public_url must be an absolute URL with a valid host")
	}
	if u.User != nil || u.RawQuery != "" || u.ForceQuery || strings.Contains(raw, "#") {
		return fmt.Errorf("mcp.public_url must not include userinfo, query, or fragment")
	}
	if u.Path != "/api/v1/mcp" || u.EscapedPath() != "/api/v1/mcp" {
		return fmt.Errorf("mcp.public_url must have the exact path /api/v1/mcp without a trailing slash")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("mcp.public_url must have a valid port")
		}
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	host := u.Hostname()
	addr, _ := netip.ParseAddr(host)
	localDevelopment := host == "localhost" || addr.IsLoopback()
	if u.Scheme != "https" && (u.Scheme != "http" || !localDevelopment) {
		return fmt.Errorf("mcp.public_url must use HTTPS (HTTP is allowed only for localhost or loopback IPs)")
	}
	c.PublicURL = u.String()
	return nil
}
