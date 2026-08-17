package geminicli

import (
	"context"
	"errors"
	"testing"

	platformegress "github.com/Wei-Shaw/sub2api/internal/platform/egress"
)

func TestDriveClientExplicitIPv6DisabledFailsClosed(t *testing.T) {
	ctx := platformegress.WithContextRoute(
		context.Background(),
		platformegress.IPv6PoolRoute("2001:db8::30", 3, 1, false),
		platformegress.Policy{},
	)
	_, err := NewDriveClient().GetStorageQuota(ctx, "token", "")
	if !errors.Is(err, platformegress.ErrIPv6Disabled) {
		t.Fatalf("GetStorageQuota() error = %v", err)
	}
}

func TestDriveStorageInfo(t *testing.T) {
	// 测试 DriveStorageInfo 结构体
	info := &DriveStorageInfo{
		Limit: 100 * 1024 * 1024 * 1024, // 100GB
		Usage: 50 * 1024 * 1024 * 1024,  // 50GB
	}

	if info.Limit != 100*1024*1024*1024 {
		t.Errorf("Expected limit 100GB, got %d", info.Limit)
	}
	if info.Usage != 50*1024*1024*1024 {
		t.Errorf("Expected usage 50GB, got %d", info.Usage)
	}
}
