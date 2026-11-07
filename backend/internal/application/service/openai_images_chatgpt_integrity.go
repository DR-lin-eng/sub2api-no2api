package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tidwall/gjson"
)

// The recovered lXEa.jU browser implementation has an explicit error-envelope
// branch when its browser context cannot be read. A Go gateway has no browser
// context. Report that condition honestly; do not fabricate browser telemetry,
// execute a server-supplied browser VM, or invent a successful challenge proof.
func (s *chatGPTImageSession) prepareIntegrity(ctx context.Context) error {
	contextError, _ := json.Marshal("Error: browser context unavailable in the gateway")
	p := "gAAAAACwQ8Lk5FbGpA2NcR9dShT6gYjU7VxZ4D" + base64.StdEncoding.EncodeToString(contextError)
	body, err := s.json(ctx, http.MethodPost, "/sentinel/chat-requirements/prepare", map[string]any{"p": p})
	if err != nil {
		return err
	}
	if !gjson.ValidBytes(body) || !gjson.ParseBytes(body).IsObject() {
		return fmt.Errorf("invalid ChatGPT requirements response")
	}
	if gjson.GetBytes(body, "force_login").Bool() {
		return &OpenAIImagesUpstreamError{StatusCode: 401, ErrorType: "authentication_error", Code: "force_login", Message: "ChatGPT requires the selected account to sign in again."}
	}
	if gjson.GetBytes(body, "turnstile.required").Bool() || gjson.GetBytes(body, "proofofwork.required").Bool() {
		return &OpenAIImagesUpstreamError{StatusCode: 403, ErrorType: "permission_error", Code: "chatgpt_verification_required", Message: "ChatGPT App images require browser verification for this account. Use the App to verify the account or select legacy images in Gateway settings."}
	}
	s.integrity = make(http.Header)
	if token := gjson.GetBytes(body, "token").String(); token != "" {
		s.integrity.Set("OpenAI-Sentinel-Chat-Requirements-Token", token)
	} else if token := gjson.GetBytes(body, "prepare_token").String(); token != "" {
		s.integrity.Set("OpenAI-Sentinel-Chat-Requirements-Prepare-Token", token)
	}
	return nil
}
