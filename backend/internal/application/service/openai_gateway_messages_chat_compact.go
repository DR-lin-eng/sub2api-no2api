package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/shared/apicompat"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// forwardAnthropicCompactViaRawChatCompletions handles Claude Code's client
// compaction turn for accounts whose upstream only exposes Chat Completions.
// The ordinary Messages bridge can answer a small summary prompt directly,
// but an overlong transcript must be chunked before it reaches the smaller
// chat context window.  This path mirrors the Responses compact fallback and
// returns a normal Anthropic text response, which is exactly what Claude Code
// expects for its client-side compaction turn.
func (s *OpenAIGatewayService) forwardAnthropicCompactViaRawChatCompletions(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	req *apicompat.AnthropicRequest,
	originalModel string,
	defaultMappedModel string,
	clientStream bool,
	startTime time.Time,
) (*OpenAIForwardResult, error) {
	if req == nil {
		return nil, errors.New("compact chat request is nil")
	}
	billingModel := resolveOpenAIForwardModel(account, req.Model, defaultMappedModel)
	upstreamModel := normalizeOpenAIModelForUpstream(account, billingModel)
	compactPrompt, transcript := buildAnthropicCompactFallbackTranscript(req)
	chunks := splitAnthropicCompactTranscriptChunks(transcript, openAIAnthropicCompactChunkTargetChars, openAIAnthropicCompactFallbackMaxChunks)
	if len(chunks) == 0 {
		return nil, errors.New("compact chat transcript is empty")
	}

	token, targetURL, err := s.resolveCCFallbackTarget(account)
	if err != nil {
		return nil, err
	}

	totalUsage := OpenAIUsage{}
	summaries := make([]string, 0, len(chunks))
	lastRequestID := ""
	for i, chunk := range chunks {
		prompt := fmt.Sprintf("Chunk %d of %d from a Claude Code conversation transcript:\n\n%s", i+1, len(chunks), chunk)
		resp, usage, requestID, requestErr := s.runAnthropicCompactChatRequest(
			ctx, c, account, token, targetURL, upstreamModel,
			openAIAnthropicCompactChunkInstructions(), prompt,
			openAIAnthropicCompactChunkMaxOutputTokens,
		)
		totalUsage = sumOpenAIUsage(totalUsage, usage)
		lastRequestID = firstNonEmpty(requestID, lastRequestID)
		if requestErr != nil {
			return &OpenAIForwardResult{
				RequestID:     lastRequestID,
				Usage:         totalUsage,
				Model:         originalModel,
				BillingModel:  billingModel,
				UpstreamModel: upstreamModel,
				Stream:        clientStream,
				Duration:      time.Since(startTime),
			}, requestErr
		}
		if resp == nil {
			return nil, errors.New("compact chat chunk response is nil")
		}
		lastRequestID = firstNonEmpty(resp.ID, lastRequestID)
		summary := strings.TrimSpace(chatMessagePlainText(resp.Choices[0].Message.Content))
		if summary == "" {
			summary = strings.TrimSpace(resp.Choices[0].Message.ReasoningContent)
		}
		if summary == "" {
			return nil, fmt.Errorf("compact chat chunk %d produced empty summary", i+1)
		}
		summaries = append(summaries, fmt.Sprintf("## Chunk %d/%d\n%s", i+1, len(chunks), summary))
	}

	finalSummary, mergeUsage, mergeRequestID, mergeErr := s.mergeAnthropicCompactChatSummaries(
		ctx, c, account, token, targetURL, upstreamModel, compactPrompt, summaries,
		openAIAnthropicCompactMergeTargetChars, 0,
	)
	totalUsage = sumOpenAIUsage(totalUsage, mergeUsage)
	lastRequestID = firstNonEmpty(mergeRequestID, lastRequestID)
	if mergeErr != nil {
		return &OpenAIForwardResult{
			RequestID:     lastRequestID,
			Usage:         totalUsage,
			Model:         originalModel,
			BillingModel:  billingModel,
			UpstreamModel: upstreamModel,
			Stream:        clientStream,
			Duration:      time.Since(startTime),
		}, mergeErr
	}

	if strings.TrimSpace(finalSummary) == "" {
		return nil, errors.New("compact chat final summary is empty")
	}
	responseID := lastRequestID
	if responseID == "" {
		responseID = "chatcmpl_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}
	anthropicResp := &apicompat.AnthropicResponse{
		ID:         responseID,
		Type:       "message",
		Role:       "assistant",
		Model:      originalModel,
		Content:    []apicompat.AnthropicContentBlock{{Type: "text", Text: finalSummary}},
		StopReason: apicompat.AnthropicStopReasonPtr("end_turn"),
		Usage: apicompat.AnthropicUsage{
			InputTokens:              totalUsage.InputTokens,
			OutputTokens:             totalUsage.OutputTokens,
			CacheCreationInputTokens: totalUsage.CacheCreationInputTokens,
			CacheReadInputTokens:     totalUsage.CacheReadInputTokens,
		},
	}
	if clientStream {
		if err := writeAnthropicResponseAsSSE(c, anthropicResp); err != nil {
			return nil, err
		}
	} else {
		c.Header("Content-Type", "application/json; charset=utf-8")
		c.JSON(http.StatusOK, anthropicResp)
	}

	logger.L().Info("openai messages chat fallback: compact summary generated",
		zap.Int64("account_id", account.ID),
		zap.String("model", originalModel),
		zap.String("upstream_model", upstreamModel),
		zap.Int("chunks", len(chunks)),
		zap.String("request_id", responseID),
	)
	return &OpenAIForwardResult{
		RequestID:     responseID,
		ResponseID:    responseID,
		Usage:         totalUsage,
		Model:         originalModel,
		BillingModel:  billingModel,
		UpstreamModel: upstreamModel,
		Stream:        clientStream,
		Duration:      time.Since(startTime),
	}, nil
}

func (s *OpenAIGatewayService) runAnthropicCompactChatRequest(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	targetURL string,
	model string,
	instructions string,
	userText string,
	maxOutputTokens int,
) (*apicompat.ChatCompletionsResponse, OpenAIUsage, string, error) {
	maxTokens := maxOutputTokens
	systemContent, err := json.Marshal(instructions)
	if err != nil {
		return nil, OpenAIUsage{}, "", err
	}
	userContent, err := json.Marshal(userText)
	if err != nil {
		return nil, OpenAIUsage{}, "", err
	}
	body, err := json.Marshal(&apicompat.ChatCompletionsRequest{
		Model: model,
		Messages: []apicompat.ChatMessage{
			{Role: "system", Content: systemContent},
			{Role: "user", Content: userContent},
		},
		MaxCompletionTokens: &maxTokens,
		Stream:              false,
	})
	if err != nil {
		return nil, OpenAIUsage{}, "", fmt.Errorf("marshal compact chat request: %w", err)
	}

	resp, err := s.sendCCUpstreamRequest(ctx, c, account, targetURL, body, false, token, account.GetOpenAIUserAgent(), "")
	if err != nil {
		return nil, OpenAIUsage{}, "", err
	}
	defer func() { _ = resp.Body.Close() }()
	requestID := resp.Header.Get("x-request-id")
	if resp.StatusCode >= 400 {
		respBody, upstreamMsg := s.readOpenAIUpstreamError(resp)
		if foErr := s.failoverOpenAIUpstreamHTTPError(ctx, c, account, resp, respBody, upstreamMsg, model); foErr != nil {
			return nil, OpenAIUsage{}, requestID, foErr
		}
		if strings.TrimSpace(upstreamMsg) == "" {
			upstreamMsg = string(respBody)
		}
		return nil, OpenAIUsage{}, requestID, fmt.Errorf("compact chat upstream status %d: %s", resp.StatusCode, sanitizeUpstreamErrorMessage(upstreamMsg))
	}

	respBody, err := ReadUpstreamResponseBody(resp.Body, s.cfg, c, openAITooLargeError)
	if err != nil {
		return nil, OpenAIUsage{}, requestID, fmt.Errorf("read compact chat response: %w", err)
	}
	var chatResp apicompat.ChatCompletionsResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return nil, OpenAIUsage{}, requestID, fmt.Errorf("parse compact chat response: %w", err)
	}
	usage, _ := extractOpenAIUsageFromJSONBytes(respBody)
	if len(chatResp.Choices) == 0 {
		return nil, usage, requestID, errors.New("compact chat response has no choices")
	}
	choice := chatResp.Choices[0]
	if strings.TrimSpace(choice.FinishReason) == "length" {
		return nil, usage, requestID, errors.New("compact chat response ended at max output tokens")
	}
	return &chatResp, usage, requestID, nil
}

func (s *OpenAIGatewayService) mergeAnthropicCompactChatSummaries(
	ctx context.Context,
	c *gin.Context,
	account *Account,
	token string,
	targetURL string,
	model string,
	compactPrompt string,
	summaries []string,
	targetChars int,
	depth int,
) (string, OpenAIUsage, string, error) {
	if len(summaries) == 0 {
		return "", OpenAIUsage{}, "", errors.New("compact chat merge summaries are empty")
	}
	if depth > openAIAnthropicCompactMergeMaxDepth {
		return "", OpenAIUsage{}, "", errors.New("compact chat merge exceeded recursive depth")
	}
	groups := groupAnthropicCompactSummariesForMerge(compactPrompt, summaries, targetChars)
	if len(groups) > 1 {
		totalUsage := OpenAIUsage{}
		merged := make([]string, 0, len(groups))
		lastRequestID := ""
		for i, group := range groups {
			text, usage, requestID, err := s.mergeAnthropicCompactChatSummaries(ctx, c, account, token, targetURL, model, compactPrompt, group, targetChars, depth+1)
			totalUsage = sumOpenAIUsage(totalUsage, usage)
			lastRequestID = firstNonEmpty(requestID, lastRequestID)
			if err != nil {
				return "", totalUsage, lastRequestID, err
			}
			merged = append(merged, fmt.Sprintf("## Summary group %d/%d\n%s", i+1, len(groups), text))
		}
		return s.mergeAnthropicCompactChatSummaries(ctx, c, account, token, targetURL, model, compactPrompt, merged, targetChars, depth+1)
	}

	mergePrompt := buildAnthropicCompactMergePrompt(compactPrompt, summaries)
	resp, usage, requestID, err := s.runAnthropicCompactChatRequest(ctx, c, account, token, targetURL, model, openAIAnthropicCompactMergeInstructions(), mergePrompt, openAIAnthropicCompactMergeMaxOutputTokens)
	if err == nil && resp != nil && len(resp.Choices) > 0 {
		text := strings.TrimSpace(chatMessagePlainText(resp.Choices[0].Message.Content))
		if text == "" {
			text = strings.TrimSpace(resp.Choices[0].Message.ReasoningContent)
		}
		if text != "" {
			return text, usage, firstNonEmpty(resp.ID, requestID), nil
		}
		err = errors.New("compact chat merge produced empty summary")
	}
	if err != nil && !isCompactChatContextError(err) {
		return "", usage, requestID, err
	}
	if err != nil && targetChars > openAIAnthropicCompactFallbackMinSplitRunes && depth < openAIAnthropicCompactMergeMaxDepth {
		nextTarget := targetChars / 2
		if nextTarget < openAIAnthropicCompactFallbackMinSplitRunes {
			nextTarget = openAIAnthropicCompactFallbackMinSplitRunes
		}
		retryGroups := retryAnthropicCompactFallbackSummaries(compactPrompt, summaries, nextTarget)
		if len(retryGroups) > 0 && nextTarget < targetChars {
			retryText, retryUsage, retryID, retryErr := s.mergeAnthropicCompactChatSummaries(ctx, c, account, token, targetURL, model, compactPrompt, retryGroups, nextTarget, depth+1)
			return retryText, sumOpenAIUsage(usage, retryUsage), firstNonEmpty(retryID, requestID), retryErr
		}
	}
	// Keep the compact turn useful even when the merge model rejects the final
	// prompt.  This mirrors the Responses path's emergency guard.
	return buildAnthropicCompactEmergencySummary(compactPrompt, summaries), usage, requestID, nil
}

func isCompactChatContextError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	for _, marker := range []string{
		"context length",
		"context window",
		"context_length_exceeded",
		"maximum context",
		"too many tokens",
		"prompt is too long",
		"input is too long",
	} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}
