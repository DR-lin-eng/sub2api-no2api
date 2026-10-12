//go:build unit

package service

import (
	"fmt"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/stretchr/testify/require"
)

func TestConfiguredHTTPBaseURLAcrossServices(t *testing.T) {
	for _, allowlistEnabled := range []bool{false, true} {
		for _, legacyHTTPFlag := range []bool{false, true} {
			cfg := &config.Config{Security: config.SecurityConfig{URLAllowlist: config.URLAllowlistConfig{
				Enabled: allowlistEnabled, AllowInsecureHTTP: legacyHTTPFlag,
				UpstreamHosts: []string{"upstream.example.com"},
			}}}
			validators := []struct {
				name     string
				validate func(string) (string, error)
			}{
				{"account test", (&AccountTestService{cfg: cfg}).validateUpstreamBaseURL},
				{"Anthropic gateway", (&GatewayService{cfg: cfg}).validateUpstreamBaseURL},
				{"OpenAI gateway", (&OpenAIGatewayService{cfg: cfg}).validateUpstreamBaseURL},
				{"Gemini messages", (&GeminiMessagesCompatService{cfg: cfg}).validateUpstreamBaseURL},
				{"OAuth relay", func(raw string) (string, error) { return validateOpenAIOAuthCustomRelayBaseURL(raw, cfg) }},
				{"CN quota probe", func(raw string) (string, error) { return cnValidateProbeURL(cfg, raw) }},
			}
			for _, validator := range validators {
				t.Run(fmt.Sprintf("%s/allowlist=%t/legacyHTTP=%t", validator.name, allowlistEnabled, legacyHTTPFlag), func(t *testing.T) {
					baseURL, err := validator.validate("http://upstream.example.com/v1/")
					require.NoError(t, err)
					require.Equal(t, "http://upstream.example.com/v1", baseURL)
				})
			}
		}
	}
}
