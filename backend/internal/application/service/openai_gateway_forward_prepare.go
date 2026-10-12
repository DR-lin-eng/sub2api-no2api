package service

import (
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
)

// prepareOpenAIForwardOAuthRequest owns the existing decoded OAuth preparation.
// It reuses the caller's map: no extra body decode, identity lookup or allocation
// is introduced by moving this responsibility out of the Forward orchestrator.
func (s *OpenAIGatewayService) prepareOpenAIForwardOAuthRequest(
	c *gin.Context, account *Account, decoded map[string]any, opts codexOAuthTransformOptions,
) (codexTransformResult, *codexFingerprintIDs, error) {
	modified := false
	// Group reasoning has already been applied before account selection.
	if overridden, err := applyCodexPrewarmContinuationReasoningOverride(c, account, decoded); err != nil {
		return codexTransformResult{}, nil, err
	} else if overridden {
		modified = true
		logOpenAIWSModeInfo("prewarm_reasoning_override account_id=%d effort=none", account.ID)
	}
	result := applyCodexOAuthTransformWithOptions(decoded, opts)
	if opts.SkipDefaultInstructions {
		ensureCodexOAuthInstructionsField(decoded)
		modified = true
	}
	if !opts.IsCompact && applyCodexClientMetadata(decoded, account) {
		modified = true
	}
	var ids *codexFingerprintIDs
	if !opts.IsCompact || s.codexFullSimulationEnabledForAccount(c, account) {
		ids = resolveCodexFingerprintIDsFromGinContext(account, c)
		if applyCodexFingerprintClientMetadata(decoded, ids, c) {
			modified = true
		}
	}
	result.Modified = result.Modified || modified
	return result, ids, nil
}

// normalizeOpenAIForwardLiteRequest retains the existing HTTP error contract.
func normalizeOpenAIForwardLiteRequest(c *gin.Context, account *Account, body []byte) ([]byte, error) {
	liteBody, changed, err := normalizeOpenAIResponsesLitePayloadForAccount(body, account)
	if err != nil {
		param := "tools"
		var validationErr *openAIResponsesLiteValidationError
		if errors.As(err, &validationErr) {
			param = validationErr.param
		}
		setOpsUpstreamError(c, http.StatusBadRequest, err.Error(), "")
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{
			"type": "invalid_request_error", "message": err.Error(), "param": param,
		}})
		return body, err
	}
	if changed {
		return liteBody, nil
	}
	return body, nil
}

func codexForwardOAuthOptions(isCodexCLI, compact, lite, compatMessagesBridge bool) codexOAuthTransformOptions {
	return codexOAuthTransformOptions{
		IsCodexCLI:              isCodexCLI,
		IsCompact:               compact,
		SkipDefaultInstructions: compatMessagesBridge,
		PreserveToolCallIDs:     compatMessagesBridge,
		ResponsesLite:           lite,
	}
}
