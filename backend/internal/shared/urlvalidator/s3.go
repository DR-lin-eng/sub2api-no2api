package urlvalidator

import (
	"fmt"
	"net/url"
	"strings"
)

// An empty endpoint selects the SDK's AWS endpoint. DNS answers still require
// public-IP validation at connection time, independently of upstream settings.
func ValidateS3Endpoint(raw string) (string, error) {
	if strings.TrimSpace(raw) == "" {
		return "", nil
	}
	normalized, err := ValidatePublicHTTPURL(raw, true)
	if err != nil {
		return "", fmt.Errorf("S3 endpoint must be a public HTTP(S) URL without credentials")
	}
	parsed, err := url.Parse(normalized)
	if err != nil || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" {
		return "", fmt.Errorf("S3 endpoint must not contain a query or fragment")
	}
	return normalized, nil
}
