package urlvalidator

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestS3EndpointPolicy(t *testing.T) {
	for _, endpoint := range []string{"http://127.0.0.1:8080", "http://localhost", "http://10.1.2.3", "http://169.254.169.254", "http://[::1]", "http://[::ffff:127.0.0.1]", "http://[fc00::1]", "file:///etc/passwd", "ftp://example.com", "https://user:secret@example.com", "https://example.com?a=b", "https://example.com#fragment", "http://example.com:0"} {
		t.Run(endpoint, func(t *testing.T) {
			_, err := ValidateS3Endpoint(endpoint)
			require.Error(t, err)
			require.NotContains(t, err.Error(), "secret")
		})
	}
	for _, endpoint := range []string{"", "https://s3.amazonaws.com", "https://account.r2.cloudflarestorage.com", "http://s3.example.com:9000/minio/"} {
		t.Run(endpoint, func(t *testing.T) {
			actual, err := ValidateS3Endpoint(endpoint)
			require.NoError(t, err)
			require.Equal(t, endpoint, actual)
		})
	}
}
