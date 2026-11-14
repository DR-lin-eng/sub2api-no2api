package service

import (
	"context"
	"errors"
	"net/http"

	"github.com/Wei-Shaw/sub2api/internal/modules/chatgptimages"
)

func (s *chatGPTImageSession) prepareIntegrity(ctx context.Context) error {
	// Clear the previous turn before doing any work. Every prepare gets a new
	// environment UUID/key and consumes only its own seed/difficulty/dx.
	s.integrity = nil
	integrity, err := chatgptimages.NewIntegrity(s.headers.Get("User-Agent"), s.headers.Get("OAI-Language"), s.headers.Get("oai-did"))
	if err != nil {
		return err
	}
	body, err := s.json(ctx, http.MethodPost, "/sentinel/chat-requirements/prepare", map[string]any{"p": integrity.RequirementsKey()})
	if err != nil {
		return err
	}
	headers, err := integrity.Headers(ctx, body)
	if errors.Is(err, chatgptimages.ErrForceLogin) {
		return &OpenAIImagesUpstreamError{StatusCode: 401, ErrorType: "authentication_error", Code: "force_login", Message: "ChatGPT requires the selected account to sign in again."}
	}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		if errors.Is(err, chatgptimages.ErrRequirements) {
			return err
		}
		return &OpenAIImagesUpstreamError{StatusCode: 403, ErrorType: "permission_error", Code: "chatgpt_verification_required", Message: "ChatGPT App image verification could not be completed within the protocol limits. Sign in with the App or select legacy images in Gateway settings."}
	}
	s.integrity = headers
	return nil
}
