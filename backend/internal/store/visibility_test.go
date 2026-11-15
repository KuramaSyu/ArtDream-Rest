package store

import "testing"

func TestVisibilityPolicy(t *testing.T) {
	policy, err := CompileVisibility(AppConfig{
		ExcludeAuthors: []string{"spam.*"},
		ExcludeTitles:  []string{"advert"},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cases := []struct {
		author, title string
		hidden        bool
	}{
		{"Ada", "Sunset over water", false},
		{"SpamBot", "Nice hills", true},
		{"Bob", "Advert spam collage", true},
		{"spambot", "clean title", true}, // case-insensitive
	}
	for _, tc := range cases {
		if got := policy.HiddenMetadata(tc.author, tc.title); got != tc.hidden {
			t.Errorf("HiddenMetadata(%q, %q) = %v, want %v", tc.author, tc.title, got, tc.hidden)
		}
	}
}

func TestVisibilityPolicyAllowlists(t *testing.T) {
	policy, err := CompileVisibility(AppConfig{
		IncludeAuthors: []string{"ada|bob"},
		IncludeTitles:  []string{"green"},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cases := []struct {
		author, title string
		hidden        bool
	}{
		{"Ada", "Dreaming in Green", false},
		{"Bob", "Green hills", false},
		{"Ada", "Blue hour", true},   // title allowlist fails
		{"Zoe", "Green hills", true}, // author allowlist fails
	}
	for _, tc := range cases {
		if got := policy.HiddenMetadata(tc.author, tc.title); got != tc.hidden {
			t.Errorf("HiddenMetadata(%q, %q) = %v, want %v", tc.author, tc.title, got, tc.hidden)
		}
	}
}

func TestModerationFor(t *testing.T) {
	rows := []ImageModeration{
		{ImageID: "img1", TimeWindow: ModerationAll, Hidden: true},
		{ImageID: "img2", TimeWindow: ModerationDay, Deranked: true},
		{ImageID: "img3", TimeWindow: ModerationAll, Hidden: true},
		{ImageID: "img3", TimeWindow: ModerationWeek, Hidden: false},
	}

	if got, ok := ModerationFor(rows, "img1", ModerationWeek); !ok || !got.Hidden {
		t.Errorf("expected all-time fallback to hide img1, got %+v (%v)", got, ok)
	}
	if got, ok := ModerationFor(rows, "img2", ModerationWeek); ok {
		t.Errorf("did not expect a week moderation for img2, got %+v", got)
	}
	if got, ok := ModerationFor(rows, "img2", ModerationDay); !ok || !got.Deranked {
		t.Errorf("expected day derank for img2, got %+v (%v)", got, ok)
	}
	// An explicit row overrides the global fallback even when it clears it.
	if got, ok := ModerationFor(rows, "img3", ModerationWeek); !ok || got.Hidden {
		t.Errorf("expected week override to unhide img3, got %+v (%v)", got, ok)
	}
	if _, ok := ModerationFor(rows, "missing", ModerationDay); ok {
		t.Errorf("expected no moderation for an unknown image")
	}
}
