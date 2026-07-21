package service

import (
	"sync"
	"sync/atomic"
	"time"
)

const apiKeyLastUsedCacheCapacity = 32768

// apiKeyLastUsedDebounceCache keeps the single-key hit path on sync.Map's
// read-only fast path while bounding cold-key retention above the supported
// 20k-key standalone deployment target.
type apiKeyLastUsedDebounceCache struct {
	entries sync.Map // keyID -> expiresAtUnixNano(int64)
	size    atomic.Int64
	pruneMu sync.Mutex
}

func (c *apiKeyLastUsedDebounceCache) Get(keyID int64, now time.Time) (time.Time, bool) {
	value, ok := c.entries.Load(keyID)
	if !ok {
		return time.Time{}, false
	}
	expiresAt, ok := value.(int64)
	if !ok || expiresAt <= now.UnixNano() {
		if c.entries.CompareAndDelete(keyID, value) {
			c.size.Add(-1)
		}
		return time.Time{}, false
	}
	return time.Unix(0, expiresAt), true
}

func (c *apiKeyLastUsedDebounceCache) Store(keyID int64, expiresAt time.Time) {
	expiresAtUnix := expiresAt.UnixNano()
	for {
		current, loaded := c.entries.LoadOrStore(keyID, expiresAtUnix)
		if !loaded {
			c.size.Add(1)
			break
		}
		if c.entries.CompareAndSwap(keyID, current, expiresAtUnix) {
			break
		}
	}
	if c.size.Load() > apiKeyLastUsedCacheCapacity {
		c.prune(time.Now().UnixNano())
	}
}

func (c *apiKeyLastUsedDebounceCache) Delete(keyID int64) {
	if _, loaded := c.entries.LoadAndDelete(keyID); loaded {
		c.size.Add(-1)
	}
}

func (c *apiKeyLastUsedDebounceCache) Len() int {
	return int(c.size.Load())
}

func (c *apiKeyLastUsedDebounceCache) prune(nowUnix int64) {
	if !c.pruneMu.TryLock() {
		return
	}
	defer c.pruneMu.Unlock()
	c.entries.Range(func(key, value any) bool {
		expiresAt, ok := value.(int64)
		if !ok || expiresAt <= nowUnix {
			if c.entries.CompareAndDelete(key, value) {
				c.size.Add(-1)
			}
		}
		return true
	})
	if c.size.Load() <= apiKeyLastUsedCacheCapacity {
		return
	}
	c.entries.Range(func(key, value any) bool {
		if c.size.Load() <= apiKeyLastUsedCacheCapacity {
			return false
		}
		if c.entries.CompareAndDelete(key, value) {
			c.size.Add(-1)
		}
		return true
	})
}
