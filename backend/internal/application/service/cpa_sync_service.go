package service

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Wei-Shaw/sub2api/internal/modules/cpasync"
	"github.com/Wei-Shaw/sub2api/internal/platform/config"
	infraerrors "github.com/Wei-Shaw/sub2api/internal/shared/errors"
	"github.com/Wei-Shaw/sub2api/internal/shared/httpclient"
	"github.com/Wei-Shaw/sub2api/internal/shared/urlvalidator"
)

const cpaSyncSourceKey = "cpa_sync_source"
const cpaSyncFileKey = "cpa_sync_file"
const cpaSyncedAtKey = "cpa_synced_at"

type CPAConnectionInput struct {
	BaseURL            string `json:"base_url"`
	ManagementPassword string `json:"management_password"`
	Platform           string `json:"platform"`
}
type CPASyncInput struct {
	CPAConnectionInput
	SelectedFiles           []string        `json:"selected_files"`
	GroupIDs                []int64         `json:"group_ids"`
	OAuthOptions            CPAOAuthOptions `json:"oauth_options"`
	ApplySettingsToExisting bool            `json:"apply_settings_to_existing"`
}
type CPAPreviewAccount struct {
	FileName  string `json:"file_name"`
	Name      string `json:"name"`
	Email     string `json:"email,omitempty"`
	Platform  string `json:"platform"`
	Existing  bool   `json:"existing"`
	AccountID int64  `json:"account_id,omitempty"`
}
type CPAPreviewResult struct {
	Accounts    []CPAPreviewAccount `json:"accounts"`
	Total       int                 `json:"total"`
	Skipped     int                 `json:"skipped"`
	SkipReasons map[string]int      `json:"skip_reasons"`
	BatchSize   int                 `json:"batch_size"`
}
type CPASyncItem struct {
	FileName  string `json:"file_name"`
	Action    string `json:"action"`
	Reason    string `json:"reason,omitempty"`
	Message   string `json:"message,omitempty"`
	AccountID int64  `json:"account_id,omitempty"`
}
type CPASyncResult struct {
	Created int           `json:"created"`
	Updated int           `json:"updated"`
	Skipped int           `json:"skipped"`
	Failed  int           `json:"failed"`
	Items   []CPASyncItem `json:"items"`
}

type CPASyncService struct {
	repo        AccountRepository
	admin       AdminService
	cfg         *config.Config
	db          *sql.DB
	invalidator TokenCacheInvalidator
	mu          sync.Mutex
}

func NewCPASyncService(repo AccountRepository, admin AdminService, cfg *config.Config, db *sql.DB, invalidator TokenCacheInvalidator) *CPASyncService {
	return &CPASyncService{repo: repo, admin: admin, cfg: cfg, db: db, invalidator: invalidator}
}

func (s *CPASyncService) connection(input CPAConnectionInput) (*cpasync.Client, string, error) {
	if s == nil || s.cfg == nil {
		return nil, "", infraerrors.New(503, "CPA_SYNC_UNAVAILABLE", "CPA sync is not configured")
	}
	switch input.Platform {
	case PlatformOpenAI, PlatformAnthropic, PlatformGemini, PlatformAntigravity:
	default:
		return nil, "", infraerrors.BadRequest("INVALID_CPA_PLATFORM", "select a supported CPA OAuth platform")
	}
	password := strings.TrimSpace(input.ManagementPassword)
	if password == "" || len(password) > 4096 || strings.ContainsAny(password, "\r\n") {
		return nil, "", infraerrors.BadRequest("INVALID_CPA_PASSWORD", "CPA administrator password is required")
	}
	endpoint, err := cpasync.Endpoint(input.BaseURL)
	if err != nil {
		return nil, "", infraerrors.BadRequest("INVALID_CPA_URL", "invalid CPA management URL")
	}
	policy := s.cfg.Security.URLAllowlist
	opts := urlvalidator.ValidationOptions{AllowPrivate: policy.AllowPrivateHosts}
	if policy.Enabled {
		opts.AllowedHosts = policy.CRSHosts
		opts.RequireAllowlist = len(policy.CRSHosts) > 0
	}
	endpoint, err = urlvalidator.ValidateHTTPURL(endpoint, policy.AllowInsecureHTTP, opts)
	if err != nil {
		return nil, "", infraerrors.BadRequest("INVALID_CPA_URL", "CPA URL does not match the configured remote-sync URL policy")
	}
	client, err := httpclient.GetClient(httpclient.Options{Timeout: 10 * time.Second, ValidateResolvedIP: true, AllowPrivateHosts: policy.AllowPrivateHosts, MaxConnsPerHost: 4})
	if err != nil {
		return nil, "", infraerrors.New(502, "CPA_CONNECTION_FAILED", "CPA HTTP client could not be created")
	}
	source := fmt.Sprintf("%x", sha256.Sum256([]byte(endpoint)))
	return cpasync.NewClient(endpoint, password, client), source, nil
}

func (s *CPASyncService) existing(ctx context.Context, source string) (map[string]*Account, error) {
	accounts, err := s.repo.FindByExtraField(ctx, cpaSyncSourceKey, source)
	if err != nil {
		return nil, infraerrors.New(500, "CPA_ACCOUNT_LOOKUP_FAILED", "local CPA accounts could not be loaded")
	}
	result := make(map[string]*Account, len(accounts))
	for i := range accounts {
		account := &accounts[i]
		if account.IsCredentialShadow() {
			continue
		}
		name, _ := account.Extra[cpaSyncFileKey].(string)
		if name == "" {
			continue
		}
		if _, duplicate := result[name]; duplicate {
			return nil, infraerrors.New(409, "CPA_DUPLICATE_SYNC_IDENTITY", "multiple local accounts have the same CPA file identity")
		}
		result[name] = account
	}
	return result, nil
}

func cpaFilesByName(files []cpasync.File) (map[string]cpasync.File, map[string]bool) {
	byName := make(map[string]cpasync.File, len(files))
	duplicates := make(map[string]bool)
	for _, file := range files {
		if _, ok := byName[file.Name]; ok {
			duplicates[file.Name] = true
		}
		byName[file.Name] = file
	}
	return byName, duplicates
}
func cpaFileSkipReason(file cpasync.File, platform string, duplicate bool) string {
	if duplicate {
		return "duplicate_file_name"
	}
	if reason := file.SkipReason(time.Now()); reason != "" {
		return reason
	}
	if file.Platform() != platform {
		return "other_platform"
	}
	return ""
}
func cpaRemoteError(err error) error {
	return infraerrors.New(502, "CPA_CONNECTION_FAILED", err.Error())
}

func (s *CPASyncService) Preview(ctx context.Context, input CPAConnectionInput) (*CPAPreviewResult, error) {
	client, source, err := s.connection(input)
	if err != nil {
		return nil, err
	}
	files, err := client.List(ctx)
	if err != nil {
		return nil, cpaRemoteError(err)
	}
	existing, err := s.existing(ctx, source)
	if err != nil {
		return nil, err
	}
	_, duplicates := cpaFilesByName(files)
	result := &CPAPreviewResult{Accounts: []CPAPreviewAccount{}, Total: len(files), SkipReasons: map[string]int{}, BatchSize: cpasync.MaxBatchSize}
	for _, file := range files {
		if reason := cpaFileSkipReason(file, input.Platform, duplicates[file.Name]); reason != "" {
			result.Skipped++
			result.SkipReasons[reason]++
			continue
		}
		item := CPAPreviewAccount{FileName: file.Name, Name: file.Label, Email: file.Email, Platform: file.Platform()}
		if item.Name == "" {
			item.Name = file.Name
		}
		if account := existing[file.Name]; account != nil {
			item.Existing = true
			item.AccountID = account.ID
		}
		result.Accounts = append(result.Accounts, item)
	}
	sort.Slice(result.Accounts, func(i, j int) bool { return result.Accounts[i].FileName < result.Accounts[j].FileName })
	return result, nil
}

func (s *CPASyncService) validateSync(ctx context.Context, input CPASyncInput) error {
	if len(input.SelectedFiles) == 0 || len(input.SelectedFiles) > cpasync.MaxBatchSize {
		return infraerrors.BadRequest("INVALID_CPA_SELECTION", "select between 1 and 100 CPA files per batch")
	}
	seen := make(map[string]bool, len(input.SelectedFiles))
	for _, name := range input.SelectedFiles {
		if !cpasync.ValidName(name) || seen[name] {
			return infraerrors.BadRequest("INVALID_CPA_SELECTION", "CPA file selection contains invalid or duplicate names")
		}
		seen[name] = true
	}
	if len(input.GroupIDs) > 50 {
		return infraerrors.BadRequest("INVALID_CPA_GROUP", "too many target groups")
	}
	for _, id := range input.GroupIDs {
		if id <= 0 {
			return infraerrors.BadRequest("INVALID_CPA_GROUP", "invalid target group")
		}
		group, err := s.admin.GetGroup(ctx, id)
		if err != nil || group == nil || !group.IsActive() || (group.Platform != input.Platform && group.Platform != PlatformComposite) {
			return infraerrors.BadRequest("INVALID_CPA_GROUP", "target groups must be active and match the selected platform")
		}
	}
	return input.OAuthOptions.validate(input.Platform)
}

func (s *CPASyncService) Sync(ctx context.Context, input CPASyncInput) (*CPASyncResult, error) {
	client, source, err := s.connection(input.CPAConnectionInput)
	if err != nil {
		return nil, err
	}
	if err := s.validateSync(ctx, input); err != nil {
		return nil, err
	}
	// Refuse a competing operation rather than holding an unbounded import queue.
	if !s.mu.TryLock() {
		return nil, infraerrors.New(409, "CPA_SYNC_BUSY", "another CPA sync is running")
	}
	defer s.mu.Unlock()
	if s.db != nil {
		release, acquired, lockErr := tryAcquireDBAdvisoryLockWithError(ctx, s.db, hashAdvisoryLockID("cpa-sync:"+source))
		if lockErr != nil {
			return nil, infraerrors.New(503, "CPA_SYNC_LOCK_FAILED", "CPA sync lock could not be acquired")
		}
		if !acquired {
			return nil, infraerrors.New(409, "CPA_SYNC_BUSY", "this CPA source is already being synced")
		}
		defer release()
	}
	ctx, cancel := context.WithTimeout(ctx, 150*time.Second)
	defer cancel()
	files, err := client.List(ctx)
	if err != nil {
		return nil, cpaRemoteError(err)
	}
	byName, duplicates := cpaFilesByName(files)
	existing, err := s.existing(ctx, source)
	if err != nil {
		return nil, err
	}
	result := &CPASyncResult{Items: make([]CPASyncItem, len(input.SelectedFiles))}
	data := make([]*cpasync.AccountData, len(input.SelectedFiles))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for index := range jobs {
				name := input.SelectedFiles[index]
				item := CPASyncItem{FileName: name}
				file, ok := byName[name]
				reason := "removed"
				if ok {
					reason = cpaFileSkipReason(file, input.Platform, duplicates[name])
				}
				if reason != "" {
					item.Action, item.Reason = "skipped", reason
					result.Items[index] = item
					continue
				}
				raw, downloadErr := client.Download(ctx, name)
				if downloadErr != nil {
					item.Action, item.Reason, item.Message = "failed", "download_failed", downloadErr.Error()
					result.Items[index] = item
					continue
				}
				converted, convertErr := cpasync.Convert(file, raw)
				if convertErr != nil {
					item.Action, item.Reason, item.Message = "skipped", "invalid_credential", convertErr.Error()
					result.Items[index] = item
					continue
				}
				data[index] = converted
				result.Items[index] = item
			}
		}()
	}
	for index := range input.SelectedFiles {
		jobs <- index
	}
	close(jobs)
	wg.Wait()
	// Recheck live state after downloading and before the first local mutation;
	// a preview or credential file never overrides the manager's current status.
	fresh, err := client.List(ctx)
	if err != nil {
		return nil, cpaRemoteError(err)
	}
	byName, duplicates = cpaFilesByName(fresh)
	for index := range result.Items {
		item := &result.Items[index]
		if item.Action != "" {
			continue
		}
		file, ok := byName[item.FileName]
		if !ok {
			item.Action, item.Reason = "skipped", "removed"
			continue
		}
		if reason := cpaFileSkipReason(file, input.Platform, duplicates[item.FileName]); reason != "" {
			item.Action, item.Reason = "skipped", reason
			continue
		}
		if ctx.Err() != nil {
			item.Action, item.Reason = "failed", "cancelled"
			continue
		}
		s.save(ctx, input, source, existing[item.FileName], data[index], item)
	}
	for _, item := range result.Items {
		switch item.Action {
		case "created":
			result.Created++
		case "updated":
			result.Updated++
		case "skipped":
			result.Skipped++
		default:
			result.Failed++
		}
	}
	return result, nil
}

func (s *CPASyncService) save(ctx context.Context, input CPASyncInput, source string, existing *Account, data *cpasync.AccountData, item *CPASyncItem) {
	extra := data.Extra
	credentials := data.Credentials
	if existing != nil {
		if existing.Platform != input.Platform || existing.Type != AccountTypeOAuth {
			item.Action, item.Reason = "failed", "local_type_conflict"
			return
		}
		for _, key := range []string{"chatgpt_account_id", "chatgpt_user_id"} {
			oldValue, newValue := existing.GetCredential(key), fmt.Sprint(credentials[key])
			if oldValue != "" && credentials[key] != nil && newValue != "" && oldValue != newValue {
				item.Action, item.Reason = "failed", "identity_changed"
				return
			}
		}
		credentials = mergeMap(existing.Credentials, credentials)
		extra = mergeMap(existing.Extra, extra)
	}
	extra[cpaSyncSourceKey], extra[cpaSyncFileKey], extra[cpaSyncedAtKey] = source, item.FileName, time.Now().UTC().Format(time.RFC3339)
	if existing == nil || input.ApplySettingsToExisting {
		input.OAuthOptions.apply(credentials, extra)
	}
	var account *Account
	var err error
	if existing == nil {
		account, err = s.admin.CreateAccount(ctx, &CreateAccountInput{Name: data.Name, Platform: input.Platform, Type: AccountTypeOAuth, Credentials: credentials, Extra: extra, Concurrency: 3, Priority: 50, GroupIDs: input.GroupIDs, SkipDefaultGroupBind: true})
		item.Action = "created"
	} else {
		update := &UpdateAccountInput{Credentials: credentials, Extra: extra}
		if input.ApplySettingsToExisting && input.GroupIDs != nil {
			ids := append([]int64{}, input.GroupIDs...)
			update.GroupIDs = &ids
		}
		account, err = s.admin.UpdateAccount(ctx, existing.ID, update)
		item.Action = "updated"
	}
	if err != nil || account == nil {
		item.Action, item.Reason = "failed", "save_failed"
		return
	}
	item.AccountID = account.ID
	if s.invalidator != nil {
		if err := s.invalidator.InvalidateToken(ctx, account); err != nil {
			item.Reason = "token_cache_invalidation_failed"
		}
	}
}
