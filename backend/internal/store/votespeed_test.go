package store

import (
	"context"
	"math"
	"path/filepath"
	"testing"
)

func ms(v int64) *int64 { return &v }

func TestVoteSpeedFor(t *testing.T) {
	history := []int64{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000}
	cases := []struct {
		name       string
		priorVotes int
		durations  []int64
		durationMs *int64
		want       float64
	}{
		{"calibration phase stays neutral", 9, history, ms(100), VoteSpeedNeutral},
		{"no duration stays neutral", 20, history, nil, VoteSpeedNeutral},
		{"fast vote is maximally obvious", 10, history, ms(100), VoteSpeedMax},
		{"slow vote is discounted", 10, history, ms(1000), VoteSpeedMin},
		{"median timing is neutral", 10, history, ms(550), VoteSpeedNeutral},
		{"outliers are trimmed", 10, []int64{1, 500, 500, 500, 500, 500, 500, 500, 500, 100000}, ms(500), VoteSpeedNeutral},
	}
	for _, tc := range cases {
		got := VoteSpeedFor(tc.priorVotes, tc.durations, tc.durationMs)
		if math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("%s: VoteSpeedFor = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestEffectiveWeightScalesWithSpeed(t *testing.T) {
	cases := []struct {
		speed, weight, want float64
	}{
		{VoteSpeedNeutral, 0.8, 0.8},
		{VoteSpeedMax, 0.8, 0.8 * VoteSpeedMaxFactor},
		{VoteSpeedMin, 0.8, 0.8 * VoteSpeedMin / VoteSpeedNeutral},
		{0, 0.8, 0.8}, // pre-migration rows fall back to neutral
	}
	for _, tc := range cases {
		v := &Vote{Weight: tc.weight, VoteSpeed: tc.speed}
		if got := v.EffectiveWeight(); math.Abs(got-tc.want) > 1e-9 {
			t.Errorf("EffectiveWeight(speed=%v, weight=%v) = %v, want %v", tc.speed, tc.weight, got, tc.want)
		}
	}
}

func TestVoteDurationHistoryTracksIdentity(t *testing.T) {
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
	base := Vote{ModeID: mode.ID, ModeName: mode.Name, LeftImageID: "a", RightImageID: "b", MinValue: 0, MaxValue: 1, AnonymousID: "device-secret-aaaaaaaa", Weight: 1}
	for i, d := range []int64{900, 100, 500} {
		v := base
		v.DurationMs = ms(d)
		if err := st.CreateVote(ctx, &v); err != nil {
			t.Fatalf("create vote %d: %v", i, err)
		}
	}

	count, durations, err := st.VoteDurationHistory(ctx, nil, "device-secret-aaaaaaaa", 0)
	if err != nil {
		t.Fatalf("duration history: %v", err)
	}
	if count != 3 || len(durations) != 3 {
		t.Fatalf("count=%d durations=%v, want 3/3", count, durations)
	}

	// A logged-in user does not see another device's anonymous votes.
	if n, _, err := st.VoteDurationHistory(ctx, &user.ID, "", 0); err != nil || n != 0 {
		t.Fatalf("user history count=%d err=%v, want 0", n, err)
	}

	// Excluding the vote itself keeps calibration honest on admin edits.
	all, _, _ := st.VoteDurationHistory(ctx, nil, "device-secret-aaaaaaaa", 0)
	excluded, _, _ := st.VoteDurationHistory(ctx, nil, "device-secret-aaaaaaaa", 1)
	if all != 3 || excluded != 2 {
		t.Fatalf("history with exclusion = %d, want 2 (of %d)", excluded, all)
	}
}
