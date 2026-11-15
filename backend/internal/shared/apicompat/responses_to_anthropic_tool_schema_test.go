package apicompat

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
)

func TestAnthropicRootUnionFlattensToolBranchesWithBoundedWork(t *testing.T) {
	body := json.RawMessage(`{"oneOf":[{"type":"object","properties":{"action":{"const":"read"},"shared":{"type":"string"}},"required":["action","shared"]},{"type":"object","properties":{"action":{"const":"write"},"value":{"type":"string"}},"required":["action","value"]}]}`)
	normalized := normalizeAnthropicInputSchema(body)
	require.False(t, gjson.GetBytes(normalized, "oneOf").Exists())
	require.Equal(t, "object", gjson.GetBytes(normalized, "type").String())
	require.Equal(t, `["action"]`, gjson.GetBytes(normalized, "required").Raw)
	require.Equal(t, int64(2), gjson.GetBytes(normalized, "properties.action.anyOf.#").Int())
	require.True(t, gjson.GetBytes(normalized, "properties.value").Exists())
	tooWide := json.RawMessage(`{"anyOf":[` + strings.TrimSuffix(strings.Repeat(`{"type":"object"},`, anthropicToolUnionMaxBranches+1), ",") + `]}`)
	require.Equal(t, tooWide, normalizeAnthropicInputSchema(tooWide), "unsupported schemas must not be silently weakened")
}
