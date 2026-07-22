package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

const openAIForcedImageChildConcurrency = 4

var openAIForcedImageGlobalChildSlots = make(chan struct{}, openAIForcedImageChildConcurrency)

func runOpenAIForcedImageChildInGlobalSlot(
	ctx context.Context,
	index int,
	run func() openAIForcedImageChildResult,
) (result openAIForcedImageChildResult) {
	select {
	case openAIForcedImageGlobalChildSlots <- struct{}{}:
		defer func() { <-openAIForcedImageGlobalChildSlots }()
	case <-ctx.Done():
		return openAIForcedImageChildResult{index: index, err: ctx.Err()}
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			result = openAIForcedImageChildResult{
				index: index,
				err:   fmt.Errorf("image child panicked: %v", recovered),
			}
		}
	}()
	return run()
}

type openAIForcedImageChildEvent struct {
	index int
	event map[string]any
}

type openAIForcedImageChildResult struct {
	index   int
	output  map[string]any
	usage   map[string]any
	err     error
	account *service.Account
	result  *service.OpenAIForwardResult
}

type openAIForcedImageRunInput struct {
	apiKey        *service.APIKey
	subscription  *service.UserSubscription
	plan          *service.OpenAIResponsesImagePlan
	requestModel  string
	stream        bool
	sessionHash   string
	requestHash   string
	reqLog        *zap.Logger
	requestCtx    context.Context
	userAgent     string
	clientIP      string
	inbound       string
	quotaPlatform string
}

type openAIForcedImageChildWriter struct {
	gin.ResponseWriter
	mu             sync.Mutex
	header         http.Header
	status         int
	size           int
	stream         bool
	pending        []byte
	body           bytes.Buffer
	terminalOutput map[string]any
	terminalUsage  map[string]any
	sawOutputItem  bool
	onEvent        func(map[string]any)
}

func newOpenAIForcedImageChildWriter(base gin.ResponseWriter, stream bool, onEvent func(map[string]any)) *openAIForcedImageChildWriter {
	return &openAIForcedImageChildWriter{
		ResponseWriter: base,
		header:         make(http.Header),
		status:         http.StatusOK,
		stream:         stream,
		onEvent:        onEvent,
	}
}

func (w *openAIForcedImageChildWriter) Header() http.Header { return w.header }

func (w *openAIForcedImageChildWriter) WriteHeader(code int) {
	w.mu.Lock()
	if code > 0 {
		w.status = code
	}
	w.mu.Unlock()
}

func (w *openAIForcedImageChildWriter) WriteHeaderNow() {}

func (w *openAIForcedImageChildWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	w.size += len(data)
	if !w.stream {
		_, _ = w.body.Write(data)
		w.mu.Unlock()
		return len(data), nil
	}
	w.pending = append(w.pending, data...)
	events := w.takeSSEEventsLocked()
	w.mu.Unlock()
	for _, event := range events {
		w.captureEvent(event)
		if w.onEvent != nil {
			w.onEvent(event)
		}
	}
	return len(data), nil
}

func (w *openAIForcedImageChildWriter) WriteString(value string) (int, error) {
	return w.Write([]byte(value))
}

func (w *openAIForcedImageChildWriter) Status() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.status
}

func (w *openAIForcedImageChildWriter) Size() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.size == 0 {
		return -1
	}
	return w.size
}

func (w *openAIForcedImageChildWriter) Written() bool { return w.Size() >= 0 }
func (w *openAIForcedImageChildWriter) Flush()        {}

func (w *openAIForcedImageChildWriter) takeSSEEventsLocked() []map[string]any {
	events := make([]map[string]any, 0, 1)
	for {
		index := bytes.Index(w.pending, []byte("\n\n"))
		separatorSize := 2
		if index < 0 {
			index = bytes.Index(w.pending, []byte("\r\n\r\n"))
			separatorSize = 4
		}
		if index < 0 {
			break
		}
		block := append([]byte(nil), w.pending[:index]...)
		w.pending = append(w.pending[:0], w.pending[index+separatorSize:]...)
		for _, line := range bytes.Split(block, []byte("\n")) {
			line = bytes.TrimSpace(line)
			if !bytes.HasPrefix(line, []byte("data:")) {
				continue
			}
			payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data:")))
			if len(payload) == 0 || bytes.Equal(payload, []byte("[DONE]")) {
				continue
			}
			var event map[string]any
			decoder := json.NewDecoder(bytes.NewReader(payload))
			decoder.UseNumber()
			if decoder.Decode(&event) == nil && event != nil {
				events = append(events, event)
			}
		}
	}
	return events
}

func (w *openAIForcedImageChildWriter) captureEvent(event map[string]any) {
	eventType, _ := event["type"].(string)
	if eventType == "response.output_item.added" || eventType == "response.output_item.done" {
		w.mu.Lock()
		w.sawOutputItem = true
		w.mu.Unlock()
	}
	if eventType != "response.completed" {
		return
	}
	response, _ := event["response"].(map[string]any)
	if response == nil {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if output, ok := response["output"].([]any); ok && len(output) > 0 {
		w.terminalOutput, _ = output[0].(map[string]any)
	}
	w.terminalUsage, _ = response["usage"].(map[string]any)
}

func (w *openAIForcedImageChildWriter) SawOutputItem() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.sawOutputItem
}

func (w *openAIForcedImageChildWriter) response() (map[string]any, map[string]any, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stream {
		if w.terminalOutput == nil {
			return nil, nil, errors.New("image child stream ended without a completed output item")
		}
		return cloneOpenAIForcedImageMap(w.terminalOutput), cloneOpenAIForcedImageMap(w.terminalUsage), nil
	}
	body := bytes.TrimSpace(w.body.Bytes())
	var response map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return nil, nil, fmt.Errorf("decode image child response: %w", err)
	}
	if responseError, ok := response["error"].(map[string]any); ok && responseError != nil {
		return nil, nil, fmt.Errorf("image child failed: %v", responseError["message"])
	}
	output, _ := response["output"].([]any)
	if len(output) == 0 {
		return nil, nil, errors.New("image child response did not contain output")
	}
	item, _ := output[0].(map[string]any)
	if item == nil {
		return nil, nil, errors.New("image child output is invalid")
	}
	usage, _ := response["usage"].(map[string]any)
	return cloneOpenAIForcedImageMap(item), cloneOpenAIForcedImageMap(usage), nil
}

func cloneOpenAIForcedImageMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	cloned := make(map[string]any, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func newOpenAIForcedImageResponseID() string {
	return "resp_" + strings.ReplaceAll(uuid.NewString(), "-", "")
}
