package egress

import "github.com/google/wire"

var ProviderSet = wire.NewSet(NewService, NewHETunnelControlService)
