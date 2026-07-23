package service

import (
	"fmt"
	"testing"
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
