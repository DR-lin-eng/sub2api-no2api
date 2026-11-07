package service

import (
	"context"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/chatgptimages"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Configured admin probes follow the same protocol toggle and session adapter
// as gateway requests. The low-level tester without settings retains its legacy
// probe contract, as it cannot advertise a selected gateway protocol.
func (s *AccountTestService) testChatGPTAppImage(c *gin.Context, ctx context.Context, account *Account, model, prompt string) error {
	c.Writer.Header().Set("Content-Type", "text/event-stream")
	c.Writer.Header().Set("Cache-Control", "no-cache")
	c.Writer.Header().Set("X-Accel-Buffering", "no")
	c.Writer.Flush()
	s.sendEvent(c, TestEvent{Type: "test_start", Model: model})
	s.sendEvent(c, TestEvent{Type: "content", Text: "Calling ChatGPT App image generation...\n"})
	gateway := s.oauthGatewayLimiter
	if gateway == nil {
		gateway = &OpenAIGatewayService{cfg: s.cfg, httpUpstream: s.httpUpstream, accountRepo: s.accountRepo, settingService: s.settingService, tlsFPProfileService: s.tlsFPProfileService}
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	session, err := gateway.newChatGPTImageSession(ctx, c, account)
	if err != nil {
		return s.sendErrorAndEnd(c, sanitizeUpstreamErrorMessage(err.Error()))
	}
	session.model = model
	timezone := "UTC"
	if enabled, configured := s.settingService.GetOpenAIRequestTimezoneSettings(ctx); enabled {
		timezone = configured
	}
	payload := chatgptimages.Conversation(prompt, openAIImagesResponsesMainModelValue(), uuid.NewString(), uuid.NewString(), uuid.NewString(), timezone, nil)
	decoder := &chatgptimages.Decoder{}
	if err = session.generate(ctx, payload, decoder); err != nil {
		return s.sendErrorAndEnd(c, sanitizeUpstreamErrorMessage(err.Error()))
	}
	images, err := session.materialize(ctx, decoder)
	if err != nil || len(images) == 0 {
		return s.sendErrorAndEnd(c, "ChatGPT App image test returned no complete image")
	}
	for _, item := range images {
		mimeType := openAIImageOutputMIMEType(item.OutputFormat)
		s.sendEvent(c, TestEvent{Type: "image", ImageURL: fmt.Sprintf("data:%s;base64,%s", mimeType, item.Result), MimeType: mimeType})
	}
	s.sendEvent(c, TestEvent{Type: "test_complete", Success: true})
	return nil
}
