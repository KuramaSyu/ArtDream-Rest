package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestClaimAnonymousVotes(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	user := &User{Username: "ada", PasswordHash: "x"}
	if err := st.CreateUser(ctx, user); err != nil {
		t.Fatalf("create user: %v", err)
	}
	mode, err := st.GetVoteModeByName(ctx, "duel")
	if err != nil {
		t.Fatalf("get mode: %v", err)
	}

	anonymousA := &Vote{ModeID: mode.ID, ModeName: mode.Name, LeftImageID: "a", RightImageID: "b", MinValue: 0, MaxValue: 1, AnonymousID: "device-secret-aaaaaaaa"}
	anonymousB := &Vote{ModeID: mode.ID, ModeName: mode.Name, LeftImageID: "b", RightImageID: "c", MinValue: 0, MaxValue: 1, AnonymousID: "device-secret-aaaaaaaa"}
	owned := &Vote{ModeID: mode.ID, ModeName: mode.Name, LeftImageID: "c", RightImageID: "d", MinValue: 0, MaxValue: 1, UserID: &user.ID, Username: user.Username, AnonymousID: "device-secret-aaaaaaaa"}
	other := &Vote{ModeID: mode.ID, ModeName: mode.Name, LeftImageID: "d", RightImageID: "e", MinValue: 0, MaxValue: 1, AnonymousID: "device-secret-bbbbbbbb"}
	for _, v := range []*Vote{anonymousA, anonymousB, owned, other} {
		if err := st.CreateVote(ctx, v); err != nil {
			t.Fatalf("create vote: %v", err)
		}
	}

	claimed, err := st.ClaimAnonymousVotes(ctx, user.ID, user.Username, []string{"device-secret-aaaaaaaa"})
	if err != nil {
		t.Fatalf("claim votes: %v", err)
	}
	if claimed != 2 {
		t.Fatalf("claimed = %d, want 2", claimed)
	}

	for _, v := range []*Vote{anonymousA, anonymousB} {
		got, err := st.GetVote(ctx, v.ID)
		if err != nil {
			t.Fatalf("get vote: %v", err)
		}
		if got.UserID == nil || *got.UserID != user.ID || got.Username != user.Username {
			t.Fatalf("vote %d not owned by user: %+v", v.ID, got)
		}
	}
	got, err := st.GetVote(ctx, other.ID)
	if err != nil {
		t.Fatalf("get other vote: %v", err)
	}
	if got.UserID != nil {
		t.Fatalf("other device vote was claimed: %+v", got)
	}

	again, err := st.ClaimAnonymousVotes(ctx, user.ID, user.Username, []string{"device-secret-aaaaaaaa"})
	if err != nil {
		t.Fatalf("repeat claim: %v", err)
	}
	if again != 0 {
		t.Fatalf("repeat claim = %d, want 0", again)
	}
}
