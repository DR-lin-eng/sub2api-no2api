package service

import (
	"context"
	"sync"
	"time"
)

type singleAccountRetryProbeContextKey struct{}

// singleAccountRetryProbe 在首次需要时判定是否进入单账号退避重试模式，同一请求内至多判定一次。
type singleAccountRetryProbe struct {
	once   sync.Once
	check  func(ctx context.Context) bool
	result bool
}

func WithSingleAccountRetryProbe(ctx context.Context, check func(ctx context.Context) bool) context.Context {
	if ctx == nil || check == nil {
		return ctx
	}
	return context.WithValue(ctx, singleAccountRetryProbeContextKey{}, &singleAccountRetryProbe{check: check})
}

func singleAccountRetryProbeResult(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	probe, _ := ctx.Value(singleAccountRetryProbeContextKey{}).(*singleAccountRetryProbe)
	if probe == nil {
		return false
	}
	// 判定结果在整个请求内复用，不能因调用时 ctx 已取消而被记成 false。
	probe.once.Do(func() {
		probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		probe.result = probe.check(probeCtx)
	})
	return probe.result
}

func (s *GatewayService) WithSingleAntigravityAccountGroupProbe(ctx context.Context, groupID *int64) context.Context {
	var probeGroupID *int64
	if groupID != nil {
		id := *groupID
		probeGroupID = &id
	}
	return WithSingleAccountRetryProbe(ctx, func(probeCtx context.Context) bool {
		return s.IsSingleAntigravityAccountGroup(probeCtx, probeGroupID)
	})
}
