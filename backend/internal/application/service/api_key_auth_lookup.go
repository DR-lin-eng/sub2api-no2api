package service

import (
	"context"
	"fmt"
	"time"
	"unicode/utf8"
)

// GetByKey 根据Key字符串获取API Key（用于认证）
func (s *APIKeyService) GetByKey(ctx context.Context, key string) (*APIKey, error) {
	// 非 UTF-8 的 key 不可能存在（生成和自定义的 key 都是 ASCII），且 Postgres 会以
	// invalid byte sequence 拒绝该参数；直接按不存在处理，避免返回 500。
	if len(key) == 0 || len(key) > MaxAPIKeyCredentialBytes || !utf8.ValidString(key) {
		return nil, ErrAPIKeyNotFound
	}
	digest := authCacheDigest(key)
	if entry, ok := s.getAuthHotCacheEntry(digest, time.Now()); ok {
		if apiKey, used, err := s.applyAuthHotCacheEntry(key, entry); used {
			if err != nil {
				return nil, fmt.Errorf("get api key: %w", err)
			}
			return apiKey, nil
		}
	}
	cacheKey := authCacheKeyFromDigest(digest)

	entry, cached := s.getLocalAuthCacheEntry(cacheKey)
	if !cached {
		if s.authBloomRejects(ctx, digest) {
			return nil, fmt.Errorf("get api key: %w", ErrAPIKeyNotFound)
		}
		entry, cached = s.getAuthCacheEntry(ctx, cacheKey)
	}
	if cached {
		s.setAuthHotCacheEntry(digest, key, entry)
		if apiKey, used, err := s.applyAuthCacheEntry(key, entry); used {
			if err != nil {
				return nil, fmt.Errorf("get api key: %w", err)
			}
			return apiKey, nil
		}
	}

	if s.authCfg.singleflight {
		value, err, _ := s.authGroup.Do(cacheKey, func() (any, error) {
			return s.loadAuthCacheEntry(ctx, key, cacheKey)
		})
		if err != nil {
			return nil, err
		}
		entry, _ := value.(*APIKeyAuthCacheEntry)
		s.setAuthHotCacheEntry(digest, key, entry)
		if apiKey, used, err := s.applyAuthCacheEntry(key, entry); used {
			if err != nil {
				return nil, fmt.Errorf("get api key: %w", err)
			}
			return apiKey, nil
		}
	} else {
		entry, err := s.loadAuthCacheEntry(ctx, key, cacheKey)
		if err != nil {
			return nil, err
		}
		s.setAuthHotCacheEntry(digest, key, entry)
		if apiKey, used, err := s.applyAuthCacheEntry(key, entry); used {
			if err != nil {
				return nil, fmt.Errorf("get api key: %w", err)
			}
			return apiKey, nil
		}
	}

	apiKey, err := s.lookupAPIKeyForAuth(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("get api key: %w", err)
	}
	apiKey.Key = key
	s.compileAPIKeyIPRules(apiKey)
	return apiKey, nil
}
