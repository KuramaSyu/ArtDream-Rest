package web

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"artdream/internal/store"
)

func TestRankEntriesForWindowExploresNewUnratedImages(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	now := time.Now().UTC()
	images := []*store.Image{
		{ID: "left", Author: "Ada", StoragePath: "left.jpg", MimeType: "image/jpeg", Width: 400, Height: 300, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "right", Author: "Ada", StoragePath: "right.jpg", MimeType: "image/jpeg", Width: 400, Height: 300, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "fresh", Author: "Ada", StoragePath: "fresh.jpg", MimeType: "image/jpeg", Width: 400, Height: 300, CreatedAt: now.Add(-time.Hour)},
		{ID: "stale", Author: "Ada", StoragePath: "stale.jpg", MimeType: "image/jpeg", Width: 400, Height: 300, CreatedAt: now.Add(-30 * 24 * time.Hour)},
	}
	for _, img := range images {
		if err := st.CreateImage(ctx, img); err != nil {
			t.Fatalf("create image %s: %v", img.ID, err)
		}
	}
	winner := "left"
	vote := &store.Vote{
		LeftImageID: "left", RightImageID: "right", WinnerImageID: &winner,
		Weight: 1, MinValue: 1, MaxValue: 5, ModeName: "ahp5",
	}
	if err := st.CreateVote(ctx, vote); err != nil {
		t.Fatalf("create vote: %v", err)
	}

	srv := &Server{Store: st, rankCache: newRankingCache()}
	rng := rangeWindow{Label: "all"}
	filter := store.VoteFilter{Limit: -1}

	pool := make([]store.Image, 0, len(images))
	for _, img := range images {
		pool = append(pool, *img)
	}
	loads := 0
	plan := &explorePlan{
		Now:     now,
		Scope:   "all",
		Enabled: true,
		Load: func() ([]store.Image, error) {
			loads++
			return pool, nil
		},
	}

	entries, _, err := srv.rankEntriesForWindow(ctx, rng, filter, nil, nil, "all", nil, ahpRankingStrategy{}, plan)
	if err != nil {
		t.Fatalf("rank entries: %v", err)
	}
	if loads != 1 {
		t.Fatalf("candidate loader called %d times, want 1", loads)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3 (two ranked + one explored)", len(entries))
	}
	var explored *rankEntry
	for i := range entries {
		if entries[i].Explore {
			explored = &entries[i]
		}
	}
	if explored == nil || explored.ID != "fresh" {
		t.Fatalf("expected the fresh unvoted image to be explored, got %+v", explored)
	}
	if explored.Votes != 0 || explored.Score != 0 {
		t.Fatalf("explored entry must keep zero vote stats, got votes=%d score=%v", explored.Votes, explored.Score)
	}
	if explored.Boost <= 1 {
		t.Fatalf("explored entry needs a boost > 1, got %v", explored.Boost)
	}
	if explored.orderKey <= 0 {
		t.Fatalf("explored entry needs an ordering key, got %v", explored.orderKey)
	}

	// Archived in the cache: a second call must not hit the library again.
	if _, _, err := srv.rankEntriesForWindow(ctx, rng, filter, nil, nil, "all", nil, ahpRankingStrategy{}, plan); err != nil {
		t.Fatalf("rank entries (cached): %v", err)
	}
	if loads != 1 {
		t.Fatalf("cached ranking re-queried the library: loads = %d", loads)
	}
}

func TestRankEntriesForWindowTodayOnlyDropsOlderUploads(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	now := time.Now().UTC()
	images := []*store.Image{
		{ID: "old-a", Author: "Ada", StoragePath: "old-a.jpg", MimeType: "image/jpeg", Width: 400, Height: 300, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "old-b", Author: "Ada", StoragePath: "old-b.jpg", MimeType: "image/jpeg", Width: 400, Height: 300, CreatedAt: now.Add(-48 * time.Hour)},
		{ID: "today-a", Author: "Ada", StoragePath: "today-a.jpg", MimeType: "image/jpeg", Width: 400, Height: 300, CreatedAt: now.Add(-time.Hour)},
		{ID: "today-b", Author: "Ada", StoragePath: "today-b.jpg", MimeType: "image/jpeg", Width: 400, Height: 300, CreatedAt: now.Add(-2 * time.Hour)},
	}
	for _, img := range images {
		if err := st.CreateImage(ctx, img); err != nil {
			t.Fatalf("create image %s: %v", img.ID, err)
		}
	}
	for _, pair := range [][2]string{{"old-a", "old-b"}, {"today-a", "today-b"}} {
		winner := pair[0]
		vote := &store.Vote{
			LeftImageID: pair[0], RightImageID: pair[1], WinnerImageID: &winner,
			Weight: 1, MinValue: 1, MaxValue: 5, ModeName: "ahp5",
		}
		if err := st.CreateVote(ctx, vote); err != nil {
			t.Fatalf("create vote: %v", err)
		}
	}

	srv := &Server{Store: st, rankCache: newRankingCache()}
	rng := rangeWindow{Label: "all"}
	filter := store.VoteFilter{Limit: -1}
	plan := &explorePlan{Now: now, Scope: "all", CreatedFrom: todayStart(now)}

	entries, _, err := srv.rankEntriesForWindow(ctx, rng, filter, nil, nil, "all", nil, ahpRankingStrategy{}, plan)
	if err != nil {
		t.Fatalf("rank entries: %v", err)
	}
	ids := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		ids[entry.ID] = struct{}{}
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want only today's two uploads", len(entries))
	}
	for _, want := range []string{"today-a", "today-b"} {
		if _, ok := ids[want]; !ok {
			t.Fatalf("missing today's upload %s in %v", want, ids)
		}
	}

	// Without the filter the old pair ranks again.
	unfiltered, _, err := srv.rankEntriesForWindow(ctx, rng, filter, nil, nil, "all", nil, ahpRankingStrategy{}, nil)
	if err != nil {
		t.Fatalf("unfiltered rank entries: %v", err)
	}
	if len(unfiltered) != 4 {
		t.Fatalf("unfiltered entries = %d, want 4", len(unfiltered))
	}
}
