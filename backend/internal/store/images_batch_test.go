package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
)

func TestGetImagesByIDsDeduplicatesAndBatches(t *testing.T) {
	st, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	if err := st.CreateImage(ctx, &Image{
		ID: "target", Author: "Ada", StoragePath: "target.jpg", MimeType: "image/jpeg", Width: 400, Height: 300,
	}); err != nil {
		t.Fatalf("create image: %v", err)
	}

	// Exceed SQLite's SQL-variable limit if the ids were sent in one query.
	ids := make([]string, 40000)
	for i := range ids {
		ids[i] = fmt.Sprintf("missing-%d", i)
	}
	ids[25000] = "target"
	ids[25001] = "target" // duplicate must not break the batch query

	images, err := st.GetImagesByIDs(ctx, ids)
	if err != nil {
		t.Fatalf("GetImagesByIDs: %v", err)
	}
	if len(images) != 1 || images["target"] == nil {
		t.Fatalf("images = %+v, want only target", images)
	}
}
