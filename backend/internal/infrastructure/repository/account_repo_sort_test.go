package repository

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/stretchr/testify/require"
)

func TestUpstreamBillingRateSortExpressionUsesPrevalidatedMetadata(t *testing.T) {
	expression := upstreamBillingRateSortExpression("accounts.extra")

	require.Contains(t, expression, service.UpstreamBillingProbeSortMetadataVersionKey)
	require.Contains(t, expression, service.UpstreamBillingProbePeakStartMinuteKey)
	require.Contains(t, expression, service.UpstreamBillingProbePeakEndMinuteKey)
	require.Contains(t, expression, "effective_rate_multiplier")
	require.NotContains(t, expression, "pg_timezone_names")
	require.NotContains(t, expression, "split_part")
	require.NotContains(t, expression, "~ '")
}
