package web

import (
	"fmt"
	"testing"

	"artdream/internal/store"
)

func rotationEntry(id string) rankEntry {
	return rankEntry{ID: id, Title: id, Score: 1, WinRate: 1, Votes: 1}
}

func rotationPool(n int) []store.Image {
	out := make([]store.Image, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, store.Image{ID: fmt.Sprintf("p%02d", i), Author: "Ada", Title: "Pool"})
	}
	return out
}

func TestPreviousRangeWindow(t *testing.T) {
	from, to := int64(2_000_000), int64(2_086_400)
	previous, ok := previousRangeWindow(rangeWindow{Label: "day", From: &from, To: &to})
	if !ok {
		t.Fatal("expected a previous range")
	}
	if previous.From == nil || previous.To == nil {
		t.Fatal("previous range missing bounds")
	}
	if *previous.To != from-1 {
		t.Errorf("previousTo = %d, want %d", *previous.To, from-1)
	}
	if got := *previous.To - *previous.From; got != to-from {
		t.Errorf("previous duration = %d, want %d", got, to-from)
	}
	if _, ok := previousRangeWindow(rangeWindow{Label: "all"}); ok {
		t.Error("all-time range should not have a previous window")
	}
}

func TestApplyRankRotationReplacesStaleEntries(t *testing.T) {
	current := []rankEntry{
		rotationEntry("old1"), rotationEntry("old2"),
		rotationEntry("old3"), rotationEntry("old4"),
	}
	baseline := rankIDSet(current)
	final, changed, required := applyRankRotation(
		current, current, 0, 4, baseline, rotationPool(10),
		rangeWindow{Label: "day", From: int64Ptr(100), To: int64Ptr(200)}, 60,
	)
	if required != 3 {
		t.Fatalf("required = %d, want 3", required)
	}
	if changed < required {
		t.Fatalf("changed = %d, want at least %d", changed, required)
	}
	if len(final) != 4 {
		t.Fatalf("len(final) = %d, want 4", len(final))
	}
	seen := map[string]struct{}{}
	for _, entry := range final {
		if _, duplicate := seen[entry.ID]; duplicate {
			t.Fatalf("duplicate id %q", entry.ID)
		}
		seen[entry.ID] = struct{}{}
	}
	if got := countRankOutside(final, baseline); got != changed {
		t.Fatalf("countRankOutside = %d, reported %d", got, changed)
	}
}

func TestApplyRankRotationFillsEmptyPage(t *testing.T) {
	baseline := map[string]struct{}{"p00": {}, "p01": {}}
	final, changed, required := applyRankRotation(
		nil, nil, 0, 5, baseline, rotationPool(10),
		rangeWindow{Label: "day", From: int64Ptr(100), To: int64Ptr(200)}, 60,
	)
	if len(final) != 5 {
		t.Fatalf("len(final) = %d, want 5", len(final))
	}
	if required != 3 {
		t.Fatalf("required = %d, want 3", required)
	}
	if changed != 5 {
		t.Fatalf("changed = %d, want 5", changed)
	}
	for _, entry := range final {
		if _, stale := baseline[entry.ID]; stale {
			t.Errorf("final contains previous-period image %q", entry.ID)
		}
		if !entry.Rotated {
			t.Errorf("fallback entry %q should be marked rotated", entry.ID)
		}
	}
}

func TestRotationOrderIsDeterministicAndAdvances(t *testing.T) {
	pool := rotationPool(10)
	firstWindow := rangeWindow{Label: "day", From: int64Ptr(100), To: int64Ptr(200)}
	secondWindow := rangeWindow{Label: "day", From: int64Ptr(200), To: int64Ptr(300)}
	first := rotationOrder(pool, firstWindow, 5, 0)
	firstAgain := rotationOrder(pool, firstWindow, 5, 0)
	second := rotationOrder(pool, secondWindow, 5, 0)
	for i := range first {
		if first[i].ID != firstAgain[i].ID {
			t.Fatalf("rotation order is not deterministic at index %d", i)
		}
	}
	for i := 0; i < 5; i++ {
		if first[i].ID == second[i].ID {
			t.Fatalf("consecutive windows overlap at index %d (%q)", i, first[i].ID)
		}
	}
}

func int64Ptr(v int64) *int64 { return &v }

func TestConsecutiveQuietWindowsChangeEnough(t *testing.T) {
	pool := rotationPool(20)
	firstWindow := rangeWindow{Label: "day", From: int64Ptr(100), To: int64Ptr(200)}
	secondWindow := rangeWindow{Label: "day", From: int64Ptr(200), To: int64Ptr(300)}

	// The previous quiet page is reconstructed from its own rotation slice,
	// which matches what the endpoint would have returned.
	previousOrder := rotationOrder(pool, firstWindow, 10, 0)
	baselineEntries := make([]rankEntry, 0, 10)
	for i := 0; i < 10; i++ {
		baselineEntries = append(baselineEntries, syntheticRankEntry(&previousOrder[i]))
	}
	baseline := rankIDSet(baselineEntries)

	final, changed, required := applyRankRotation(
		nil, nil, 0, 10, baseline, pool, secondWindow, 60,
	)
	if len(final) != 10 {
		t.Fatalf("len(final) = %d, want 10", len(final))
	}
	if required != 6 {
		t.Fatalf("required = %d, want 6", required)
	}
	if changed < required {
		t.Fatalf("changed = %d, want at least %d", changed, required)
	}
	for _, entry := range final {
		if _, stale := baseline[entry.ID]; stale {
			t.Errorf("consecutive result still contains %q", entry.ID)
		}
	}
}

func TestRotationOrderRespectsPaginationOffset(t *testing.T) {
	pool := rotationPool(20)
	rng := rangeWindow{Label: "day", From: int64Ptr(200), To: int64Ptr(300)}
	first := rotationOrder(pool, rng, 10, 0)
	second := rotationOrder(pool, rng, 10, 10)
	seen := map[string]struct{}{}
	for i := 0; i < 10; i++ {
		seen[first[i].ID] = struct{}{}
	}
	for i := 0; i < 10; i++ {
		if _, duplicate := seen[second[i].ID]; duplicate {
			t.Fatalf("page 2 candidate %q duplicates page 1", second[i].ID)
		}
	}
}
