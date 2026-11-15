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

func TestCompileImageRequestVisibilityAndUploader(t *testing.T) {
	cfg := AppConfig{DefaultImageVisibility: VisibilityAll}
	policy, err := CompileImageRequest(cfg, ImageRequest{
		Visibility: VisibilityPublic,
		UploadedBy: []int64{7, 9},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	uid7, uid9, uid12 := int64(7), int64(9), int64(12)
	cases := []struct {
		name string
		img  Image
		want bool
	}{
		{"public owned", Image{FromPublicService: true, UploadedBy: &uid7}, false},
		{"public friend", Image{FromPublicService: true, UploadedBy: &uid9}, false},
		{"private owned", Image{FromPublicService: false, UploadedBy: &uid7}, true},
		{"public stranger", Image{FromPublicService: true, UploadedBy: &uid12}, true},
		{"public anonymous", Image{FromPublicService: true}, true},
	}
	for _, tc := range cases {
		if got := policy.Hidden(&tc.img); got != tc.want {
			t.Errorf("%s: Hidden = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCompileImageRequestUsesAdminDefault(t *testing.T) {
	policy, err := CompileImageRequest(AppConfig{DefaultImageVisibility: VisibilityPrivate}, ImageRequest{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if policy.Hidden(&Image{FromPublicService: false}) {
		t.Errorf("private default should keep private images")
	}
	if !policy.Hidden(&Image{FromPublicService: true}) {
		t.Errorf("private default should hide public-service images")
	}
}

func TestCompileImageRequestLayeredListsAndService(t *testing.T) {
	cfg := AppConfig{
		IncludeAuthors: []string{"ada|bob"},
		ExcludeTitles:  []string{"spam"},
	}
	policy, err := CompileImageRequest(cfg, ImageRequest{
		Service:        "instagram",
		IncludeTitles:  []string{"sunset|sea"},
		ExcludeAuthors: []string{"bot"},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	cases := []struct {
		name string
		img  Image
		want bool
	}{
		{"matches all", Image{Author: "Ada", Title: "Sunset", ServiceName: "Instagram"}, false},
		{"global title deny", Image{Author: "Ada", Title: "Spam sunset", ServiceName: "Instagram"}, true},
		{"request title allow fails", Image{Author: "Ada", Title: "Portrait", ServiceName: "Instagram"}, true},
		{"request author deny", Image{Author: "Bot", Title: "Sunset", ServiceName: "Instagram"}, true},
		{"wrong service", Image{Author: "Bob", Title: "Sea", ServiceName: "Flickr"}, true},
		{"empty service", Image{Author: "Bob", Title: "Sea"}, true},
	}
	for _, tc := range cases {
		if got := policy.Hidden(&tc.img); got != tc.want {
			t.Errorf("%s: Hidden = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestSQLFilter(t *testing.T) {
	policy, err := CompileImageRequest(AppConfig{}, ImageRequest{
		Visibility: VisibilityPublic,
		Service:    "Art",
		UploadedBy: []int64{9, 3},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	where, args := policy.SQLFilter("images")
	if where == "" {
		t.Fatal("expected SQL filter")
	}
	if len(args) != 3 {
		t.Fatalf("args = %v, want 3", args)
	}
	if args[0] != "%art%" {
		t.Errorf("service arg = %v", args[0])
	}
	if args[1] != int64(3) || args[2] != int64(9) {
		t.Errorf("uploader ids should be sorted, got %v", args[1:])
	}
}
