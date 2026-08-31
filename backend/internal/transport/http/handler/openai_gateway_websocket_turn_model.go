package handler

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	coderws "github.com/coder/websocket"
)

type openAIWSTurnChannelMappingSnapshot struct {
	turn           int
	requestedModel string
	mapping        service.ChannelMappingResult
}

type openAIWSTurnChannelMappingState struct {
	mu       sync.RWMutex
	snapshot openAIWSTurnChannelMappingSnapshot
	set      bool
}

func (s *openAIWSTurnChannelMappingState) Store(
	turn int,
	requestedModel string,
	mapping service.ChannelMappingResult,
) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.snapshot = openAIWSTurnChannelMappingSnapshot{
		turn:           turn,
		requestedModel: strings.TrimSpace(requestedModel),
		mapping:        mapping,
	}
	s.set = true
	s.mu.Unlock()
}

func (s *openAIWSTurnChannelMappingState) Load(turn int) (openAIWSTurnChannelMappingSnapshot, bool) {
	if s == nil {
		return openAIWSTurnChannelMappingSnapshot{}, false
	}
	s.mu.RLock()
	snapshot, set := s.snapshot, s.set
	s.mu.RUnlock()
	if !set || snapshot.turn != turn {
		return openAIWSTurnChannelMappingSnapshot{}, false
	}
	return snapshot, true
}

func (s *openAIWSTurnChannelMappingState) Latest() (openAIWSTurnChannelMappingSnapshot, bool) {
	if s == nil {
		return openAIWSTurnChannelMappingSnapshot{}, false
	}
	s.mu.RLock()
	snapshot, set := s.snapshot, s.set
	s.mu.RUnlock()
	if !set {
		return openAIWSTurnChannelMappingSnapshot{}, false
	}
	return snapshot, true
}

var errOpenAIWSUnsupportedModelSwitch = errors.New("selected account does not support websocket model switch")

func newOpenAIWSUnsupportedModelSwitchError(model string) error {
	cause := fmt.Errorf("%w: model %q", errOpenAIWSUnsupportedModelSwitch, strings.TrimSpace(model))
	return service.NewOpenAIWSClientCloseError(coderws.StatusPolicyViolation, "model switch requires reconnect", cause)
}

func shouldReportOpenAIWSProxyAccountFailure(err error) bool {
	return err != nil &&
		!errors.Is(err, errOpenAIWSUnsupportedModelSwitch) &&
		!openAIWSIngressEndedByClient(err)
}

func openAIWSIngressEndedByClient(err error) bool {
	if err == nil {
		return true
	}
	var closeErr *service.OpenAIWSClientCloseError
	if errors.As(err, &closeErr) && closeErr.StatusCode() == coderws.StatusNormalClosure {
		return true
	}
	if coderws.CloseStatus(err) == coderws.StatusNormalClosure {
		return true
	}
	return errors.Is(err, context.Canceled)
}

func openAIWSTurnBillingModel(
	result *service.OpenAIForwardResult,
	mapping service.ChannelMappingResult,
	requestedModel string,
	upstreamModel string,
) string {
	billingModel := ""
	if result != nil {
		billingModel = strings.TrimSpace(result.BillingModel)
	}
	if billingModel == "" {
		billingModel = strings.TrimSpace(upstreamModel)
	}
	if billingModel == "" {
		billingModel = strings.TrimSpace(requestedModel)
	}

	requestedModel = strings.TrimSpace(requestedModel)
	switch mapping.BillingModelSource {
	case service.BillingModelSourceRequested:
		if requestedModel != "" {
			billingModel = requestedModel
		}
	case service.BillingModelSourceChannelMapped:
		mappedModel := strings.TrimSpace(mapping.MappedModel)
		if mappedModel != "" && mappedModel != requestedModel {
			billingModel = mappedModel
		}
	}
	return billingModel
}
