package web

import (
	"math"
	"testing"

	"artdream/internal/store"
)

func strategyImages(ids ...string) map[string]*store.Image {
	images := make(map[string]*store.Image, len(ids))
	for _, id := range ids {
		images[id] = &store.Image{ID: id}
	}
	return images
}

func winnerVote(left, right, winner string, weight float64) store.Vote {
	id := winner
	return store.Vote{
		ID:            1,
		LeftImageID:   left,
		RightImageID:  right,
		WinnerImageID: &id,
		Weight:        weight,
		MinValue:      1,
		MaxValue:      5,
	}
}

func tieVote(left, right string) store.Vote {
	return store.Vote{
		ID:           1,
		LeftImageID:  left,
		RightImageID: right,
		Weight:       0,
		MinValue:     1,
		MaxValue:     5,
	}
}

func statsByID(stats []*rankStats) map[string]*rankStats {
	out := make(map[string]*rankStats, len(stats))
	for _, st := range stats {
		out[st.image.ID] = st
	}
	return out
}

func TestResolveRankingStrategy(t *testing.T) {
	for _, name := range []string{"", "ahp", "AHP", "ahp-v1", "saaty", "default"} {
		strategy, err := resolveRankingStrategyName(name)
		if err != nil {
			t.Fatalf("resolve(%q): %v", name, err)
		}
		if strategy.Name() != strategyAHP {
			t.Fatalf("resolve(%q).Name() = %q, want %q", name, strategy.Name(), strategyAHP)
		}
	}
	for _, name := range []string{"legacy", "additive", "score"} {
		strategy, err := resolveRankingStrategyName(name)
		if err != nil {
			t.Fatalf("resolve(%q): %v", name, err)
		}
		if strategy.Name() != strategyLegacy {
			t.Fatalf("resolve(%q).Name() = %q, want %q", name, strategy.Name(), strategyLegacy)
		}
	}
	if _, err := resolveRankingStrategyName("elo"); err == nil {
		t.Fatal("expected an error for an unknown strategy")
	}
}

func TestAHPPriorityVectorIsNormalizedAndWinnerRanksFirst(t *testing.T) {
	strategy := ahpRankingStrategy{}
	votes := []store.Vote{
		winnerVote("a", "b", "a", 1),
		winnerVote("a", "c", "a", 1),
		winnerVote("b", "c", "b", 1),
	}
	stats, totalVotes := strategy.Compute(votes, strategyImages("a", "b", "c"), nil)
	if totalVotes != 3 {
		t.Fatalf("totalVotes = %d, want 3", totalVotes)
	}
	byID := statsByID(stats)
	sum := 0.0
	for _, st := range stats {
		sum += st.score
	}
	if math.Abs(sum-1) > 1e-9 {
		t.Fatalf("sum of AHP scores = %.12f, want 1", sum)
	}
	if !(byID["a"].score > byID["b"].score && byID["b"].score > byID["c"].score) {
		t.Fatalf("AHP order not a > b > c: %.6f %.6f %.6f",
			byID["a"].score, byID["b"].score, byID["c"].score)
	}
}

func TestAHPTiesProduceEqualPriorities(t *testing.T) {
	strategy := ahpRankingStrategy{}
	stats, totalVotes := strategy.Compute(
		[]store.Vote{tieVote("a", "b")},
		strategyImages("a", "b"),
		nil,
	)
	if totalVotes != 1 {
		t.Fatalf("totalVotes = %d, want 1", totalVotes)
	}
	byID := statsByID(stats)
	if math.Abs(byID["a"].score-byID["b"].score) > 1e-9 {
		t.Fatalf("tie priorities differ: %.12f vs %.12f", byID["a"].score, byID["b"].score)
	}
	if math.Abs(byID["a"].score-0.5) > 1e-9 {
		t.Fatalf("tie priority = %.12f, want 0.5", byID["a"].score)
	}
}

func TestAHPMaximumConfidenceWinMatchesSaatyNineToOne(t *testing.T) {
	strategy := ahpRankingStrategy{}
	stats, _ := strategy.Compute(
		[]store.Vote{winnerVote("a", "b", "a", 1)},
		strategyImages("a", "b"),
		nil,
	)
	byID := statsByID(stats)
	if got, want := byID["a"].score, 0.9; math.Abs(got-want) > 1e-9 {
		t.Fatalf("winner priority = %.12f, want %.12f", got, want)
	}
	if got, want := byID["b"].score, 0.1; math.Abs(got-want) > 1e-9 {
		t.Fatalf("loser priority = %.12f, want %.12f", got, want)
	}
}

func TestAHPSkippedImageDoesNotEnterTheMatrix(t *testing.T) {
	strategy := ahpRankingStrategy{}
	skipB := func(img *store.Image) bool { return img.ID == "b" }
	stats, totalVotes := strategy.Compute(
		[]store.Vote{winnerVote("a", "b", "a", 1)},
		strategyImages("a", "b"),
		skipB,
	)
	if totalVotes != 0 {
		t.Fatalf("totalVotes = %d, want 0", totalVotes)
	}
	if len(stats) != 0 {
		t.Fatalf("len(stats) = %d, want 0", len(stats))
	}
}

func TestLegacyStrategyKeepsAdditiveScores(t *testing.T) {
	strategy := legacyRankingStrategy{}
	stats, _ := strategy.Compute(
		[]store.Vote{winnerVote("a", "b", "a", 1)},
		strategyImages("a", "b"),
		nil,
	)
	byID := statsByID(stats)
	if byID["a"].score != 1 || byID["b"].score != 0 {
		t.Fatalf("legacy scores = %.3f / %.3f, want 1 / 0", byID["a"].score, byID["b"].score)
	}
}
