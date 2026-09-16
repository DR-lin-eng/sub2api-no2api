package service

import "database/sql"

// ProvideProxyHealthService starts the persisted automatic proxy-pool monitor.
func ProvideProxyHealthService(
	proxyRepo ProxyRepository,
	settingService *SettingService,
	prober ProxyExitInfoProber,
	lockCache LeaderLockCache,
	db *sql.DB,
) *ProxyHealthService {
	healthRepo, _ := proxyRepo.(ProxyHealthRepository)
	svc := NewProxyHealthService(healthRepo, settingService, prober)
	svc.SetLeaderLock(lockCache, db)
	svc.Start()
	return svc
}
