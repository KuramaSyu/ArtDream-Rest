package web

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"artdream/internal/store"

	"github.com/gin-gonic/gin"
)

const (
	// exploreFreshWindow is how long an upload still counts as new. Freshness
	// decays linearly from 1 at upload time to 0 at this age.
	exploreFreshWindow = 7 * 24 * time.Hour
	// exploreVoteSaturation is the number of votes inside the requested window
	// at which an image counts as fully rated and stops receiving a boost.
	exploreVoteSaturation = 10
	// exploreFreshWeight and exploreUnratedWeight split the maximum boost
	// between both signals; together they cap the factor at 2 (double weight
	// for a brand-new image that nobody has voted on yet).
	exploreFreshWeight   = 0.6
	exploreUnratedWeight = 0.4
	// exploreMaxInject caps how many unrated images one ranking may gain.
	exploreMaxInject = 12
)

// explorePlan carries the exploration inputs of one ranking request. A nil
// plan disables exploration entirely.
type explorePlan struct {
	// Candidates are the library images matching the request filters. Only
	// new images without votes in the window are injected into the ranking.
	// They are filled lazily by Load so a cached ranking never re-queries the
	// library.
	Candidates []store.Image
	// Enabled reports whether exploration weights and injected entries are
	// wanted; todayOnly can create a plan with exploration switched off.
	Enabled bool
	// Load fills Candidates on the first ranking cache miss of a request.
	Load func() ([]store.Image, error)
	// Scope distinguishes candidate sets that share a cache key, such as the
	// per-period moderation window of /images/rank and the conservative "all"
	// window of /images/smart.
	Scope string
	// CreatedFrom drops images (and votes about them) uploaded before this
	// instant; it implements ?todayOnly. The zero value disables the filter.
	CreatedFrom time.Time
	// Now anchors the freshness decay so every window of one request agrees.
	Now time.Time
}

// createdFrom returns the upload cutoff of the plan; zero when disabled.
func (p *explorePlan) createdFrom() time.Time {
	if p == nil {
		return time.Time{}
	}
	return p.CreatedFrom
}

// resolveExplore reads ?explore= (default true) plus its aliases. Exploration
// is on unless the client opts out, so quiet leaderboards keep giving new and
// under-voted images a chance to be scored.
func resolveExplore(c *gin.Context) (bool, error) {
	for _, key := range []string{"explore", "preferNew", "preferFresh", "exploreNew"} {
		raw := strings.TrimSpace(c.Query(key))
		if raw == "" {
			continue
		}
		enabled, ok := parseOptionalBool(raw)
		if !ok {
			return false, fmt.Errorf("%s must be a boolean", key)
		}
		return enabled, nil
	}
	return true, nil
}

// resolveTodayOnly reads ?todayOnly= (default false) plus its aliases. When
// enabled, only images uploaded today (UTC) are ranked.
func resolveTodayOnly(c *gin.Context) (bool, error) {
	for _, key := range []string{"todayOnly", "onlyToday", "today"} {
		raw := strings.TrimSpace(c.Query(key))
		if raw == "" {
			continue
		}
		enabled, ok := parseOptionalBool(raw)
		if !ok {
			return false, fmt.Errorf("%s must be a boolean", key)
		}
		return enabled, nil
	}
	return false, nil
}

// todayStart returns 00:00 UTC of the current day. A fixed day boundary (rather
// than a rolling 24h window) keeps the ranking cache key stable for a day.
func todayStart(now time.Time) time.Time {
	return now.UTC().Truncate(24 * time.Hour)
}

// exploreIsFresh reports whether an upload is still inside the freshness
// window, i.e. whether it counts as "new" for exploration slots.
func exploreIsFresh(img *store.Image, now time.Time) bool {
	if img == nil || img.CreatedAt.IsZero() || now.IsZero() {
		return false
	}
	return now.Sub(img.CreatedAt) < exploreFreshWindow
}

// exploreBoost returns the multiplicative ordering weight for an image with
// the given number of votes inside the requested window. Older, well-voted
// images keep their score unchanged (factor 1); a brand-new image without any
// vote is weighted twice as strong so it can climb into the visible page.
func exploreBoost(img *store.Image, votes int, now time.Time) float64 {
	fresh := 0.0
	if img != nil && !img.CreatedAt.IsZero() && !now.IsZero() {
		age := now.Sub(img.CreatedAt)
		if age < 0 {
			age = 0
		}
		if age < exploreFreshWindow {
			fresh = 1 - float64(age)/float64(exploreFreshWindow)
		}
	}
	unrated := 0.0
	if votes < exploreVoteSaturation {
		if votes < 0 {
			votes = 0
		}
		unrated = 1 - float64(votes)/float64(exploreVoteSaturation)
	}
	return 1 + exploreFreshWeight*fresh + exploreUnratedWeight*unrated
}

// applyExploreBoost rewrites the internal ordering key of a ranked entry. The
// public Score keeps reporting the pure vote result, so clients that ignore
// exploration still see comparable numbers.
func applyExploreBoost(entry *rankEntry, img *store.Image, votes int, plan *explorePlan) {
	if plan == nil || !plan.Enabled {
		return
	}
	boost := exploreBoost(img, votes, plan.Now)
	entry.orderKey = entry.Score * boost
	if boost > 1 {
		entry.Boost = roundExplore(boost)
	}
}

// exploreInjectLimit keeps new images a minority of an assembled ranking; the
// share grows with the leaderboard so small libraries are not flooded.
func exploreInjectLimit(entries int) int {
	limit := entries / 3
	if limit < 1 {
		limit = 1
	}
	if limit > exploreMaxInject {
		limit = exploreMaxInject
	}
	return limit
}

// explorePriorScore is the score an unrated image receives before the boost:
// the mean positive score of the current leaderboard. That places injected
// images in the upper half instead of letting a single upload claim the top.
func explorePriorScore(entries []rankEntry) float64 {
	sum, count := 0.0, 0
	for i := range entries {
		if entries[i].deranked || entries[i].Score <= 0 {
			continue
		}
		sum += entries[i].Score
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / float64(count)
}

// injectExploreEntries mixes new, unvoted library images into a ranking that
// already has at least one real entry. Only images uploaded inside the
// freshness window (and after CreatedFrom when todayOnly is set) qualify.
func injectExploreEntries(entries []rankEntry, plan *explorePlan) []rankEntry {
	if plan == nil || !plan.Enabled || len(entries) == 0 || len(plan.Candidates) == 0 {
		return entries
	}
	prior := explorePriorScore(entries)
	if prior <= 0 {
		return entries
	}
	limit := exploreInjectLimit(len(entries))
	present := make(map[string]struct{}, len(entries))
	for i := range entries {
		present[entries[i].ID] = struct{}{}
	}
	injected := 0
	for i := range plan.Candidates {
		if injected >= limit {
			break
		}
		img := &plan.Candidates[i]
		if _, ranked := present[img.ID]; ranked {
			continue
		}
		if !plan.CreatedFrom.IsZero() && img.CreatedAt.Before(plan.CreatedFrom) {
			continue
		}
		if !exploreIsFresh(img, plan.Now) {
			continue // only new uploads get an unrated slot
		}
		boost := exploreBoost(img, 0, plan.Now)
		entry := rankEntryFor(&rankStats{image: img})
		entry.Explore = true
		entry.Boost = roundExplore(boost)
		entry.orderKey = prior * boost
		entries = append(entries, entry)
		present[img.ID] = struct{}{}
		injected++
	}
	return entries
}

// cacheKey fingerprints the parts of a plan that change the computed ranking.
// Now is deliberately excluded: the ranking cache TTL bounds how stale the
// freshness decay may be. The candidate library is also left out, because
// every image mutation invalidates the ranking cache; Scope keeps differing
// candidate sets (moderation windows) apart.
func (p *explorePlan) cacheKey() string {
	if p == nil {
		return "off"
	}
	created := ""
	if !p.CreatedFrom.IsZero() {
		created = strconv.FormatInt(p.CreatedFrom.Unix(), 10)
	}
	return "on:" + strconv.FormatBool(p.Enabled) + ":" + p.Scope + ":" + created
}

func roundExplore(v float64) float64 {
	return math.Round(v*1000) / 1000
}
