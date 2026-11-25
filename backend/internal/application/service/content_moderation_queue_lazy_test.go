package service

import (
	"testing"
)

func TestContentModerationQueueAllocatesOnDemandAndGrows(t *testing.T) {
	cfg := defaultContentModerationConfig()
	cfg.QueueSize = 128
	svc := NewContentModerationService(nil, nil, nil, nil, nil, nil, nil)
	if svc.asyncQueueOf() != nil {
		t.Fatal("new moderation service should not allocate the maximum queue")
	}

	for i := 0; i < contentModerationInitialQueueCapacity; i++ {
		svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{}, "task")
	}
	if got := cap(svc.asyncQueueOf()); got != contentModerationInitialQueueCapacity {
		t.Fatalf("initial queue capacity = %d, want %d", got, contentModerationInitialQueueCapacity)
	}
	svc.enqueueAsync(ContentModerationCheckInput{}, cfg, ContentModerationInput{}, "grow")
	if got := cap(svc.asyncQueueOf()); got != 128 {
		t.Fatalf("grown queue capacity = %d, want 128", got)
	}
	if got := svc.asyncEnqueued.Load(); got != contentModerationInitialQueueCapacity+1 {
		t.Fatalf("enqueued tasks = %d, want %d", got, contentModerationInitialQueueCapacity+1)
	}
	svc.Stop()
}
