package service

import (
	"encoding/json"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
)

const codexMetadataProjectionContextKey = "codex_outbound_metadata_projection"

// This request-local receipt is written only after the body has passed its
// OAuth boundary. Header construction must reuse that projection rather than
// treating its derived identifiers as new caller input and hashing them again.
type codexMetadataProjection struct {
	accountID      int64
	accountKey     string
	turnMetadata   string
	sessionIDs     *codexOutboundSessionIDs
	fingerprintIDs *codexFingerprintIDs
}

func stageCodexMetadataProjection(metadata map[string]any, account *Account, sessionIDs *codexOutboundSessionIDs, fingerprintIDs *codexFingerprintIDs, contexts ...*gin.Context) {
	for _, c := range contexts {
		if c == nil {
			continue
		}
		projection := &codexMetadataProjection{sessionIDs: sessionIDs, fingerprintIDs: fingerprintIDs}
		if account != nil {
			projection.accountID, projection.accountKey = account.ID, account.CodexVirtualClientKey()
		}
		projection.turnMetadata, _ = metadata[openAIWSTurnMetadataHeader].(string)
		c.Set(codexMetadataProjectionContextKey, projection)
	}
}

func resolvedCodexMetadataProjection(c *gin.Context, account *Account, body []byte) *codexMetadataProjection {
	if c == nil || account == nil || !account.IsOpenAIOAuth() {
		return nil
	}
	if !gjson.GetBytes(body, "client_metadata").IsObject() {
		return nil
	}
	value, _ := c.Get(codexMetadataProjectionContextKey)
	projection, _ := value.(*codexMetadataProjection)
	if projection == nil || projection.turnMetadata != codexBodyMetadataValue(body, openAIWSTurnMetadataHeader) {
		return nil
	}
	if projection.fingerprintIDs != nil {
		if resolveCodexFingerprintIDsFromGinContext(account, c) != projection.fingerprintIDs {
			return nil
		}
	} else if account.ID != projection.accountID || account.CodexVirtualClientKey() != projection.accountKey {
		return nil
	}
	return projection
}

func codexProjectedTurnMetadataHeader(raw string) string {
	var metadata map[string]any
	if json.Unmarshal([]byte(raw), &metadata) != nil || metadata == nil {
		return ""
	}
	// Match the Codex compatibility header: the tool inventory belongs in the
	// body and must not inflate a HTTP/WS handshake header.
	delete(metadata, "tool_namespaces_info")
	encoded, err := json.Marshal(metadata)
	if err != nil || len(encoded) > codexWorkspaceMetadataMaxBytes {
		return ""
	}
	return string(encoded)
}
