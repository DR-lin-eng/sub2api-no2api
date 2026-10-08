package handler

import (
	"context"
	"github.com/Wei-Shaw/sub2api/internal/application/service"
	"github.com/Wei-Shaw/sub2api/internal/shared/ctxkey"
	"sync"
)

// openAIWSTurnAPIKeyLookup 是后续 turn 重取 API Key 认证快照的入口
// （APIKeyService.GetByKey：经 L1/L2 认证缓存，未命中才回源）。
type openAIWSTurnAPIKeyLookup interface {
	GetByKey(ctx context.Context, key string) (*service.APIKey, error)
}

// openAIWSTurnBillingAPIKeys 按 turn 号保存每个 turn 计费用的 API Key 快照。
//
// API Key 认证快照只在建连时取一次。若连接内所有 turn 共用它，分组定价（倍率、
// 高峰、图片定价、利润门参数）会停留在建连时刻：管理员调价后，已打开的长连接
// 在客户端重连前一直按旧价计费、按旧售价过利润门，而 HTTP 请求每次都经认证缓存
// 取快照，缓存失效后即生效。
//
// 因此后续 turn 在 BeforeTurn（与 HTTP 请求进入认证中间件同位）经同一认证缓存
// 重取快照，只采用其中的分组，作为该 turn 利润门准入与用量计费共同的分组；用户、
// Key 限额、订阅仍沿用建连快照。只在同一把 Key、同一分组且平台与订阅类型未变时
// 采用：Key 换组或分组改平台/订阅类型不是调价，该连接按建连分组调度，继续按建连
// 快照计费；重取失败同样保留建连快照，不断连。
//
// 首轮沿用刚经认证中间件取得的建连快照；没有经过 BeforeTurn 的 turn 回退建连
// 快照。只保留当前与上一个 turn：下一 turn 的 BeforeTurn 先于上一 turn 的
// AfterTurn 执行时，上一 turn 仍取到自己的快照。
type openAIWSTurnBillingAPIKeys struct {
	mu   sync.Mutex
	keys map[int]*service.APIKey
}

// begin 在 BeforeTurn 重装利润门前调用：后续 turn 重取计费分组并按 turn 记下，
// 返回换入该分组的上下文供本 turn 的利润门使用。
func (k *openAIWSTurnBillingAPIKeys) begin(ctx context.Context, apiKeyService *service.APIKeyService, turn int, conn *service.APIKey) context.Context {
	turnKey := conn
	if turn > 1 && apiKeyService != nil {
		turnKey = refreshOpenAIWSTurnBillingAPIKey(ctx, apiKeyService, conn)
	}
	k.set(turn, turnKey)
	return withOpenAIWSTurnBillingGroup(ctx, turnKey)
}

func (k *openAIWSTurnBillingAPIKeys) set(turn int, key *service.APIKey) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.keys == nil {
		k.keys = make(map[int]*service.APIKey, 2)
	}
	for t := range k.keys {
		if t < turn-1 {
			delete(k.keys, t)
		}
	}
	k.keys[turn] = key
}

func (k *openAIWSTurnBillingAPIKeys) forTurn(turn int, conn *service.APIKey) *service.APIKey {
	k.mu.Lock()
	defer k.mu.Unlock()
	if key := k.keys[turn]; key != nil {
		return key
	}
	return conn
}

// refreshOpenAIWSTurnBillingAPIKey 返回本 turn 计费用的 API Key：分组取当前认证
// 快照，其余字段与建连快照共享。不满足采用条件时原样返回建连快照。
func refreshOpenAIWSTurnBillingAPIKey(ctx context.Context, lookup openAIWSTurnAPIKeyLookup, conn *service.APIKey) *service.APIKey {
	if lookup == nil || conn == nil || conn.Key == "" || conn.GroupID == nil || conn.Group == nil {
		return conn
	}
	latest, err := lookup.GetByKey(ctx, conn.Key)
	if err != nil || latest == nil || latest.ID != conn.ID {
		return conn
	}
	group := latest.Group
	if latest.GroupID == nil || *latest.GroupID != *conn.GroupID {
		group = nil
		for _, binding := range latest.GroupBindings {
			if binding.GroupID == *conn.GroupID {
				group = binding.Group
				break
			}
		}
	}
	if group == nil || group.ID != conn.Group.ID ||
		group.Platform != conn.Group.Platform ||
		group.SubscriptionType != conn.Group.SubscriptionType {
		return conn
	}
	turnKey := *conn
	turnKey.Group = group
	return &turnKey
}

// withOpenAIWSTurnBillingGroup 把本 turn 的计费分组换进认证分组上下文，使利润门
// 的售价与本 turn 计费同源。上下文里没有同 ID 的认证分组时不改动。
func withOpenAIWSTurnBillingGroup(ctx context.Context, turnKey *service.APIKey) context.Context {
	if turnKey == nil || turnKey.Group == nil {
		return ctx
	}
	current, ok := ctx.Value(ctxkey.Group).(*service.Group)
	if !ok || current == nil || current == turnKey.Group || current.ID != turnKey.Group.ID {
		return ctx
	}
	return context.WithValue(ctx, ctxkey.Group, turnKey.Group)
}
