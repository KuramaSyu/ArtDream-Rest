package web

import (
	"testing"
	"time"

	"artdream/internal/store"
)

func TestRankingCacheReturnsDefensiveCopies(t *testing.T) {
	now := time.Unix(1000, 0)
	cache := newRankingCacheWith(func() time.Time { return now }, time.Second, 4)

	entries := []rankEntry{{ID: "a", Rank: 1, Score: 0.7}}
	cache.Set("key", entries, 3)
	entries[0].Rank = 99 // mutating the caller's slice must not affect the cache

	got, totalVotes, ok := cache.Get("key")
	if !ok {
		t.Fatal("expected a cache hit")
	}
	if totalVotes != 3 || len(got) != 1 || got[0].ID != "a" || got[0].Rank != 1 || got[0].Score != 0.7 {
		t.Fatalf("unexpected cached entry: %+v total=%d", got, totalVotes)
	}

	got[0].Rank = 42 // mutating the returned slice must not affect the cache either
	again, _, ok := cache.Get("key")
	if !ok || again[0].Rank != 1 {
		t.Fatalf("cache entry was mutated through Get: %+v", again)
	}
}

func TestRankingCacheExpires(t *testing.T) {
	now := time.Unix(1000, 0)
	cache := newRankingCacheWith(func() time.Time { return now }, time.Second, 4)
	cache.Set("key", []rankEntry{{ID: "a"}}, 1)

	now = now.Add(500 * time.Millisecond)
	if _, _, ok := cache.Get("key"); !ok {
		t.Fatal("entry expired too early")
	}
	now = now.Add(600 * time.Millisecond)
	if _, _, ok := cache.Get("key"); ok {
		t.Fatal("entry should have expired")
	}
}

func TestRankingCacheEvictsOldestEntries(t *testing.T) {
	now := time.Unix(1000, 0)
	cache := newRankingCacheWith(func() time.Time { return now }, time.Hour, 2)
	cache.Set("a", []rankEntry{{ID: "a"}}, 1)
	cache.Set("b", []rankEntry{{ID: "b"}}, 1)
	cache.Set("c", []rankEntry{{ID: "c"}}, 1)

	if _, _, ok := cache.Get("a"); ok {
		t.Fatal("oldest entry should have been evicted")
	}
	if _, _, ok := cache.Get("b"); !ok {
		t.Fatal("entry b should still be cached")
	}
	if _, _, ok := cache.Get("c"); !ok {
		t.Fatal("entry c should still be cached")
	}
}

func TestRankingCacheInvalidate(t *testing.T) {
	cache := newRankingCache()
	cache.Set("key", []rankEntry{{ID: "a"}}, 1)
	cache.Invalidate()
	if _, _, ok := cache.Get("key"); ok {
		t.Fatal("Invalidate should clear cached rankings")
	}
}

func TestRankingCacheKeyStableForRollingWindows(t *testing.T) {
	from1, to1 := int64(1000), int64(2000)
	from2, to2 := int64(1001), int64(2001)
	policy := &store.VisibilityPolicy{}

	first := rankingCacheKey(
		ahpRankingStrategy{},
		rangeWindow{Label: "day", From: &from1, To: &to1},
		store.VoteFilter{},
		policy, nil, "day", nil,
	)
	second := rankingCacheKey(
		ahpRankingStrategy{},
		rangeWindow{Label: "day", From: &from2, To: &to2},
		store.VoteFilter{},
		policy, nil, "day", nil,
	)
	if first != second {
		t.Fatal("rolling windows should share a cache key")
	}

	custom := rankingCacheKey(
		ahpRankingStrategy{},
		rangeWindow{Label: "custom", From: &from2, To: &to2},
		store.VoteFilter{},
		policy, nil, "all", nil,
	)
	if rankingCacheKey(
		ahpRankingStrategy{},
		rangeWindow{Label: "custom", From: &from1, To: &to1},
		store.VoteFilter{},
		policy, nil, "all", nil,
	) == custom {
		t.Fatal("explicit custom ranges must not share a cache key")
	}
}
