//go:build unit

package service

import (
	"math"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/shared/pagination"
	"github.com/stretchr/testify/require"
)

func TestAPIKeyPaginationOverflowReturnsEmpty(t *testing.T) {
	require.Empty(t, paginateAPIKeys([]APIKey{{ID: 1}}, pagination.PaginationParams{Page: math.MaxInt, PageSize: 20}))
	require.Len(t, paginateAPIKeys([]APIKey{{ID: 1}}, pagination.PaginationParams{Page: 1, PageSize: 20}), 1)
}
