package service

import (
	"strings"
	"sync"
	"time"

	"github.com/cespare/xxhash/v2"
)

const (
	openAISessionIDRateMinute      = time.Minute
	openAISessionIDRateMaxAccounts = 20000
	openAISessionIDRateIdleTTL     = time.Hour
	openAISessionIDRateMaxIDs      = 1000000
)

// OpenAISessionIDRateMetrics tracks distinct OpenAI session hashes observed for
// each account in the current UTC minute. It is intentionally process-local and
// short-lived: this is an operational signal, not a billing or audit record.
type OpenAISessionIDRateMetrics struct {
	mu          sync.Mutex
	sweepMinute int64
	ids         int
	accounts    map[int64]*openAISessionIDRateAccount
}

type openAISessionIDRateAccount struct {
	minute int64
	count  int64
	seen   map[uint64]int64
}

type OpenAISessionIDRateSnapshot struct {
	Counts         map[int64]int64
	TotalPerMinute int64
	MaxPerMinute   int64
	MaxAccountID   int64
}

func NewOpenAISessionIDRateMetrics() *OpenAISessionIDRateMetrics {
	return &OpenAISessionIDRateMetrics{
		accounts: make(map[int64]*openAISessionIDRateAccount),
	}
}

var defaultOpenAISessionIDRateMetrics = NewOpenAISessionIDRateMetrics()

func DefaultOpenAISessionIDRateMetrics() *OpenAISessionIDRateMetrics {
	return defaultOpenAISessionIDRateMetrics
}

func (m *OpenAISessionIDRateMetrics) Record(accountID int64, sessionHash string, now time.Time) {
	if m == nil || accountID <= 0 || strings.TrimSpace(sessionHash) == "" {
		return
	}
	nowMinute := now.UTC().Unix() / int64(openAISessionIDRateMinute/time.Second)
	nowUnix := now.UTC().Unix()
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(nowMinute, nowUnix)
	if len(m.accounts) >= openAISessionIDRateMaxAccounts {
		if _, exists := m.accounts[accountID]; !exists {
			return
		}
	}
	account := m.accounts[accountID]
	if account == nil {
		account = &openAISessionIDRateAccount{minute: nowMinute, seen: make(map[uint64]int64)}
		m.accounts[accountID] = account
	}
	if account.minute != nowMinute {
		account.minute = nowMinute
		account.count = 0
	}
	hash := xxhash.Sum64String(sessionHash)
	if _, exists := account.seen[hash]; exists {
		account.seen[hash] = nowUnix
		return
	}
	if m.ids >= openAISessionIDRateMaxIDs {
		return
	}
	account.seen[hash] = nowUnix
	account.count++
	m.ids++
}

func (m *OpenAISessionIDRateMetrics) Snapshot(accountIDs []int64, now time.Time) OpenAISessionIDRateSnapshot {
	result := OpenAISessionIDRateSnapshot{Counts: make(map[int64]int64)}
	if m == nil {
		return result
	}
	nowMinute := now.UTC().Unix() / int64(openAISessionIDRateMinute/time.Second)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sweepLocked(nowMinute, now.UTC().Unix())
	if len(accountIDs) == 0 {
		for accountID, account := range m.accounts {
			result.add(accountID, account.countForMinute(nowMinute))
		}
		return result
	}
	seen := make(map[int64]struct{}, len(accountIDs))
	for _, accountID := range accountIDs {
		if accountID <= 0 {
			continue
		}
		if _, exists := seen[accountID]; exists {
			continue
		}
		seen[accountID] = struct{}{}
		result.add(accountID, m.accounts[accountID].countForMinute(nowMinute))
	}
	return result
}

func (s *OpenAISessionIDRateSnapshot) add(accountID, count int64) {
	if s == nil {
		return
	}
	s.Counts[accountID] = count
	s.TotalPerMinute += count
	if count > s.MaxPerMinute {
		s.MaxPerMinute = count
		s.MaxAccountID = accountID
	}
}

func (a *openAISessionIDRateAccount) countForMinute(minute int64) int64 {
	if a == nil || a.minute != minute {
		return 0
	}
	return a.count
}

func (m *OpenAISessionIDRateMetrics) sweepLocked(nowMinute, nowUnix int64) {
	if nowMinute == m.sweepMinute {
		return
	}
	m.sweepMinute = nowMinute
	cutoff := nowUnix - int64(openAISessionIDRateIdleTTL/time.Second)
	for accountID, account := range m.accounts {
		for hash, lastSeen := range account.seen {
			if lastSeen <= cutoff {
				delete(account.seen, hash)
				m.ids--
			}
		}
		if len(account.seen) == 0 {
			delete(m.accounts, accountID)
		}
	}
}
