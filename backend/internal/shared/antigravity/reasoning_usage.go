package antigravity

import (
	"encoding/json"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/tidwall/gjson"
)

func (usage *GeminiUsageMetadata) UnmarshalJSON(data []byte) error {
	type wireUsage GeminiUsageMetadata
	var decoded wireUsage
	if err := json.Unmarshal(data, &decoded); err != nil {
		return err
	}
	*usage = GeminiUsageMetadata(decoded)
	usage.ReasoningTokens = apicompat.ReasoningTokensFromUsage(gjson.ParseBytes(data))
	return nil
}
