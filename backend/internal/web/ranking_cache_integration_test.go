package web

import (
	"context"
	"path/filepath"
	"testing"

	"artdream/internal/store"
)

func TestRankEntriesForWindowUsesAndInvalidatesCache(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	for _, id := range []string{"a", "b"} {
		if err := st.CreateImage(ctx, &store.Image{
			ID: id, Author: "Ada", StoragePath: id + ".jpg", MimeType: "image/jpeg", Width: 400, Height: 300,
		}); err != nil {
			t.Fatalf("create image %s: %v", id, err)
		}
	}
	winner := "a"
	vote := &store.Vote{
		LeftImageID: "a", RightImageID: "b", WinnerImageID: &winner,
		Weight: 1, MinValue: 1, MaxValue: 5, ModeName: "ahp5",
	}
	if err := st.CreateVote(ctx, vote); err != nil {
		t.Fatalf("create vote: %v", err)
	}

	srv := &Server{Store: st, rankCache: newRankingCache()}
	rng := rangeWindow{Label: "all"}
	filter := store.VoteFilter{Limit: -1}
	compute := func() []rankEntry {
		entries, _, err := srv.rankEntriesForWindow(ctx, rng, filter, nil, nil, "all", nil, ahpRankingStrategy{})
		if err != nil {
			t.Fatalf("rank entries: %v", err)
		}
		return entries
	}

	if entries := compute(); len(entries) != 2 {
		t.Fatalf("initial entries = %d, want 2", len(entries))
	}

	// Deleting directly through the store bypasses handler invalidation, so a
	// second call must still be served from the cache.
	if err := st.DeleteVote(ctx, vote.ID); err != nil {
		t.Fatalf("delete vote: %v", err)
	}
	if entries := compute(); len(entries) != 2 {
		t.Fatalf("cached entries = %d, want 2", len(entries))
	}

	srv.invalidateRankingCache()
	if entries := compute(); len(entries) != 0 {
		t.Fatalf("entries after invalidation = %d, want 0", len(entries))
	}
}
