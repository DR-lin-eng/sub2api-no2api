package service

import "context"

// InvalidateAuthCacheByKey 清除指定 API Key 的认证缓存
func (s *APIKeyService) InvalidateAuthCacheByKey(ctx context.Context, key string) {
	if key == "" {
		return
	}
	cacheKey := s.authCacheKey(key)
	s.deleteAuthCache(ctx, cacheKey)
}

// InvalidateAuthCacheByUserID 清除用户相关的 API Key 认证缓存
func (s *APIKeyService) InvalidateAuthCacheByUserID(ctx context.Context, userID int64) {
	if userID <= 0 {
		return
	}
	keys, err := s.apiKeyRepo.ListKeysByUserID(ctx, userID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByKeys(ctx, keys)
}

// InvalidateAuthCacheByGroupID 清除分组相关的 API Key 认证缓存
func (s *APIKeyService) InvalidateAuthCacheByGroupID(ctx context.Context, groupID int64) {
	if groupID <= 0 {
		return
	}
	if keys, err := s.apiKeyRepo.ListKeysByGroupID(ctx, groupID); err == nil {
		s.deleteAuthCacheByKeys(ctx, keys)
	}
	// Scheme keys do not store a group_id, so a group change must also refresh
	// every key routed through a scheme that contains this group.
	if s.routingSchemeService == nil {
		return
	}
	schemeIDs, err := s.routingSchemeService.SchemeIDsByGroupID(ctx, groupID)
	if err != nil {
		return
	}
	for _, schemeID := range schemeIDs {
		s.InvalidateAuthCacheByRoutingScheme(ctx, schemeID)
	}
}

// InvalidateAuthCacheByRoutingScheme 清除使用该方案的 API Key 认证缓存。
func (s *APIKeyService) InvalidateAuthCacheByRoutingScheme(ctx context.Context, schemeID int64) {
	if schemeID <= 0 {
		return
	}
	lister, ok := s.apiKeyRepo.(apiKeyRoutingSchemeKeyLister)
	if !ok {
		return
	}
	keys, err := lister.ListKeysByRoutingSchemeID(ctx, schemeID)
	if err != nil {
		return
	}
	s.deleteAuthCacheByKeys(ctx, keys)
}

func (s *APIKeyService) deleteAuthCacheByKeys(ctx context.Context, keys []string) {
	if len(keys) == 0 {
		return
	}
	for _, key := range keys {
		if key == "" {
			continue
		}
		s.deleteAuthCache(ctx, s.authCacheKey(key))
	}
}
