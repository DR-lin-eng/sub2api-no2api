package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/ip"
	"github.com/Wei-Shaw/sub2api/internal/shared/logger"
	middleware2 "github.com/Wei-Shaw/sub2api/internal/transport/http/server/middleware"
	coderws "github.com/coder/websocket"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/zap"
)

func (h *OpenAIGatewayHandler) executeForcedOpenAIImageResponses(
	ctx context.Context,
	c *gin.Context,
	input openAIForcedImageRunInput,
	n int,
	emit func(map[string]any) error,
) (map[string]any, error) {
	createdAt := time.Now().Unix()
	responseID := newOpenAIForcedImageResponseID()
	imageTool := input.plan.ToolEcho()
	baseResponse := map[string]any{
		"id":         responseID,
		"object":     "response",
		"created_at": createdAt,
		"status":     "in_progress",
		"error":      nil,
		"model":      input.requestModel,
		"output":     []any{},
		"metadata":   map[string]any{},
		"tools":      []any{imageTool},
	}
	if input.plan.PreviousResponseID != "" {
		baseResponse["previous_response_id"] = input.plan.PreviousResponseID
	}
	sequence := 0
	if emit != nil {
		if err := emit(map[string]any{"type": "response.created", "sequence_number": sequence, "response": baseResponse}); err != nil {
			return nil, err
		}
		sequence++
	}

	events := make(chan openAIForcedImageChildEvent, 64)
	results := make(chan openAIForcedImageChildResult, n)
	jobs := make(chan int, n)
	for index := 0; index < n; index++ {
		jobs <- index
	}
	close(jobs)
	workerCount := min(n, openAIForcedImageChildConcurrency)
	for worker := 0; worker < workerCount; worker++ {
		go func() {
			for index := range jobs {
				result := runOpenAIForcedImageChildInGlobalSlot(ctx, index, func() openAIForcedImageChildResult {
					return h.runForcedOpenAIImageChild(ctx, c, input, index, events)
				})
				results <- result
			}
		}()
	}

	outputs := make([]any, n)
	usage := make(map[string]any)
	completed := 0
	var firstErr error
	processChildEvent := func(childEvent openAIForcedImageChildEvent) {
		if emit == nil || childEvent.event == nil {
			return
		}
		eventType, _ := childEvent.event["type"].(string)
		switch eventType {
		case "response.created", "response.completed", "response.failed", "error":
			return
		}
		childEvent.event["sequence_number"] = sequence
		if _, ok := childEvent.event["output_index"]; ok {
			childEvent.event["output_index"] = childEvent.index
		}
		if err := emit(childEvent.event); err != nil && firstErr == nil {
			firstErr = err
		}
		sequence++
	}
	for completed < n {
		select {
		case childEvent := <-events:
			processChildEvent(childEvent)
		case child := <-results:
			completed++
			if child.err != nil {
				if firstErr == nil {
					firstErr = child.err
				}
				continue
			}
			outputs[child.index] = child.output
			mergeOpenAIForcedImageUsage(usage, child.usage)
		case <-ctx.Done():
			if firstErr == nil {
				firstErr = ctx.Err()
			}
		}
	}
drainEvents:
	for {
		select {
		case childEvent := <-events:
			processChildEvent(childEvent)
		default:
			break drainEvents
		}
	}

	finalOutputs := make([]any, 0, n)
	for _, output := range outputs {
		if output != nil {
			finalOutputs = append(finalOutputs, output)
		}
	}
	finalResponse := cloneOpenAIForcedImageMap(baseResponse)
	finalResponse["output"] = finalOutputs
	finalResponse["usage"] = usage
	if firstErr != nil {
		finalResponse["status"] = "failed"
		finalResponse["error"] = map[string]any{"code": "image_generation_failed", "message": firstErr.Error()}
		if emit != nil {
			_ = emit(map[string]any{"type": "response.failed", "sequence_number": sequence, "response": finalResponse})
		}
		return finalResponse, firstErr
	}
	completedAt := time.Now().Unix()
	finalResponse["status"] = "completed"
	finalResponse["completed_at"] = completedAt
	if input.apiKey != nil {
		stateItems := openAIForcedImageStateItems(finalOutputs)
		if err := h.gatewayService.StoreOpenAIForcedImageResponseState(
			ctx,
			openAIForcedImageGroupID(input.apiKey),
			responseID,
			stateItems,
		); err != nil {
			if input.reqLog != nil {
				input.reqLog.Warn("openai.forced_image.store_response_state_failed", zap.Error(err))
			}
		}
	}
	if emit != nil {
		if err := emit(map[string]any{"type": "response.completed", "sequence_number": sequence, "response": finalResponse}); err != nil {
			return finalResponse, err
		}
	}
	return finalResponse, nil
}

func openAIForcedImageStateItems(outputs []any) []service.OpenAIResponsesImageOutputItem {
	items := make([]service.OpenAIResponsesImageOutputItem, 0, len(outputs))
	for _, raw := range outputs {
		output, _ := raw.(map[string]any)
		if output == nil {
			continue
		}
		item := service.OpenAIResponsesImageOutputItem{
			ID:            openAIForcedImageOutputString(output, "id"),
			Type:          openAIForcedImageOutputString(output, "type"),
			Status:        openAIForcedImageOutputString(output, "status"),
			Result:        openAIForcedImageOutputString(output, "result"),
			RevisedPrompt: openAIForcedImageOutputString(output, "revised_prompt"),
			OutputFormat:  openAIForcedImageOutputString(output, "output_format"),
			Size:          openAIForcedImageOutputString(output, "size"),
			Quality:       openAIForcedImageOutputString(output, "quality"),
			Background:    openAIForcedImageOutputString(output, "background"),
		}
		if item.ID != "" && item.Result != "" {
			items = append(items, item)
		}
	}
	return items
}

func openAIForcedImageOutputString(output map[string]any, key string) string {
	value, _ := output[key].(string)
	return strings.TrimSpace(value)
}

func openAIForcedImageGroupID(apiKey *service.APIKey) int64 {
	if apiKey == nil || apiKey.GroupID == nil {
		return 0
	}
	return *apiKey.GroupID
}

func (h *OpenAIGatewayHandler) runForcedOpenAIImageChild(
	ctx context.Context,
	parent *gin.Context,
	input openAIForcedImageRunInput,
	index int,
	events chan<- openAIForcedImageChildEvent,
) openAIForcedImageChildResult {
	var failedAccountIDs map[int64]struct{}
	maxSwitches := h.maxAccountSwitches
	for switchCount := 0; ; switchCount++ {
		selection, _, selectErr := h.gatewayService.SelectAccountWithSchedulerForImages(
			ctx,
			input.apiKey.GroupID,
			fmt.Sprintf("%s:image:%d", input.sessionHash, index),
			input.plan.ImageModel,
			failedAccountIDs,
			service.OpenAIImagesCapabilityForcedAPI,
		)
		if selectErr != nil || selection == nil || selection.Account == nil {
			if selectErr == nil {
				selectErr = service.ErrNoAvailableAccounts
			}
			return openAIForcedImageChildResult{index: index, err: selectErr}
		}
		account := selection.Account
		childContext := parent.Copy()
		childContext.Request = parent.Request.Clone(ctx)
		writer := newOpenAIForcedImageChildWriter(parent.Writer, input.stream, func(event map[string]any) {
			if input.stream {
				events <- openAIForcedImageChildEvent{index: index, event: event}
			}
		})
		childContext.Writer = writer
		childStreamStarted := false
		release, acquired := h.acquireResponsesAccountSlot(
			childContext,
			input.apiKey.GroupID,
			fmt.Sprintf("%s:image:%d", input.sessionHash, index),
			selection,
			input.stream,
			&childStreamStarted,
			input.reqLog,
		)
		if !acquired {
			return openAIForcedImageChildResult{index: index, err: errors.New("image account is busy")}
		}
		upstreamModel := account.GetMappedModel(input.plan.ImageModel)
		start := time.Now()
		result, forwardErr := func() (*service.OpenAIForwardResult, error) {
			if release != nil {
				defer release()
			}
			return h.gatewayService.ForwardOpenAIResponsesImagePlan(
				ctx, childContext, account, input.plan, input.requestModel, upstreamModel, 1, input.stream, true, start,
			)
		}()
		if forwardErr != nil {
			var failoverErr *service.UpstreamFailoverError
			if errors.As(forwardErr, &failoverErr) && switchCount < maxSwitches {
				h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, upstreamModel, false, nil)
				h.gatewayService.RecordOpenAIAccountSwitch()
				addFailedAccountID(&failedAccountIDs, account.ID)
				continue
			}
			if result == nil || result.ImageCount <= 0 {
				return openAIForcedImageChildResult{index: index, err: forwardErr, account: account, result: result}
			}
		}
		output, usage, parseErr := openAIForcedImageOutputFromResult(result)
		if parseErr == nil && input.stream && output != nil && !writer.SawOutputItem() {
			added := cloneOpenAIForcedImageMap(output)
			added["status"] = "in_progress"
			delete(added, "result")
			events <- openAIForcedImageChildEvent{index: index, event: map[string]any{
				"type": "response.output_item.added", "output_index": 0, "item": added,
			}}
			events <- openAIForcedImageChildEvent{index: index, event: map[string]any{
				"type": "response.output_item.done", "output_index": 0, "item": output,
			}}
		}
		if parseErr != nil && forwardErr == nil {
			forwardErr = parseErr
		}
		if result != nil {
			h.gatewayService.ReportOpenAIAccountScheduleResult(account.ID, upstreamModel, openAIForwardSucceededForScheduling(result), nil)
			h.recordForcedOpenAIImageChildUsage(input, account, result)
		}
		return openAIForcedImageChildResult{index: index, output: output, usage: usage, err: forwardErr, account: account, result: result}
	}
}

func openAIForcedImageOutputFromResult(result *service.OpenAIForwardResult) (map[string]any, map[string]any, error) {
	if result == nil || len(result.ResponsesImageItems) == 0 {
		return nil, nil, errors.New("image child response did not contain output")
	}
	item := result.ResponsesImageItems[0]
	output := map[string]any{
		"id": item.ID, "type": item.Type, "status": item.Status, "result": item.Result,
	}
	for key, value := range map[string]string{
		"revised_prompt": item.RevisedPrompt,
		"output_format":  item.OutputFormat,
		"size":           item.Size,
		"quality":        item.Quality,
		"background":     item.Background,
	} {
		if value != "" {
			output[key] = value
		}
	}
	usage := make(map[string]any)
	if len(result.ResponsesImageUsage) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(result.ResponsesImageUsage))
		decoder.UseNumber()
		if err := decoder.Decode(&usage); err != nil {
			return nil, nil, fmt.Errorf("decode image child usage: %w", err)
		}
	}
	return output, usage, nil
}

func (h *OpenAIGatewayHandler) recordForcedOpenAIImageChildUsage(
	input openAIForcedImageRunInput,
	account *service.Account,
	result *service.OpenAIForwardResult,
) {
	if result == nil || account == nil {
		return
	}
	upstreamEndpoint := result.UpstreamEndpoint
	usageFields := service.ChannelMappingResult{}.ToUsageFields(input.requestModel, result.UpstreamModel)
	h.submitOpenAIUsageRecordTask(input.requestCtx, result, func(taskCtx context.Context) {
		if err := h.gatewayService.RecordUsage(taskCtx, &service.OpenAIRecordUsageInput{
			Result:             result,
			APIKey:             input.apiKey,
			User:               input.apiKey.User,
			Account:            account,
			Subscription:       input.subscription,
			InboundEndpoint:    input.inbound,
			UpstreamEndpoint:   upstreamEndpoint,
			UserAgent:          input.userAgent,
			IPAddress:          input.clientIP,
			RequestPayloadHash: input.requestHash,
			APIKeyService:      h.apiKeyService,
			QuotaPlatform:      input.quotaPlatform,
			ChannelUsageFields: usageFields,
		}); err != nil {
			logger.L().With(
				zap.String("component", "handler.openai_gateway.forced_image"),
				zap.Int64("api_key_id", input.apiKey.ID),
				zap.Int64("account_id", account.ID),
			).Error("openai.forced_image.record_usage_failed", zap.Error(err))
		}
	})
}

func mergeOpenAIForcedImageUsage(target, source map[string]any) {
	for key, value := range source {
		switch typed := value.(type) {
		case json.Number:
			current, _ := target[key].(json.Number)
			left, _ := current.Float64()
			right, _ := typed.Float64()
			target[key] = json.Number(fmt.Sprintf("%g", left+right))
		case map[string]any:
			nested, _ := target[key].(map[string]any)
			if nested == nil {
				nested = make(map[string]any)
				target[key] = nested
			}
			mergeOpenAIForcedImageUsage(nested, typed)
		default:
			if _, exists := target[key]; !exists {
				target[key] = value
			}
		}
	}
}

func writeOpenAIForcedImageHTTPEvent(c *gin.Context, event map[string]any) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}
	eventType, _ := event["type"].(string)
	if _, err := fmt.Fprintf(c.Writer, "event: %s\ndata: %s\n\n", eventType, payload); err != nil {
		return err
	}
	c.Writer.Flush()
	return nil
}

func (h *OpenAIGatewayHandler) handleForcedOpenAIImageResponses(
	c *gin.Context,
	body []byte,
	requestModel string,
	stream bool,
	sessionHash string,
	apiKey *service.APIKey,
	subscription *service.UserSubscription,
	reqLog *zap.Logger,
	streamStarted *bool,
) {
	plan, err := service.CompileOpenAIResponsesImagePlan(body)
	if err != nil {
		h.handleStreamingAwareError(c, http.StatusBadRequest, "invalid_request_error", err.Error(), false)
		return
	}
	if err := h.gatewayService.HydrateOpenAIResponsesImagePlan(c.Request.Context(), openAIForcedImageGroupID(apiKey), plan); err != nil {
		h.handleStreamingAwareError(c, http.StatusBadRequest, "invalid_request_error", err.Error(), false)
		return
	}
	if err := h.gatewayService.PrepareOpenAIResponsesImagePlan(c.Request.Context(), c, plan); err != nil {
		h.handleStreamingAwareError(c, http.StatusBadRequest, "invalid_request_error", err.Error(), false)
		return
	}
	input := openAIForcedImageRunInput{
		apiKey:        apiKey,
		subscription:  subscription,
		plan:          plan,
		requestModel:  requestModel,
		stream:        stream,
		sessionHash:   sessionHash,
		requestHash:   service.HashUsageRequestPayload(body),
		reqLog:        reqLog,
		requestCtx:    c.Request.Context(),
		userAgent:     c.GetHeader("User-Agent"),
		clientIP:      ip.GetClientIP(c),
		inbound:       GetInboundEndpoint(c),
		quotaPlatform: service.QuotaPlatform(c.Request.Context(), apiKey),
	}

	if !stream {
		stopKeepalive := service.StartOpenAIImagesJSONKeepalive(c, h.openAIImagesJSONKeepaliveInterval())
		response, runErr := h.executeForcedOpenAIImageResponses(c.Request.Context(), c, input, plan.N, nil)
		stopKeepalive()
		status := http.StatusOK
		if runErr != nil {
			status = openAIForcedImageHTTPErrorStatus(runErr)
		}
		c.JSON(status, response)
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("X-Accel-Buffering", "no")
	if streamStarted != nil {
		*streamStarted = true
	}
	stopKeepalive := func() {}
	emit := func(event map[string]any) error {
		stopKeepalive()
		if err := writeOpenAIForcedImageHTTPEvent(c, event); err != nil {
			return err
		}
		stopKeepalive = service.StartOpenAIResponsesImageSSEKeepalive(c, h.openAIImagesStreamKeepaliveInterval())
		return nil
	}
	_, _ = h.executeForcedOpenAIImageResponses(c.Request.Context(), c, input, plan.N, emit)
	stopKeepalive()
}

func openAIForcedImageHTTPErrorStatus(err error) int {
	var upstreamErr *service.OpenAIImagesUpstreamError
	if errors.As(err, &upstreamErr) && upstreamErr.StatusCode >= 400 && upstreamErr.StatusCode < 500 {
		return upstreamErr.StatusCode
	}
	return http.StatusBadGateway
}

func (h *OpenAIGatewayHandler) handleForcedOpenAIImageResponsesWebSocket(
	ctx context.Context,
	c *gin.Context,
	wsConn *coderws.Conn,
	firstMessage []byte,
	requestModel string,
	sessionHash string,
	apiKey *service.APIKey,
	subject middleware2.AuthSubject,
	subscription *service.UserSubscription,
	reqLog *zap.Logger,
	releaseTurnSlots func(),
	ensureUserSlotHeld func() bool,
) {
	payload := firstMessage
	turn := 1
	lastResponseID := ""
	for {
		if turn > 1 && !ensureUserSlotHeld() {
			return
		}
		model := strings.TrimSpace(requestModel)
		if value, ok := openAIForcedImageJSONString(payload, "model"); ok && value != "" {
			model = value
		}
		if model == "" {
			_ = writeOpenAIForcedImageWSError(ctx, wsConn, http.StatusBadRequest, "model is required")
			releaseTurnSlots()
			return
		}
		if turn > 1 {
			if decision := h.checkSecurityAuditStage(c, reqLog, apiKey, subject, service.ContentModerationProtocolOpenAIResponses, model, payload, "subsequent_turn"); decision != nil && !decision.AllowNextStage {
				writeSecurityAuditWSError(ctx, wsConn, decision)
				releaseTurnSlots()
				return
			}
			if err := h.billingCacheService.CheckBillingEligibility(ctx, apiKey.User, apiKey, apiKey.Group, subscription, service.QuotaPlatform(c.Request.Context(), apiKey)); err != nil {
				_ = writeOpenAIForcedImageWSError(ctx, wsConn, http.StatusForbidden, "billing check failed")
				releaseTurnSlots()
				return
			}
		}

		if turn > 1 && lastResponseID != "" && strings.TrimSpace(gjson.GetBytes(payload, "previous_response_id").String()) == "" {
			payloadWithPrevious, setErr := sjson.SetBytes(payload, "previous_response_id", lastResponseID)
			if setErr != nil {
				_ = writeOpenAIForcedImageWSError(ctx, wsConn, http.StatusBadRequest, "failed to continue previous image response")
				releaseTurnSlots()
				return
			}
			payload = payloadWithPrevious
		}
		plan, err := service.CompileOpenAIResponsesImagePlan(payload)
		if err != nil {
			_ = writeOpenAIForcedImageWSError(ctx, wsConn, http.StatusBadRequest, err.Error())
			releaseTurnSlots()
			return
		}
		if err := h.gatewayService.HydrateOpenAIResponsesImagePlan(ctx, openAIForcedImageGroupID(apiKey), plan); err != nil {
			_ = writeOpenAIForcedImageWSError(ctx, wsConn, http.StatusBadRequest, err.Error())
			releaseTurnSlots()
			return
		}
		if err := h.gatewayService.PrepareOpenAIResponsesImagePlan(ctx, c, plan); err != nil {
			_ = writeOpenAIForcedImageWSError(ctx, wsConn, http.StatusBadRequest, err.Error())
			releaseTurnSlots()
			return
		}
		stopPing := startOpenAIForcedImageWSPing(ctx, wsConn, h.openAIImagesStreamKeepaliveInterval())
		imageRelease, acquired := h.acquireForcedOpenAIImageWSSlot(ctx)
		if !acquired {
			stopPing()
			_ = writeOpenAIForcedImageWSError(ctx, wsConn, http.StatusTooManyRequests, "Image generation concurrency limit exceeded, please retry later")
			releaseTurnSlots()
			return
		}
		input := openAIForcedImageRunInput{
			apiKey:        apiKey,
			subscription:  subscription,
			plan:          plan,
			requestModel:  model,
			stream:        true,
			sessionHash:   fmt.Sprintf("%s:turn:%d", sessionHash, turn),
			requestHash:   service.HashUsageRequestPayload(payload),
			reqLog:        reqLog,
			requestCtx:    c.Request.Context(),
			userAgent:     c.GetHeader("User-Agent"),
			clientIP:      ip.GetClientIP(c),
			inbound:       GetInboundEndpoint(c),
			quotaPlatform: service.QuotaPlatform(c.Request.Context(), apiKey),
		}
		emit := func(event map[string]any) error {
			encoded, err := json.Marshal(event)
			if err != nil {
				return err
			}
			return wsConn.Write(ctx, coderws.MessageText, encoded)
		}
		response, _ := h.executeForcedOpenAIImageResponses(ctx, c, input, plan.N, emit)
		if strings.TrimSpace(fmt.Sprint(response["status"])) == "completed" {
			lastResponseID = strings.TrimSpace(fmt.Sprint(response["id"]))
		}
		stopPing()
		if imageRelease != nil {
			imageRelease()
		}
		releaseTurnSlots()

		messageType, nextPayload, readErr := wsConn.Read(ctx)
		if readErr != nil {
			return
		}
		if messageType != coderws.MessageText && messageType != coderws.MessageBinary {
			_ = writeOpenAIForcedImageWSError(ctx, wsConn, http.StatusBadRequest, "unsupported websocket message type")
			return
		}
		var envelope map[string]any
		if err := json.Unmarshal(nextPayload, &envelope); err != nil {
			_ = writeOpenAIForcedImageWSError(ctx, wsConn, http.StatusBadRequest, "invalid JSON payload")
			return
		}
		if eventType, _ := envelope["type"].(string); eventType != "response.create" {
			_ = writeOpenAIForcedImageWSError(ctx, wsConn, http.StatusBadRequest, "expected response.create message")
			return
		}
		payload = nextPayload
		turn++
	}
}

func startOpenAIForcedImageWSPing(ctx context.Context, wsConn *coderws.Conn, interval time.Duration) func() {
	if wsConn == nil || interval <= 0 {
		return func() {}
	}
	pingCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-pingCtx.Done():
				return
			case <-ticker.C:
				onePingCtx, onePingCancel := context.WithTimeout(pingCtx, 5*time.Second)
				_ = wsConn.Ping(onePingCtx)
				onePingCancel()
			}
		}
	}()
	return func() {
		cancel()
		<-done
	}
}

func openAIForcedImageJSONString(payload []byte, key string) (string, bool) {
	var envelope map[string]any
	if json.Unmarshal(payload, &envelope) != nil {
		return "", false
	}
	value, ok := envelope[key].(string)
	return strings.TrimSpace(value), ok
}

func (h *OpenAIGatewayHandler) acquireForcedOpenAIImageWSSlot(ctx context.Context) (func(), bool) {
	if h == nil || h.cfg == nil || h.imageLimiter == nil {
		return nil, true
	}
	settings := h.cfg.Gateway.ImageConcurrency
	return h.imageLimiter.Acquire(
		ctx,
		settings.Enabled,
		settings.MaxConcurrentRequests,
		strings.TrimSpace(settings.OverflowMode) == "wait",
		time.Duration(settings.WaitTimeoutSeconds)*time.Second,
		settings.MaxWaitingRequests,
	)
}

func writeOpenAIForcedImageWSError(ctx context.Context, wsConn *coderws.Conn, status int, message string) error {
	payload, err := json.Marshal(map[string]any{
		"type":   "error",
		"status": status,
		"error": map[string]any{
			"type":    "invalid_request_error",
			"message": message,
		},
	})
	if err != nil {
		return err
	}
	return wsConn.Write(ctx, coderws.MessageText, payload)
}
