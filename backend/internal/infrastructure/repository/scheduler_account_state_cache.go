package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/DR-lin-eng/sub2api-no2api/internal/application/service"
	"github.com/redis/go-redis/v9"
)

func (c *schedulerCache) GetCachedAccountSchedulingState(ctx context.Context, accountID int64) (service.AccountSchedulingState, bool, error) {
	if accountID <= 0 {
		return service.AccountSchedulingState{}, false, nil
	}

	value, err := c.rdb.Get(ctx, schedulerAccountMetaKey(strconv.FormatInt(accountID, 10))).Result()
	if err == redis.Nil {
		return service.AccountSchedulingState{}, false, nil
	}
	if err != nil {
		return service.AccountSchedulingState{}, false, err
	}
	state, err := decodeCachedAccountSchedulingState(value)
	if err != nil {
		return service.AccountSchedulingState{}, false, err
	}
	return state, true, nil
}

type cachedAccountSchedulingProjection struct {
	Platform                string
	Type                    string
	Status                  string
	Schedulable             bool
	ExpiresAt               *time.Time
	AutoPauseOnExpired      bool
	OverloadUntil           *time.Time
	RateLimitResetAt        *time.Time
	TempUnschedulableUntil  *time.Time
	TempUnschedulableReason string
	Extra                   map[string]any
}

func decodeCachedAccountSchedulingState(value any) (service.AccountSchedulingState, error) {
	var payload []byte
	switch raw := value.(type) {
	case string:
		payload = []byte(raw)
	case []byte:
		payload = raw
	default:
		return service.AccountSchedulingState{}, fmt.Errorf("unexpected account scheduling cache type: %T", value)
	}

	var projection cachedAccountSchedulingProjection
	if err := json.Unmarshal(payload, &projection); err != nil {
		return service.AccountSchedulingState{}, err
	}
	account := service.Account{
		Platform:                projection.Platform,
		Type:                    projection.Type,
		Status:                  projection.Status,
		Schedulable:             projection.Schedulable,
		ExpiresAt:               projection.ExpiresAt,
		AutoPauseOnExpired:      projection.AutoPauseOnExpired,
		OverloadUntil:           projection.OverloadUntil,
		RateLimitResetAt:        projection.RateLimitResetAt,
		TempUnschedulableUntil:  projection.TempUnschedulableUntil,
		TempUnschedulableReason: projection.TempUnschedulableReason,
		Extra:                   projection.Extra,
	}
	return service.AccountSchedulingState{Exists: true, Schedulable: account.IsSchedulable()}, nil
}
