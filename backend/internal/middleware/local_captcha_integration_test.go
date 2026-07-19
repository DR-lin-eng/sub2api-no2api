//go:build integration

package middleware

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLocalCaptchaCapsConcurrentChallengesInRedis(t *testing.T) {
	ctx := context.Background()
	rdb := startRedis(t, ctx)
	captcha := NewLocalCaptcha(rdb)
	captcha.maxActive = 64

	const total = 512
	errors := make(chan error, total)
	var wg sync.WaitGroup
	for index := 0; index < total; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			captchaID := fmt.Sprintf("%032x", index)
			created, err := captcha.issue(ctx, captchaID, "digest")
			if err != nil {
				errors <- err
				return
			}
			if !created {
				errors <- fmt.Errorf("challenge %s was not created", captchaID)
			}
		}(index)
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		require.NoError(t, err)
	}

	active, err := rdb.ZCard(ctx, "auth_captcha:active").Result()
	require.NoError(t, err)
	require.Equal(t, int64(64), active)

	keys, err := rdb.Keys(ctx, "auth_captcha:*").Result()
	require.NoError(t, err)
	challengeKeys := 0
	for _, key := range keys {
		if strings.HasPrefix(key, "auth_captcha:") && key != "auth_captcha:active" && key != "auth_captcha:sequence" {
			challengeKeys++
		}
	}
	require.Equal(t, 64, challengeKeys)
}
