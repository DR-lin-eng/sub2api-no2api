package egress

import (
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/google/wire"
)

func ProvideService(store Store, settings RuntimeSettings, cfg *config.Config) *Service {
	service := NewService(store, cfg)
	service.SetRuntimeSettings(settings)
	return service
}

var ProviderSet = wire.NewSet(ProvideService, NewHETunnelControlService)
