package service

import (
	"context"
	"crypto/sha256"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

// APIKeyAuthBloomCache shares an append-only membership filter between nodes.
// Unknown/unavailable state must return an error, never a definitive negative.
type APIKeyAuthBloomCache interface {
	MayContainAuthKey(context.Context, [sha256.Size]byte) (bool, error)
	RegisterAuthKey(context.Context, string) error
	InitializeAuthKeyBloom(context.Context, func(context.Context, func([][sha256.Size]byte) error) error) error
}

// APIKeyAuthBloomRepository registers credentials before making them visible.
// The scan must include every non-deleted key, including disabled/expired keys.
type APIKeyAuthBloomRepository interface {
	SetAuthKeyBloomCache(APIKeyAuthBloomCache)
	WalkAuthKeyDigests(context.Context, func([][sha256.Size]byte) error) error
}

type apiKeyAuthBloom struct {
	cache     APIKeyAuthBloomCache
	repo      APIKeyAuthBloomRepository
	start     sync.Once
	stop      sync.Once
	cancel    context.CancelFunc
	wg        sync.WaitGroup
	rejected  atomic.Uint64
	fallbacks atomic.Uint64
	rebuilds  atomic.Uint64
	failures  atomic.Uint64
}

type APIKeyAuthBloomHealth struct {
	Enabled   bool   `json:"enabled"`
	Rejected  uint64 `json:"rejected"`
	Fallbacks uint64 `json:"fallbacks"`
	Rebuilds  uint64 `json:"rebuilds"`
	Failures  uint64 `json:"failures"`
}

func (s *APIKeyService) initAuthBloom(cfg *config.Config) {
	if cfg == nil || !cfg.APIKeyAuth.Bloom.Enabled {
		return
	}
	cache, cacheOK := s.cache.(APIKeyAuthBloomCache)
	repo, repoOK := s.apiKeyRepo.(APIKeyAuthBloomRepository)
	if !cacheOK || !repoOK {
		return
	}
	repo.SetAuthKeyBloomCache(cache)
	s.authBloom = &apiKeyAuthBloom{cache: cache, repo: repo}
}

func (s *APIKeyService) authBloomRejects(ctx context.Context, digest [sha256.Size]byte) bool {
	b := s.authBloom
	if b == nil {
		return false
	}
	checkCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	maybe, err := b.cache.MayContainAuthKey(checkCtx, digest)
	if err != nil {
		b.fallbacks.Add(1)
		return false
	}
	if !maybe {
		b.rejected.Add(1)
		return true
	}
	return false
}

func (s *APIKeyService) startAuthBloom(ctx context.Context) {
	b := s.authBloom
	if b == nil {
		return
	}
	b.start.Do(func() {
		workerCtx, cancel := context.WithCancel(ctx)
		b.cancel = cancel
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			timer := time.NewTimer(0)
			defer timer.Stop()
			for {
				select {
				case <-workerCtx.Done():
					return
				case <-timer.C:
				}
				buildCtx, buildCancel := context.WithTimeout(workerCtx, 2*time.Minute)
				err := b.cache.InitializeAuthKeyBloom(buildCtx, b.repo.WalkAuthKeyDigests)
				buildCancel()
				if err != nil {
					b.failures.Add(1)
				} else {
					b.rebuilds.Add(1)
				}
				// At most one scan per node per interval, with a shared build lease.
				timer.Reset(30 * time.Second)
			}
		}()
	})
}

func (s *APIKeyService) stopAuthBloom() {
	if b := s.authBloom; b != nil {
		b.stop.Do(func() {
			if b.cancel != nil {
				b.cancel()
			}
			b.wg.Wait()
		})
	}
}

func (s *APIKeyService) AuthBloomHealth() APIKeyAuthBloomHealth {
	if s == nil || s.authBloom == nil {
		return APIKeyAuthBloomHealth{}
	}
	b := s.authBloom
	return APIKeyAuthBloomHealth{Enabled: true, Rejected: b.rejected.Load(), Fallbacks: b.fallbacks.Load(), Rebuilds: b.rebuilds.Load(), Failures: b.failures.Load()}
}
