package web

import (
	"net/http/httptest"
	"testing"
	"time"

	"artdream/internal/store"

	"github.com/gin-gonic/gin"
)

func exploreEntry(id string, score float64) rankEntry {
	return rankEntry{ID: id, Title: id, Score: score}
}

func exploreImage(id string, created time.Time) store.Image {
	return store.Image{ID: id, Title: id, CreatedAt: created}
}

func TestExploreBoostRewardsFreshAndUnderVotedImages(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	fresh := exploreImage("fresh", now.Add(-time.Hour))
	stale := exploreImage("stale", now.Add(-30*24*time.Hour))

	brandNew := exploreBoost(&fresh, 0, now)
	if mathAbs(brandNew-2.0) > 0.01 {
		t.Fatalf("brand-new, unvoted image should reach the maximum boost 2, got %v", brandNew)
	}
	if ratedOld := exploreBoost(&stale, 50, now); mathAbs(ratedOld-1) > 0.001 {
		t.Fatalf("old, well-voted image should not be boosted, got %v", ratedOld)
	}
	// A fresh image that already collected votes keeps the freshness signal.
	freshRated := exploreBoost(&fresh, exploreVoteSaturation, now)
	if !(freshRated > 1 && freshRated < brandNew) {
		t.Fatalf("fresh rated image should be boosted less than an unvoted one, got %v", freshRated)
	}
	// An old image without votes still gets the under-voted signal.
	unratedOld := exploreBoost(&stale, 0, now)
	if !(unratedOld > 1 && unratedOld < freshRated) {
		t.Fatalf("old unvoted image should only keep the under-voted share, got %v", unratedOld)
	}
	// Future timestamps must not produce boosts above the cap.
	future := exploreImage("future", now.Add(24*time.Hour))
	if got := exploreBoost(&future, 0, now); mathAbs(got-2.0) > 0.001 {
		t.Fatalf("future created_at should clamp to full freshness, got %v", got)
	}
}

func TestInjectExploreEntriesAddsOnlyNewUnratedImages(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	entries := []rankEntry{
		exploreEntry("winner", 0.5),
		exploreEntry("runner", 0.2),
		exploreEntry("third", 0.1),
	}
	plan := &explorePlan{
		Now:     now,
		Enabled: true,
		Candidates: []store.Image{
			exploreImage("winner", now.Add(-time.Hour)), // already ranked
			exploreImage("stale", now.Add(-30*24*time.Hour)),
			exploreImage("fresh", now.Add(-time.Hour)),
		},
	}

	out := injectExploreEntries(append([]rankEntry(nil), entries...), plan)
	if len(out) != 4 {
		t.Fatalf("expected exactly one injected image, got %d entries", len(out))
	}
	injected := map[string]rankEntry{}
	for _, entry := range out {
		if entry.Explore {
			injected[entry.ID] = entry
		}
	}
	if _, ok := injected["fresh"]; !ok {
		t.Fatal("fresh unrated image should have been injected")
	}
	if _, ok := injected["stale"]; ok {
		t.Fatal("old image must not be injected")
	}
	if _, ok := injected["winner"]; ok {
		t.Fatal("already ranked image must not be injected twice")
	}
	if got := injected["fresh"].orderKey; got <= 0 {
		t.Fatalf("injected entry needs an ordering key, got %v", got)
	}
	if injected["fresh"].Score != 0 {
		t.Fatalf("injected entry must keep its real score 0, got %v", injected["fresh"].Score)
	}
}

func TestInjectExploreEntriesRespectsTodayFilterAndLimit(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	entries := make([]rankEntry, 40)
	for i := range entries {
		entries[i] = exploreEntry("ranked", 1)
	}
	entries[0].ID = "ranked-0"
	candidates := make([]store.Image, 0, 40)
	for i := 0; i < 40; i++ {
		candidates = append(candidates, exploreImage("new-"+string(rune('a'+i%26)), now.Add(-time.Duration(i)*time.Minute)))
	}
	plan := &explorePlan{
		Now:         now,
		Enabled:     true,
		CreatedFrom: todayStart(now),
		Candidates:  candidates,
	}
	out := injectExploreEntries(entries, plan)
	if len(out)-len(entries) > exploreMaxInject {
		t.Fatalf("injection limit exceeded: %d added", len(out)-len(entries))
	}
	// Every injected image is uploaded today, so the today filter lets them in.
	for _, entry := range out {
		if entry.Explore && entry.Image.CreatedAt.Before(plan.CreatedFrom) {
			t.Fatal("injected image is older than the day boundary")
		}
	}

	// With an empty ranking nothing is injected: the smart ladder keeps its
	// existing fallback semantics.
	if out := injectExploreEntries(nil, plan); len(out) != 0 {
		t.Fatalf("empty ranking must stay empty, got %d entries", len(out))
	}
}

func TestSortRankEntriesUsesExplorationOrderKey(t *testing.T) {
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	rated := rankEntry{ID: "rated", Score: 0.3}
	fresh := rankEntryFor(&rankStats{image: &store.Image{ID: "fresh", CreatedAt: now.Add(-time.Hour)}})
	fresh.Explore = true
	applyExploreBoost(&fresh, &store.Image{ID: "fresh", CreatedAt: now.Add(-time.Hour)}, 0, &explorePlan{Now: now, Enabled: true})
	fresh.orderKey = 0.2 * exploreBoost(&store.Image{ID: "fresh", CreatedAt: now.Add(-time.Hour)}, 0, now)

	entries := []rankEntry{rated, fresh}
	sortRankEntries(entries)
	if entries[0].ID != "fresh" {
		t.Fatalf("boosted fresh image should sort first, got %s", entries[0].ID)
	}
	if got := rankOrderKey(rated); got != 0.3 {
		t.Fatalf("entries without an order key must fall back to the score, got %v", got)
	}
}

func TestResolveExploreDefaultsToEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cases := []struct {
		query string
		on    bool
	}{
		{"", true},
		{"?explore=false", false},
		{"?preferNew=0", false},
		{"?exploreNew=no", false},
		{"?preferFresh=on", true},
	}
	for _, tc := range cases {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/images/rank"+tc.query, nil)
		got, err := resolveExplore(c)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", tc.query, err)
		}
		if got != tc.on {
			t.Fatalf("%s: expected explore=%v, got %v", tc.query, tc.on, got)
		}
	}

	bad, _ := gin.CreateTestContext(httptest.NewRecorder())
	bad.Request = httptest.NewRequest("GET", "/images/rank?explore=maybe", nil)
	if _, err := resolveExplore(bad); err == nil {
		t.Fatal("invalid boolean must be rejected")
	}
}

func TestResolveTodayOnlyDefaultsToDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, query := range []string{"", "?todayOnly=false", "?onlyToday=0", "?today=off"} {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest("GET", "/images/rank"+query, nil)
		got, err := resolveTodayOnly(c)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", query, err)
		}
		if got {
			t.Fatalf("%s: todayOnly must default to false", query)
		}
	}
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest("GET", "/images/rank?todayOnly=true", nil)
	if got, err := resolveTodayOnly(c); err != nil || !got {
		t.Fatalf("todayOnly=true should enable the filter, got %v err %v", got, err)
	}
}

func TestTodayStartIsUTCMidnight(t *testing.T) {
	now := time.Date(2026, 1, 10, 23, 59, 0, 0, time.FixedZone("CET", 3600))
	start := todayStart(now)
	want := time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC)
	if !start.Equal(want) {
		t.Fatalf("expected %v, got %v", want, start)
	}
}

func TestExplorePlanCacheKeyTracksScopeAndTodayFilter(t *testing.T) {
	if got := (*explorePlan)(nil).cacheKey(); got != "off" {
		t.Fatalf("nil plan should disable exploration, got %q", got)
	}
	now := time.Date(2026, 1, 10, 12, 0, 0, 0, time.UTC)
	base := &explorePlan{Now: now, Scope: "all", Enabled: true}
	// Freshness decays with the clock, but not fast enough to warrant a new
	// cache key; the cache TTL bounds the staleness instead.
	if same := (&explorePlan{Now: now.Add(time.Second), Scope: "all", Enabled: true}); base.cacheKey() != same.cacheKey() {
		t.Fatal("plans with the same scope should share a cache key")
	}
	if base.cacheKey() == (*explorePlan)(nil).cacheKey() {
		t.Fatal("exploration must not share a key with the disabled plan")
	}
	// /images/rank and /images/smart filter candidates by different moderation
	// windows, so their pools must not share cached entries.
	if day := (&explorePlan{Now: now, Scope: "day", Enabled: true}); base.cacheKey() == day.cacheKey() {
		t.Fatal("different scopes must not share a cache key")
	}
	withToday := &explorePlan{Now: now, Scope: "all", Enabled: true, CreatedFrom: todayStart(now)}
	if base.cacheKey() == withToday.cacheKey() {
		t.Fatal("today filter must change the cache key")
	}
	// Today-only without exploration must not reuse boosted entries.
	offToday := &explorePlan{Now: now, Scope: "all", CreatedFrom: todayStart(now)}
	if offToday.cacheKey() == withToday.cacheKey() {
		t.Fatal("exploration switch must change the cache key")
	}
	if entries := injectExploreEntries([]rankEntry{exploreEntry("ranked", 1)}, offToday); len(entries) != 1 {
		t.Fatal("a today-only plan without exploration must not inject entries")
	}
}

func TestExploreInjectLimit(t *testing.T) {
	if got := exploreInjectLimit(0); got != 1 {
		t.Fatalf("expected a floor of 1, got %d", got)
	}
	if got := exploreInjectLimit(9); got != 3 {
		t.Fatalf("expected a third of the ranking, got %d", got)
	}
	if got := exploreInjectLimit(10_000); got != exploreMaxInject {
		t.Fatalf("expected the cap %d, got %d", exploreMaxInject, got)
	}
}

func mathAbs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
