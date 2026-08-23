package service

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	"github.com/google/uuid"
)

const (
	defaultOAuthModelSyncInterval       = time.Hour
	defaultOAuthModelSyncAccountTimeout = 20 * time.Second
	defaultOAuthModelSyncConcurrency    = 2
	oauthModelSyncLeaderLockKey         = "sub2api:oauth-model-sync:leader"
	oauthModelSyncLeaderLockTTL         = 30 * time.Minute
)

var oauthModelSyncPlatforms = []string{
	PlatformOpenAI,
	PlatformAnthropic,
	PlatformGemini,
	PlatformAntigravity,
	PlatformGrok,
}

// OAuthModelSyncAccountRepository is the deliberately small persistence port
// needed by the background capability synchronizer. Keeping it narrower than
// AccountRepository makes the worker easy to exercise without coupling tests
// to unrelated account CRUD methods.
type OAuthModelSyncAccountRepository interface {
	ListByPlatform(ctx context.Context, platform string) ([]Account, error)
	UpdateExtra(ctx context.Context, id int64, updates map[string]any) error
}

// OAuthModelSyncFetcher abstracts the upstream model catalog probe. The
// production AccountTestService implementation routes each supported OAuth
// platform through the same authentication/model-list path used by manual
// account tests (Codex manifest for OpenAI).
type OAuthModelSyncFetcher interface {
	FetchUpstreamSupportedModels(ctx context.Context, account *Account) ([]string, error)
}

// OAuthModelSyncModelsCacheInvalidator lets a successful refresh immediately
// invalidate the short-lived /v1/models aggregation cache. It is optional so
// the worker remains usable in focused tests and minimal deployments.
type OAuthModelSyncModelsCacheInvalidator interface {
	InvalidateAvailableModelsCache(groupID *int64, platform string)
}

// OAuthModelSyncOptions controls one worker instance. Zero values are replaced
// with conservative defaults by NewOAuthModelSyncService.
type OAuthModelSyncOptions struct {
	Enabled        bool
	Interval       time.Duration
	AccountTimeout time.Duration
	MaxConcurrent  int
	CycleTimeout   time.Duration
}

// OAuthModelSyncStats describes one best-effort synchronization cycle.
type OAuthModelSyncStats struct {
	Considered      int
	Updated         int
	SkippedExplicit int
	SkippedInactive int
	SkippedRefresh  int
	Failed          int
}

// OAuthModelSyncService periodically refreshes live model capabilities for
// OAuth accounts without an explicit model mapping. A successful probe
// replaces only the hidden extra snapshot; explicit mappings and passthrough
// accounts are never modified.
type OAuthModelSyncService struct {
	repo    OAuthModelSyncAccountRepository
	fetcher OAuthModelSyncFetcher
	opts    OAuthModelSyncOptions

	lockCache  LeaderLockCache
	db         *sql.DB
	instanceID string
	cache      OAuthModelSyncModelsCacheInvalidator

	mu     sync.Mutex
	cancel context.CancelFunc
	done   chan struct{}
	runMu  sync.Mutex
}

func NewOAuthModelSyncService(
	repo OAuthModelSyncAccountRepository,
	fetcher OAuthModelSyncFetcher,
	opts OAuthModelSyncOptions,
) *OAuthModelSyncService {
	if opts.Interval <= 0 {
		opts.Interval = defaultOAuthModelSyncInterval
	}
	if opts.AccountTimeout <= 0 {
		opts.AccountTimeout = defaultOAuthModelSyncAccountTimeout
	}
	if opts.MaxConcurrent <= 0 {
		opts.MaxConcurrent = defaultOAuthModelSyncConcurrency
	}
	if opts.MaxConcurrent > 32 {
		opts.MaxConcurrent = 32
	}
	if opts.CycleTimeout <= 0 {
		opts.CycleTimeout = 10 * time.Minute
	}
	return &OAuthModelSyncService{
		repo:       repo,
		fetcher:    fetcher,
		opts:       opts,
		instanceID: uuid.NewString(),
	}
}

// NewOAuthModelSyncServiceFromConfig builds the worker from deployment
// configuration. A nil config keeps the feature disabled, which is useful for
// lightweight command/test wiring; production config always supplies defaults.
func NewOAuthModelSyncServiceFromConfig(
	repo OAuthModelSyncAccountRepository,
	fetcher OAuthModelSyncFetcher,
	cfg *config.Config,
) *OAuthModelSyncService {
	if cfg == nil {
		return NewOAuthModelSyncService(repo, fetcher, OAuthModelSyncOptions{})
	}
	options := OAuthModelSyncOptions{
		Enabled:        cfg.OAuthModelSync.Enabled,
		Interval:       time.Duration(cfg.OAuthModelSync.IntervalMinutes) * time.Minute,
		AccountTimeout: time.Duration(cfg.OAuthModelSync.AccountTimeoutSeconds) * time.Second,
		MaxConcurrent:  cfg.OAuthModelSync.MaxConcurrent,
	}
	return NewOAuthModelSyncService(repo, fetcher, options)
}

// SetLeaderLock enables cross-instance single-flight execution. Without a
// coordination backend (for example in unit tests), each instance runs its
// own cycle by design.
func (s *OAuthModelSyncService) SetLeaderLock(lockCache LeaderLockCache, db *sql.DB) {
	if s == nil {
		return
	}
	s.lockCache = lockCache
	s.db = db
}

func (s *OAuthModelSyncService) SetModelsCacheInvalidator(invalidator OAuthModelSyncModelsCacheInvalidator) {
	if s == nil {
		return
	}
	s.cache = invalidator
}

// Start launches the periodic worker. The first cycle runs immediately so a
// freshly deployed instance does not wait a full interval before correcting a
// stale capability snapshot.
func (s *OAuthModelSyncService) Start() {
	if s == nil || !s.opts.Enabled || s.repo == nil || s.fetcher == nil || s.opts.Interval <= 0 {
		return
	}
	s.mu.Lock()
	if s.cancel != nil {
		s.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.done = make(chan struct{})
	done := s.done
	s.mu.Unlock()

	go func() {
		defer close(done)
		slog.Info("oauth_model_sync.started",
			"interval", s.opts.Interval.String(),
			"account_timeout", s.opts.AccountTimeout.String(),
			"max_concurrent", s.opts.MaxConcurrent,
		)
		ticker := time.NewTicker(s.opts.Interval)
		defer ticker.Stop()
		s.runOnce(ctx)
		for {
			select {
			case <-ticker.C:
				s.runOnce(ctx)
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Stop terminates the worker and waits for an in-flight cycle to finish.
func (s *OAuthModelSyncService) Stop() {
	if s == nil {
		return
	}
	s.mu.Lock()
	cancel := s.cancel
	done := s.done
	s.cancel = nil
	s.done = nil
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// RunOnce executes one cycle synchronously. It is exported for an operational
// admin/diagnostic caller and for deterministic tests; normal production use
// goes through Start.
func (s *OAuthModelSyncService) RunOnce(ctx context.Context) OAuthModelSyncStats {
	if s == nil {
		return OAuthModelSyncStats{}
	}
	return s.runOnce(ctx)
}

func (s *OAuthModelSyncService) runOnce(ctx context.Context) OAuthModelSyncStats {
	if ctx == nil {
		ctx = context.Background()
	}
	if s.repo == nil || s.fetcher == nil || !s.opts.Enabled {
		return OAuthModelSyncStats{}
	}

	// Prevent a manually-triggered run from overlapping the ticker run in the
	// same process. The distributed leader lock below handles other instances.
	s.runMu.Lock()
	defer s.runMu.Unlock()

	cycleCtx, cancel := context.WithTimeout(ctx, s.opts.CycleTimeout)
	defer cancel()
	release, acquired := tryAcquireSingletonLeaderLock(
		cycleCtx,
		s.lockCache,
		s.db,
		oauthModelSyncLeaderLockKey,
		s.instanceID,
		oauthModelSyncLeaderLockTTL,
	)
	if !acquired {
		return OAuthModelSyncStats{}
	}
	defer release()

	accounts := make([]Account, 0)
	seenAccountIDs := make(map[int64]struct{})
	for _, platform := range oauthModelSyncPlatforms {
		platformAccounts, err := s.repo.ListByPlatform(cycleCtx, platform)
		if err != nil {
			slog.Warn("oauth_model_sync.list_failed", "platform", platform, "error", err)
			continue
		}
		for _, account := range platformAccounts {
			if account.ID > 0 {
				if _, seen := seenAccountIDs[account.ID]; seen {
					continue
				}
				seenAccountIDs[account.ID] = struct{}{}
			}
			accounts = append(accounts, account)
		}
	}

	candidates := make([]Account, 0, len(accounts))
	stats := OAuthModelSyncStats{}
	for _, account := range accounts {
		if !isOAuthModelSyncEligibleAccount(&account) {
			continue
		}
		if account.Status != "" && account.Status != StatusActive {
			stats.SkippedInactive++
			continue
		}
		// The manifest client uses the account's current access token. Let the
		// dedicated token-refresh worker renew tokens that are expired or within
		// its refresh skew first; probing with an about-to-expire token could
		// otherwise create a needless OAuth 401 cooldown.
		if expiresAt := account.GetCredentialAsTime("expires_at"); expiresAt != nil &&
			!account.IsOpenAIPersonalAccessToken() && time.Until(*expiresAt) <= openAITokenRefreshSkew {
			stats.SkippedRefresh++
			continue
		}
		if account.HasExplicitModelMapping() || account.IsOpenAIPassthroughEnabled() {
			stats.SkippedExplicit++
			continue
		}
		candidates = append(candidates, account)
	}
	stats.Considered = len(candidates)
	if len(candidates) == 0 {
		return stats
	}

	workerCount := s.opts.MaxConcurrent
	if workerCount > len(candidates) {
		workerCount = len(candidates)
	}
	jobs := make(chan Account)
	results := make(chan bool, len(candidates))
	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for account := range jobs {
				results <- s.syncAccount(cycleCtx, account)
			}
		}()
	}

dispatch:
	for _, account := range candidates {
		select {
		case jobs <- account:
		case <-cycleCtx.Done():
			break dispatch
		}
	}
	close(jobs)
	wg.Wait()
	close(results)
	for updated := range results {
		if updated {
			stats.Updated++
		} else {
			stats.Failed++
		}
	}
	if stats.Updated > 0 && s.cache != nil {
		for _, platform := range oauthModelSyncPlatforms {
			s.cache.InvalidateAvailableModelsCache(nil, platform)
		}
	}
	if stats.Considered > 0 || stats.Failed > 0 {
		slog.Info("oauth_model_sync.completed",
			"considered", stats.Considered,
			"updated", stats.Updated,
			"failed", stats.Failed,
			"skipped_explicit", stats.SkippedExplicit,
			"skipped_inactive", stats.SkippedInactive,
			"skipped_refresh", stats.SkippedRefresh,
		)
	}
	return stats
}

func isOAuthModelSyncEligibleAccount(account *Account) bool {
	if account == nil || !account.IsOAuth() {
		return false
	}
	// OpenAI's Codex manifest is an OAuth-only endpoint; setup-token accounts
	// use the Anthropic credential semantics and must not be sent there.
	if account.Platform == PlatformOpenAI {
		return account.IsOpenAIOAuth()
	}
	if account.Platform == PlatformGemini && account.IsGeminiCodeAssist() {
		// The Code Assist API exposes entitlement-specific models through its
		// project endpoint rather than the public /v1beta/models catalog.
		return false
	}
	// Other platform model catalogs currently accept regular OAuth credentials
	// (Anthropic also supports setup-token via its existing test path).
	return account.Type == AccountTypeOAuth ||
		(account.Platform == PlatformAnthropic && account.Type == AccountTypeSetupToken)
}

func (s *OAuthModelSyncService) syncAccount(ctx context.Context, account Account) bool {
	accountCtx, cancel := context.WithTimeout(ctx, s.opts.AccountTimeout)
	defer cancel()
	models, err := s.fetcher.FetchUpstreamSupportedModels(accountCtx, &account)
	if err != nil {
		// Keep the previous snapshot on failure. Replacing it with an empty list
		// would make a transient 429/timeout look like a permanent capability
		// revocation and could unnecessarily deny all requests.
		message := "upstream model sync failed"
		var syncErr *UpstreamModelSyncError
		if errors.As(err, &syncErr) {
			message = syncErr.SafeMessage()
		}
		slog.Warn("oauth_model_sync.account_failed", "account_id", account.ID, "reason", message)
		return false
	}
	normalized := normalizeOAuthSupportedModelValues(models)
	if len(normalized) == 0 {
		slog.Warn("oauth_model_sync.account_empty", "account_id", account.ID)
		return false
	}
	if err := s.repo.UpdateExtra(ctx, account.ID, map[string]any{
		OAuthSupportedModelsExtraKey:         normalized,
		OAuthSupportedModelsSyncedAtExtraKey: time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		slog.Warn("oauth_model_sync.account_persist_failed", "account_id", account.ID, "error", err)
		return false
	}
	return true
}
