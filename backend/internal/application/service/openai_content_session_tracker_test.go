package service

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func BenchmarkOpenAIContentSessionTracker(b *testing.B) {
	b.Run("same_session_serial", func(b *testing.B) {
		tracker := &openAIContentSessionTracker{}
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			tracker.begin(42, "0123456789abcdef")
			tracker.release(42, "0123456789abcdef")
		}
	})

	b.Run("same_session_parallel", func(b *testing.B) {
		tracker := &openAIContentSessionTracker{}
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			for pb.Next() {
				tracker.begin(42, "0123456789abcdef")
				tracker.release(42, "0123456789abcdef")
			}
		})
	})

	b.Run("distinct_sessions_parallel", func(b *testing.B) {
		tracker := &openAIContentSessionTracker{}
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				hash := fmt.Sprintf("%016x", i)
				tracker.begin(42, hash)
				tracker.release(42, hash)
				i++
			}
		})
	})
}

func TestOpenAIContentSessionTrackerBoundsBurstSelections(t *testing.T) {
	tracker := &openAIContentSessionTracker{}
	concurrent, overflow := tracker.begin(42, "same-session")
	if concurrent || overflow {
		t.Fatalf("first request must stay on the normal path: concurrent=%v overflow=%v", concurrent, overflow)
	}
	for i := 0; i < openAIContentSessionBurstBalanceMaxSelections; i++ {
		concurrent, overflow = tracker.begin(42, "same-session")
		if !concurrent || overflow {
			t.Fatalf("selection %d must be balanced: concurrent=%v overflow=%v", i, concurrent, overflow)
		}
	}
	concurrent, overflow = tracker.begin(42, "same-session")
	if concurrent || !overflow {
		t.Fatalf("request beyond burst bound must overflow: concurrent=%v overflow=%v", concurrent, overflow)
	}

	for i := 0; i < openAIContentSessionBurstBalanceMaxSelections+2; i++ {
		tracker.release(42, "same-session")
	}
	concurrent, overflow = tracker.begin(42, "same-session")
	if concurrent || overflow {
		t.Fatalf("released session state was not reset: concurrent=%v overflow=%v", concurrent, overflow)
	}
	tracker.release(42, "same-session")
}

func TestOpenAIContentSessionTrackerCyclesBoundedCandidates(t *testing.T) {
	tracker := &openAIContentSessionTracker{}
	const (
		groupID     = int64(42)
		sessionHash = "same-session"
	)
	tracker.begin(groupID, sessionHash)
	tracker.recordCandidate(groupID, sessionHash, 101)
	tracker.recordCandidate(groupID, sessionHash, 102)
	tracker.recordCandidate(groupID, sessionHash, 103)
	tracker.recordCandidate(groupID, sessionHash, 102) // de-duplicate

	candidates, count := tracker.nextCandidates(groupID, sessionHash)
	if count != 2 || candidates != [openAIContentSessionBurstCandidateAttempts]int64{101, 102} {
		t.Fatalf("first candidate window = %v (count %d), want [101 102]", candidates, count)
	}
	candidates, count = tracker.nextCandidates(groupID, sessionHash)
	if count != 2 || candidates != [openAIContentSessionBurstCandidateAttempts]int64{102, 103} {
		t.Fatalf("second candidate window = %v (count %d), want [102 103]", candidates, count)
	}

	tracker.dropCandidate(groupID, sessionHash, 102)
	candidates, count = tracker.nextCandidates(groupID, sessionHash)
	if count != 2 || candidates[0] == 102 || candidates[1] == 102 {
		t.Fatalf("dropped candidate was returned: %v (count %d)", candidates, count)
	}
	tracker.release(groupID, sessionHash)
}

func TestOpenAIContentSessionTrackerBoundsParallelLoadBalance(t *testing.T) {
	tracker := &openAIContentSessionTracker{}
	releases := make([]func(), 0, openAIContentSessionLoadBalanceMaxParallel)
	for i := 0; i < openAIContentSessionLoadBalanceMaxParallel; i++ {
		release, err := tracker.acquireLoadBalance(context.Background())
		if err != nil {
			t.Fatalf("acquire %d: %v", i, err)
		}
		releases = append(releases, release)
	}

	blockedCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := tracker.acquireLoadBalance(blockedCtx); err == nil {
		t.Fatal("acquire beyond global bound unexpectedly succeeded")
	}
	releases[0]()
	release, err := tracker.acquireLoadBalance(context.Background())
	if err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
	release()
	for _, release := range releases[1:] {
		release()
	}
}
