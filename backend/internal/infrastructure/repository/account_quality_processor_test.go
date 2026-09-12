package repository

import (
	"github.com/stretchr/testify/require"
	"testing"
)

func TestQualityProcessorDefaultsToEmbeddedWithoutChangingConfiguration(t *testing.T) {
	t.Setenv("ACCOUNT_QUALITY_RENDERER_URL", "")
	t.Setenv("ACCOUNT_QUALITY_RENDERER_TOKEN", "")
	require.IsType(t, &embeddedAccountQualityProcessor{}, NewAccountQualityArtifactProcessor())
	t.Setenv("ACCOUNT_QUALITY_RENDERER_URL", "http://existing-renderer:8090/")
	t.Setenv("ACCOUNT_QUALITY_RENDERER_TOKEN", "existing-token")
	p, ok := NewAccountQualityArtifactProcessor().(*httpAccountQualityArtifactProcessor)
	require.True(t, ok)
	require.Equal(t, "http://existing-renderer:8090", p.endpoint)
	require.Equal(t, "existing-token", p.token)
}
