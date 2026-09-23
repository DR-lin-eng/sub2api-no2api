package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

const (
	oauth2AuthorizationCodePrefix = "oauth2:authorization-code:"
	oauth2AccessTokenPrefix       = "oauth2:access-token:"
)

var oauth2GetAndDeleteScript = redis.NewScript(`
local value = redis.call('GET', KEYS[1])
if value then
  redis.call('DEL', KEYS[1])
end
return value
`)

type oauth2ProviderStore struct {
	rdb *redis.Client
}

func NewOAuth2ProviderStore(rdb *redis.Client) service.OAuth2ProviderStore {
	return &oauth2ProviderStore{rdb: rdb}
}

func (s *oauth2ProviderStore) StoreAuthorizationCode(ctx context.Context, hash string, data *service.OAuth2AuthorizationCodeData, ttl time.Duration) error {
	return s.store(ctx, oauth2AuthorizationCodePrefix+hash, data, ttl)
}

func (s *oauth2ProviderStore) ConsumeAuthorizationCode(ctx context.Context, hash string) (*service.OAuth2AuthorizationCodeData, error) {
	value, err := oauth2GetAndDeleteScript.Run(ctx, s.rdb, []string{oauth2AuthorizationCodePrefix + hash}).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrOAuth2GrantNotFound
		}
		return nil, fmt.Errorf("consume oauth2 authorization code: %w", err)
	}
	encoded, ok := value.(string)
	if !ok || encoded == "" {
		return nil, service.ErrOAuth2GrantNotFound
	}
	var data service.OAuth2AuthorizationCodeData
	if err := json.Unmarshal([]byte(encoded), &data); err != nil {
		return nil, fmt.Errorf("decode oauth2 authorization code: %w", err)
	}
	return &data, nil
}

func (s *oauth2ProviderStore) StoreAccessToken(ctx context.Context, hash string, data *service.OAuth2AccessTokenData, ttl time.Duration) error {
	return s.store(ctx, oauth2AccessTokenPrefix+hash, data, ttl)
}

func (s *oauth2ProviderStore) GetAccessToken(ctx context.Context, hash string) (*service.OAuth2AccessTokenData, error) {
	encoded, err := s.rdb.Get(ctx, oauth2AccessTokenPrefix+hash).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, service.ErrOAuth2GrantNotFound
		}
		return nil, fmt.Errorf("get oauth2 access token: %w", err)
	}
	var data service.OAuth2AccessTokenData
	if err := json.Unmarshal([]byte(encoded), &data); err != nil {
		return nil, fmt.Errorf("decode oauth2 access token: %w", err)
	}
	return &data, nil
}

func (s *oauth2ProviderStore) DeleteAccessToken(ctx context.Context, hash string) error {
	return s.rdb.Del(ctx, oauth2AccessTokenPrefix+hash).Err()
}

func (s *oauth2ProviderStore) store(ctx context.Context, key string, value any, ttl time.Duration) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode oauth2 grant: %w", err)
	}
	if ttl <= 0 {
		return fmt.Errorf("oauth2 grant ttl must be positive")
	}
	return s.rdb.Set(ctx, key, encoded, ttl).Err()
}
