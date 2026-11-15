package web

import (
	"fmt"
	"math"
	"strings"

	"artdream/internal/store"

	"github.com/gin-gonic/gin"
)

const (
	// strategyAHP derives normalized priority weights from the pairwise vote
	// stream. It is the default because vote modes are AHP-style comparisons.
	strategyAHP = "ahp"
	// strategyLegacy keeps the original additive score/win-rate aggregation.
	strategyLegacy = "legacy"

	// ahpMaxRatio is the top of the Saaty 1-9 pairwise comparison scale. Vote
	// weights (0..1) are mapped linearly onto 1..9, so a tie is 1 and a
	// maximum-confidence win is 9.
	ahpMaxRatio = 9.0
)

// rankingStrategy turns a vote stream into per-image ranking statistics.
// Implementations are stateless and safe for concurrent requests.
type rankingStrategy interface {
	Name() string
	Compute(votes []store.Vote, images map[string]*store.Image, skip func(*store.Image) bool) ([]*rankStats, int)
}

// resolveRankingStrategy reads ?strategy= (or ?rankingStrategy=) and defaults
// to AHP. Unknown strategies are rejected so clients cannot silently get a
// different ranking than they asked for.
func resolveRankingStrategy(c *gin.Context) (rankingStrategy, error) {
	return resolveRankingStrategyName(firstNonEmpty(
		c.Query("strategy"),
		c.Query("rankingStrategy"),
	))
}

func resolveRankingStrategyName(name string) (rankingStrategy, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", strategyAHP, "ahp-v1", "saaty", "default":
		return ahpRankingStrategy{}, nil
	case strategyLegacy, "additive", "score":
		return legacyRankingStrategy{}, nil
	default:
		return nil, fmt.Errorf("strategy must be one of ahp, legacy")
	}
}

// --- legacy additive strategy --------------------------------------------

type legacyRankingStrategy struct{}

func (legacyRankingStrategy) Name() string { return strategyLegacy }

func (legacyRankingStrategy) Compute(votes []store.Vote, images map[string]*store.Image, skip func(*store.Image) bool) ([]*rankStats, int) {
	return computeRankStats(votes, images, skip)
}

// --- AHP strategy --------------------------------------------------------

type ahpRankingStrategy struct{}

func (ahpRankingStrategy) Name() string { return strategyAHP }

// Compute builds an (implicit) pairwise comparison matrix from all votes and
// derives the AHP priority vector with the geometric-mean method.
//
// For every counted vote the winner/loser relationship contributes a Saaty
// ratio of 1+8*weight. Multiple judgments about the same pair are aggregated
// with their geometric mean. Pairs without a direct vote are treated as
// neutral (ratio 1), which makes the matrix complete without allocating an
// N×N grid: only observed pairs are stored.
//
// The priority vector is the normalized geometric mean of each row:
//
//	log(w_i) = (1/N) * sum_j log(a_ij)
//
// Exponents are normalized with a max-shift to avoid overflow/underflow.
func (ahpRankingStrategy) Compute(votes []store.Vote, images map[string]*store.Image, skip func(*store.Image) bool) ([]*rankStats, int) {
	byID := make(map[string]*rankStats, len(images))
	ensure := func(img *store.Image) *rankStats {
		st := byID[img.ID]
		if st == nil {
			st = &rankStats{image: img}
			byID[img.ID] = st
		}
		return st
	}

	type pairKey struct{ low, high string }
	type pairAgg struct {
		sumLog float64
		count  int
	}
	pairs := make(map[pairKey]*pairAgg)
	totalVotes := 0

	for i := range votes {
		v := &votes[i]
		left, right := images[v.LeftImageID], images[v.RightImageID]
		if left == nil || right == nil {
			continue // image deleted: its history no longer counts
		}
		if skip != nil && (skip(left) || skip(right)) {
			continue // hidden by policy/exclude filters
		}
		totalVotes++

		ls, rs := ensure(left), ensure(right)
		ls.votes++
		rs.votes++

		low, high := left.ID, right.ID
		if low > high {
			low, high = high, low
		}
		key := pairKey{low: low, high: high}
		agg := pairs[key]
		if agg == nil {
			agg = &pairAgg{}
			pairs[key] = agg
		}
		agg.count++

		switch {
		case v.WinnerImageID == nil:
			ls.ties++
			rs.ties++
			// A tie is a neutral 1:1 judgment; log(1) is zero.
		case *v.WinnerImageID == left.ID:
			ls.wins++
			rs.losses++
			if left.ID == low {
				agg.sumLog += ahpLogRatio(v.Weight)
			} else {
				agg.sumLog -= ahpLogRatio(v.Weight)
			}
		default:
			rs.wins++
			ls.losses++
			if right.ID == low {
				agg.sumLog += ahpLogRatio(v.Weight)
			} else {
				agg.sumLog -= ahpLogRatio(v.Weight)
			}
		}
	}

	out := make([]*rankStats, 0, len(byID))
	if len(byID) == 0 {
		return out, totalVotes
	}

	rowLogSums := make(map[string]float64, len(byID))
	for key, agg := range pairs {
		if agg.count == 0 {
			continue
		}
		meanLog := agg.sumLog / float64(agg.count)
		rowLogSums[key.low] += meanLog
		rowLogSums[key.high] -= meanLog
	}

	n := float64(len(byID))
	logWeights := make(map[string]float64, len(byID))
	maxLog := math.Inf(-1)
	for id := range byID {
		lw := rowLogSums[id] / n
		logWeights[id] = lw
		if lw > maxLog {
			maxLog = lw
		}
	}

	total := 0.0
	weights := make(map[string]float64, len(byID))
	for id, lw := range logWeights {
		w := math.Exp(lw - maxLog)
		weights[id] = w
		total += w
	}

	for id, st := range byID {
		if total > 0 {
			st.score = weights[id] / total
		}
		out = append(out, st)
	}
	return out, totalVotes
}

// ahpLogRatio maps a normalized vote weight (0..1) onto the Saaty 1-9 scale
// and returns its natural logarithm.
func ahpLogRatio(weight float64) float64 {
	if weight < 0 {
		weight = 0
	}
	if weight > 1 {
		weight = 1
	}
	return math.Log(1 + (ahpMaxRatio-1)*weight)
}
