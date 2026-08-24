// Package chatgptcookies keeps the small process-scoped Cloudflare cookie
// surface used by Codex ChatGPT HTTP clients. It deliberately rejects account,
// auth and session cookies, while retaining host/domain/path matching so sharing
// the store cannot cross-send cookies between ChatGPT hosts or paths.
package chatgptcookies

import (
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

type storedCookie struct {
	name      string
	value     string
	domain    string
	path      string
	hostOnly  bool
	expiresAt time.Time
	createdAt time.Time
}

type store struct {
	mu       sync.Mutex
	cookies  map[string]storedCookie
	rejected uint64
	expired  uint64
}

const maxStoredCookies = 256

type Stats struct {
	Stored   int
	Rejected uint64
	Expired  uint64
}

type ObservableJar interface {
	http.CookieJar
	Stats() Stats
	Clear()
}

var globalStore = &store{cookies: make(map[string]storedCookie)}

// NewJar returns a CookieJar backed by the process-scoped Cloudflare store.
// Only allowlisted Cloudflare cookies are accepted.
func NewJar() http.CookieJar {
	return &jar{store: globalStore}
}

// NewObservableJar returns the same process-scoped jar with lifecycle stats
// available through CookieStoreStats. It is useful for admin diagnostics
// without exposing cookie values.
func NewObservableJar() ObservableJar {
	return &jar{store: globalStore}
}

type jar struct{ store *store }

func (j *jar) SetCookies(rawURL *url.URL, cookies []*http.Cookie) {
	if j == nil || j.store == nil || !allowedURL(rawURL) {
		return
	}
	host := canonicalHost(rawURL)
	defaultPath := defaultCookiePath(rawURL.Path)
	now := time.Now()
	j.store.mu.Lock()
	defer j.store.mu.Unlock()
	for _, cookie := range cookies {
		if cookie == nil || !AllowedName(cookie.Name) {
			j.store.rejected++
			continue
		}
		domain, hostOnly, ok := cookieDomain(cookie.Domain, host)
		if !ok {
			j.store.rejected++
			continue
		}
		path := cookie.Path
		if path == "" || path[0] != '/' {
			path = defaultPath
		}
		key := cookieKey(domain, path, cookie.Name)
		if cookie.MaxAge < 0 || (!cookie.Expires.IsZero() && !cookie.Expires.After(now)) {
			delete(j.store.cookies, key)
			j.store.expired++
			continue
		}
		expiresAt := time.Time{}
		if cookie.MaxAge > 0 {
			expiresAt = now.Add(time.Duration(cookie.MaxAge) * time.Second)
		} else if !cookie.Expires.IsZero() {
			expiresAt = cookie.Expires
		}
		j.store.cookies[key] = storedCookie{
			name:      cookie.Name,
			value:     cookie.Value,
			domain:    domain,
			path:      path,
			hostOnly:  hostOnly,
			expiresAt: expiresAt,
			createdAt: now,
		}
		j.evictOverflowLocked()
	}
}

func (j *jar) Cookies(rawURL *url.URL) []*http.Cookie {
	if j == nil || j.store == nil || !allowedURL(rawURL) {
		return nil
	}
	host := canonicalHost(rawURL)
	requestPath := rawURL.Path
	if requestPath == "" {
		requestPath = "/"
	}
	now := time.Now()
	j.store.mu.Lock()
	defer j.store.mu.Unlock()
	matching := make([]storedCookie, 0, len(j.store.cookies))
	for key, value := range j.store.cookies {
		if !value.expiresAt.IsZero() && !value.expiresAt.After(now) {
			delete(j.store.cookies, key)
			j.store.expired++
			continue
		}
		if !cookieDomainMatches(host, value.domain, value.hostOnly) || !cookiePathMatches(requestPath, value.path) {
			continue
		}
		matching = append(matching, value)
	}
	sort.SliceStable(matching, func(i, k int) bool {
		if len(matching[i].path) != len(matching[k].path) {
			return len(matching[i].path) > len(matching[k].path)
		}
		if matching[i].name != matching[k].name {
			return matching[i].name < matching[k].name
		}
		return matching[i].domain < matching[k].domain
	})
	result := make([]*http.Cookie, 0, len(matching))
	for _, value := range matching {
		result = append(result, &http.Cookie{Name: value.name, Value: value.value})
	}
	return result
}

func (j *jar) evictOverflowLocked() {
	if j == nil || j.store == nil || len(j.store.cookies) <= maxStoredCookies {
		return
	}
	for len(j.store.cookies) > maxStoredCookies {
		var oldestKey string
		var oldest time.Time
		for key, cookie := range j.store.cookies {
			if oldestKey == "" || cookie.createdAt.Before(oldest) {
				oldestKey, oldest = key, cookie.createdAt
			}
		}
		if oldestKey == "" {
			break
		}
		delete(j.store.cookies, oldestKey)
		j.store.expired++
	}
}

// Stats returns counts only; cookie names and values are never exposed.
func (j *jar) Stats() Stats {
	if j == nil || j.store == nil {
		return Stats{}
	}
	j.store.mu.Lock()
	defer j.store.mu.Unlock()
	return Stats{Stored: len(j.store.cookies), Rejected: j.store.rejected, Expired: j.store.expired}
}

func (j *jar) Clear() {
	if j == nil || j.store == nil {
		return
	}
	j.store.mu.Lock()
	j.store.cookies = make(map[string]storedCookie)
	j.store.mu.Unlock()
}

func cookieKey(domain, path, name string) string {
	return domain + "\x00" + path + "\x00" + name
}

func canonicalHost(rawURL *url.URL) string {
	if rawURL == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSuffix(rawURL.Hostname(), "."))
}

func cookieDomain(rawDomain, host string) (domain string, hostOnly bool, ok bool) {
	domain = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(rawDomain), "."))
	domain = strings.TrimPrefix(domain, ".")
	if domain == "" {
		return host, true, host != ""
	}
	if host == "" || !cookieDomainMatches(host, domain, false) || !allowedHost(domain) {
		return "", false, false
	}
	return domain, false, true
}

func cookieDomainMatches(host, domain string, hostOnly bool) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	domain = strings.ToLower(strings.TrimPrefix(strings.TrimSuffix(domain, "."), "."))
	if host == "" || domain == "" {
		return false
	}
	if hostOnly {
		return host == domain
	}
	return host == domain || strings.HasSuffix(host, "."+domain)
}

func defaultCookiePath(requestPath string) string {
	if requestPath == "" || requestPath[0] != '/' {
		return "/"
	}
	index := strings.LastIndexByte(requestPath, '/')
	if index <= 0 {
		return "/"
	}
	return requestPath[:index]
}

func cookiePathMatches(requestPath, cookiePath string) bool {
	if requestPath == "" {
		requestPath = "/"
	}
	if cookiePath == "" || cookiePath[0] != '/' {
		cookiePath = "/"
	}
	if cookiePath == "/" {
		return true
	}
	if requestPath == cookiePath {
		return true
	}
	return strings.HasPrefix(requestPath, cookiePath) && requestPath[len(cookiePath)] == '/'
}

func allowedURL(rawURL *url.URL) bool {
	if rawURL == nil || !strings.EqualFold(rawURL.Scheme, "https") {
		return false
	}
	return allowedHost(canonicalHost(rawURL))
}

func allowedHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if host == "chatgpt.com" || host == "chat.openai.com" || host == "chatgpt-staging.com" {
		return true
	}
	return strings.HasSuffix(host, ".chatgpt.com") || strings.HasSuffix(host, ".chatgpt-staging.com")
}

// AllowedName reports whether a cookie is a Cloudflare infrastructure cookie.
func AllowedName(name string) bool {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "cf_chl_") {
		return true
	}
	switch name {
	case "__cf_bm", "__cflb", "__cfruid", "__cfseq", "__cfwaitingroom", "_cfuvid", "cf_clearance", "cf_ob_info", "cf_use_ob":
		return true
	default:
		return false
	}
}

func resetForTest() {
	globalStore.mu.Lock()
	globalStore.cookies = make(map[string]storedCookie)
	globalStore.rejected = 0
	globalStore.expired = 0
	globalStore.mu.Unlock()
}
