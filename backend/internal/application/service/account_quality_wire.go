package service

import (
	"database/sql"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
)

// ProvideAccountQualityMonitoringService creates the independent quality
// monitoring runner. Its settings, lock and persisted state are isolated from
// the account health inspection runner.
func ProvideAccountQualityMonitoringService(
	accountRepo AccountRepository,
	settingRepo SettingRepository,
	lockCache LeaderLockCache,
	db *sql.DB,
	cfg *config.Config,
	accountTestSvc *AccountTestService,
	qualityProcessor AccountQualityArtifactProcessor,
	qualityArtifacts AccountQualityArtifactRepository,
	groupRepo GroupRepository,
) *AccountQualityMonitoringService {
	svc := NewAccountQualityMonitoringService(accountRepo, settingRepo, accountTestSvc, qualityProcessor, qualityArtifacts, groupRepo)
	svc.SetLeaderLock(lockCache, db)
	if cfg != nil && cfg.Deployment.WorkerEnabledResolved() {
		svc.Start()
	}
	return svc
}
