package web

import (
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"artdream/internal/store"
)

const (
	// A short TTL keeps rankings fresh while still collapsing the repeated
	// window scans of /images/rank, /images/smart and rank rotation.
	defaultRankingCacheTTL = 30 * time.Second
	// Bound both the number of query variants and the total number of cached
	// rank entries so the cache cannot grow without limit on big instances.
	defaultRankingCacheMaxKey = 32
	maxCachedRankEntries      = 40000
)

type rankingCacheEntry struct {
	entries    []rankEntry
	totalVotes int
	expiresAt  time.Time
}

// rankingCache stores the fully sorted entries for one
// (strategy, window, filter, policy, moderation, exclude) combination.
// Callers always receive a copy, because handlers set Rank/Rotated/Random on
// the returned entries.
type rankingCache struct {
	mu         sync.RWMutex
	entries    map[string]rankingCacheEntry
	order      []string
	ttl        time.Duration
	maxEntries int
	cachedRank int
	now        func() time.Time
}

func newRankingCache() *rankingCache {
	return newRankingCacheWith(time.Now, defaultRankingCacheTTL, defaultRankingCacheMaxKey)
}

func newRankingCacheWith(now func() time.Time, ttl time.Duration, maxEntries int) *rankingCache {
	if now == nil {
		now = time.Now
	}
	if ttl <= 0 {
		ttl = defaultRankingCacheTTL
	}
	if maxEntries <= 0 {
		maxEntries = defaultRankingCacheMaxKey
	}
	return &rankingCache{
		entries:    make(map[string]rankingCacheEntry),
		ttl:        ttl,
		maxEntries: maxEntries,
		now:        now,
	}
}

// Get returns a defensive copy of the cached entries.
func (c *rankingCache) Get(key string) ([]rankEntry, int, bool) {
	if c == nil {
		return nil, 0, false
	}
	c.mu.RLock()
	entry, ok := c.entries[key]
	expired := ok && !c.now().Before(entry.expiresAt)
	c.mu.RUnlock()
	if !ok || expired {
		return nil, 0, false
	}
	entries := make([]rankEntry, len(entry.entries))
	copy(entries, entry.entries)
	return entries, entry.totalVotes, true
}

// Set stores a defensive copy of the entries.
func (c *rankingCache) Set(key string, entries []rankEntry, totalVotes int) {
	if c == nil {
		return
	}
	stored := make([]rankEntry, len(entries))
	copy(stored, entries)
	if len(stored) > maxCachedRankEntries {
		return // do not let one huge result evict every other query
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	for existing, entry := range c.entries {
		if !now.Before(entry.expiresAt) {
			delete(c.entries, existing)
			c.cachedRank -= len(entry.entries)
		}
	}
	if previous, exists := c.entries[key]; exists {
		c.cachedRank -= len(previous.entries)
	} else {
		c.order = append(c.order, key)
	}
	c.entries[key] = rankingCacheEntry{
		entries:    stored,
		totalVotes: totalVotes,
		expiresAt:  now.Add(c.ttl),
	}
	c.cachedRank += len(stored)

	for (len(c.entries) > c.maxEntries || c.cachedRank > maxCachedRankEntries) && len(c.order) > 0 {
		oldest := c.order[0]
		c.order = c.order[1:]
		if entry, exists := c.entries[oldest]; exists {
			delete(c.entries, oldest)
			c.cachedRank -= len(entry.entries)
		}
	}
	if c.cachedRank < 0 {
		c.cachedRank = 0
	}
}

// Invalidate clears every cached ranking. Called after vote/image mutations.
func (c *rankingCache) Invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]rankingCacheEntry)
	c.order = nil
	c.cachedRank = 0
}

func (s *Server) invalidateRankingCache() {
	if s.rankCache != nil {
		s.rankCache.Invalidate()
	}
}

// rankingCacheKey captures every input that can change the computed entries:
// strategy, window bounds, vote filters, the compiled visibility policy,
// moderation state and excluded image ids. Length prefixes avoid accidental
// separator collisions in user-supplied regexes/ids.
func rankingCacheKey(strategy rankingStrategy, rng rangeWindow, filter store.VoteFilter, policy *store.VisibilityPolicy, modRows []store.ImageModeration, window string, exclude map[string]struct{}) string {
	var b strings.Builder
	write := func(key, value string) {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(strconv.Itoa(len(value)))
		b.WriteByte(':')
		b.WriteString(value)
		b.WriteByte(';')
	}

	write("strategy", strategy.Name())
	write("label", rng.Label)
	// Rolling windows move every second; keying them by exact bounds would
	// defeat the cache. The TTL is the freshness bound for day/week/month/
	// year/all. Explicit custom ranges keep their exact bounds.
	fromKey, toKey := "", ""
	if rng.Label == "custom" {
		fromKey = formatUnixPtr(rng.From)
		toKey = formatUnixPtr(rng.To)
	}
	write("from", fromKey)
	write("to", toKey)
	write("mode", filter.Mode)
	write("author", filter.Author)
	write("titleRegex", filter.TitleRegex)
	write("user", filter.Username)
	write("image", filter.ImageID)
	write("policy", policy.Fingerprint())
	write("window", window)

	excluded := make([]string, 0, len(exclude))
	for id := range exclude {
		excluded = append(excluded, id)
	}
	sort.Strings(excluded)
	write("exclude", strings.Join(excluded, ","))

	moderation := make([]string, 0, len(modRows))
	for _, row := range modRows {
		moderation = append(moderation, strings.Join([]string{
			row.ImageID,
			row.TimeWindow,
			strconv.FormatBool(row.Hidden),
			strconv.FormatBool(row.Deranked),
		}, "|"))
	}
	sort.Strings(moderation)
	write("moderation", strings.Join(moderation, "\n"))

	return b.String()
}

func formatUnixPtr(v *int64) string {
	if v == nil {
		return ""
	}
	return strconv.FormatInt(*v, 10)
}
