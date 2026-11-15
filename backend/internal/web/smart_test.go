package web

import (
	"testing"
	"time"
)

func TestSmartRandomPageHonorsOffsetAndLimit(t *testing.T) {
	pool := rotationPool(20)
	now := time.Unix(1_760_000_000, 0).UTC()

	first := smartRandomPage(pool, now, 0, 5)
	if len(first) != 5 {
		t.Fatalf("first page length = %d, want 5", len(first))
	}
	again := smartRandomPage(pool, now, 0, 5)
	for i := range first {
		if first[i].ID != again[i].ID {
			t.Fatalf("daily random page changed at index %d", i)
		}
		if !first[i].Random {
			t.Errorf("entry %q should be marked random", first[i].ID)
		}
	}

	last := smartRandomPage(pool, now, 15, 5)
	if len(last) != 5 {
		t.Fatalf("last page length = %d, want remaining 5", len(last))
	}
	seen := map[string]struct{}{}
	for _, entry := range first {
		seen[entry.ID] = struct{}{}
	}
	for _, entry := range last {
		if _, duplicate := seen[entry.ID]; duplicate {
			t.Errorf("last page overlaps first page at %q", entry.ID)
		}
	}
	if got := smartRandomPage(pool, now, 20, 5); len(got) != 0 {
		t.Fatalf("offset past end returned %d entries", len(got))
	}
}

func TestRollingRangeForLabel(t *testing.T) {
	now := time.Unix(1_760_000_000, 0).UTC()
	cases := map[string]time.Duration{
		"day":   rangeDay,
		"week":  rangeWeek,
		"month": rangeMonth,
		"year":  rangeYear,
	}
	for label, want := range cases {
		rng := rollingRangeForLabel(label, now)
		if rng.From == nil || rng.To == nil {
			t.Fatalf("%s range missing bounds", label)
		}
		if got := time.Duration(*rng.To-*rng.From) * time.Second; got != want {
			t.Errorf("%s duration = %v, want %v", label, got, want)
		}
	}
}
