package service

import (
	"github.com/Wei-Shaw/sub2api/internal/shared/antigravity"
	"github.com/tidwall/gjson"
)

// thinkingBlocksNeedFiltering reports whether filterThinkingBlocksDecoded would
// drop a block, mirroring its decisions on borrowed gjson views with
// encoding/json's last-wins rule for duplicate keys. The body is borrowed only
// during this call. A non-object root is left to the decoding path
// (decided=false). Invalid JSON needs no separate scan: the decoding path
// returns it unchanged, so either answer yields the same body.
func thinkingBlocksNeedFiltering(body []byte, alwaysThinking bool) (needs, decided bool) {
	view := requestJSONReadOnlyString(body)
	root := gjson.Parse(view)
	if !root.IsObject() {
		return false, false
	}
	var thinking, messages gjson.Result
	root.ForEach(func(key, value gjson.Result) bool {
		switch key.String() {
		case "thinking":
			thinking = value
		case "messages":
			messages = value
		}
		return true
	})

	thinkingEnabled := alwaysThinking
	if thinking.IsObject() {
		var thinkType gjson.Result
		thinking.ForEach(func(key, value gjson.Result) bool {
			if key.String() == "type" {
				thinkType = value
			}
			return true
		})
		if thinkType.Type == gjson.String && (thinkType.Str == "enabled" || thinkType.Str == "adaptive") {
			thinkingEnabled = true
		}
	}
	if !messages.IsArray() {
		return false, true
	}

	messages.ForEach(func(_, msg gjson.Result) bool {
		if !msg.IsObject() {
			return true
		}
		var role, content gjson.Result
		msg.ForEach(func(key, value gjson.Result) bool {
			switch key.String() {
			case "role":
				role = value
			case "content":
				content = value
			}
			return true
		})
		if !content.IsArray() {
			return true
		}
		isAssistant := role.Type == gjson.String && role.Str == "assistant"
		content.ForEach(func(_, block gjson.Result) bool {
			if !block.IsObject() {
				return true
			}
			var blockType, data, signature gjson.Result
			hasThinking := false
			block.ForEach(func(key, value gjson.Result) bool {
				switch key.String() {
				case "type":
					blockType = value
				case "data":
					data = value
				case "signature":
					signature = value
				case "thinking":
					hasThinking = true
				}
				return true
			})
			typ := ""
			if blockType.Type == gjson.String {
				typ = blockType.Str
			}
			switch {
			case typ == "thinking" || typ == "redacted_thinking":
				if thinkingEnabled && isAssistant {
					if alwaysThinking && typ == "redacted_thinking" && data.Type == gjson.String && data.Str != "" {
						return true
					}
					if signature.Type == gjson.String && signature.Str != "" && signature.Str != antigravity.DummyThoughtSignature {
						return true
					}
				}
				needs = true
			case typ == "" && hasThinking:
				needs = true
			}
			return !needs
		})
		return !needs
	})
	return needs, true
}
