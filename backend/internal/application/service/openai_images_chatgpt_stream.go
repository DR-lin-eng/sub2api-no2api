package service

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/chatgptimages"
)

func (s *chatGPTImageSession) requestJSONBody(value any) ([]byte, error) { return json.Marshal(value) }

func (s *chatGPTImageSession) generate(ctx context.Context, payload any, d *chatgptimages.Decoder) error {
	if err := s.prepareIntegrity(ctx); err != nil {
		return err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := s.request(ctx, http.MethodPost, "/f/conversation", body, true)
	if err != nil {
		return err
	}
	err = s.consume(ctx, resp, d)
	if (d.Done || d.MessageStreamComplete) && len(d.Images) > 0 {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	// A handoff resumes this generation. It never creates another conversation.
	if d.Handoff.Topic != "" {
		if topicErr := s.consumeTopic(ctx, d); topicErr == nil && len(d.Images) > 0 {
			return nil
		}
	}
	if d.ConversationID == "" {
		if err != nil {
			return err
		}
		return fmt.Errorf("ChatGPT image stream ended without a conversation identity")
	}
	if err != nil || d.Handoff.ResumeURL != "" {
		path := "/f/conversation/resume"
		if d.Handoff.ResumeURL != "" {
			path = d.Handoff.ResumeURL
		}
		resumeBody, _ := json.Marshal(map[string]any{"conversation_id": d.ConversationID, "offset": d.Offset})
		for attempt := 0; attempt < 3; attempt++ {
			response, resumeErr := s.request(ctx, http.MethodPost, path, resumeBody, true)
			if resumeErr == nil {
				resumeErr = s.consume(ctx, response, d)
			}
			if resumeErr == nil && len(d.Images) > 0 {
				return nil
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if err := chatGPTImageWait(ctx, time.Duration(attempt+1)*300*time.Millisecond); err != nil {
				return err
			}
		}
	}
	// App image generation can continue asynchronously after SSE completion.
	// Poll only after interruption, a handoff or an explicit async placeholder.
	return s.recoverHistory(ctx, d)
}

func (s *chatGPTImageSession) consume(ctx context.Context, resp *http.Response, d *chatgptimages.Decoder) error {
	readCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer func() { _ = resp.Body.Close() }()
	if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
		body, err := readChatGPTImageBody(resp.Body, chatgptimages.MaxPayloadBytes)
		if err != nil {
			return err
		}
		return d.Feed(body)
	}
	type frame struct {
		line string
		err  error
	}
	frames := make(chan frame, 1)
	go func() {
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 32<<10), chatgptimages.MaxPayloadBytes)
		for scanner.Scan() {
			select {
			case frames <- frame{line: scanner.Text()}:
			case <-readCtx.Done():
				return
			}
		}
		select {
		case frames <- frame{err: func() error {
			if err := scanner.Err(); err != nil {
				return err
			}
			return io.EOF
		}()}:
		case <-readCtx.Done():
		}
	}()
	idle := time.NewTimer(60 * time.Second)
	defer idle.Stop()
	var data strings.Builder
	flush := func() error {
		if data.Len() == 0 {
			return nil
		}
		raw := data.String()
		data.Reset()
		return d.Feed([]byte(raw))
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-idle.C:
			return fmt.Errorf("ChatGPT image SSE idle timeout")
		case f := <-frames:
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(60 * time.Second)
			if f.err != nil {
				if err := flush(); err != nil {
					return err
				}
				if f.err == io.EOF && (d.Done || d.MessageStreamComplete) {
					return nil
				}
				return f.err
			}
			line := strings.TrimSuffix(f.line, "\r")
			if line == "" {
				if err := flush(); err != nil {
					return err
				}
				if d.Done {
					return nil
				}
				continue
			}
			if strings.HasPrefix(line, "data:") {
				if data.Len() > 0 {
					_ = data.WriteByte('\n')
				}
				if data.Len()+len(line) > chatgptimages.MaxPayloadBytes {
					return fmt.Errorf("ChatGPT image SSE event exceeds limit")
				}
				_, _ = data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
			}
		}
	}
}

func (s *chatGPTImageSession) recoverHistory(ctx context.Context, d *chatgptimages.Decoder) error {
	if !validChatGPTImageFileID(d.ConversationID) {
		return fmt.Errorf("invalid ChatGPT image conversation identity")
	}
	interval := time.Duration(d.PollIntervalMS) * time.Millisecond
	if interval < 5*time.Second {
		interval = 5 * time.Second
	}
	if d.PollIntervalMS <= 0 {
		interval = 10 * time.Second
	}
	interval = min(interval, time.Minute)
	freshness := 15 * time.Minute
	if d.FreshnessMinutes > 0 {
		freshness = min(time.Duration(d.FreshnessMinutes)*time.Minute, freshness)
	}
	deadline := time.Now().Add(freshness)
	for attempt := 0; attempt < 180 && time.Now().Before(deadline); attempt++ {
		body, err := s.json(ctx, http.MethodGet, "/conversation/"+urlPathEscapeChatGPTImage(d.ConversationID), nil)
		if err != nil {
			return err
		}
		if err = d.Feed(body); err != nil {
			return err
		}
		if len(d.Images) > 0 {
			return nil
		}
		if !d.Async && d.Handoff.Topic == "" && d.Handoff.ResumeURL == "" {
			return fmt.Errorf("ChatGPT completed without image output")
		}
		if err := chatGPTImageWait(ctx, min(interval, time.Until(deadline))); err != nil {
			return err
		}
	}
	return fmt.Errorf("ChatGPT image recovery expired")
}
