package service

import (
	"github.com/DR-lin-eng/sub2api-no2api/internal/platform/config"
	"github.com/DR-lin-eng/sub2api-no2api/internal/shared/responseheaders"
)

func compileResponseHeaderFilter(cfg *config.Config) *responseheaders.CompiledHeaderFilter {
	if cfg == nil {
		return nil
	}
	return responseheaders.CompileHeaderFilter(cfg.Security.ResponseHeaders)
}
