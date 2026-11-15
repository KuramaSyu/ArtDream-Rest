package store

import (
	"context"
	"path/filepath"
	"testing"
)

func TestImageProvenanceFilters(t *testing.T) {
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
	other := &User{Username: "bob", PasswordHash: "x"}
	if err := st.CreateUser(ctx, other); err != nil {
		t.Fatalf("create user: %v", err)
	}

	images := []*Image{
		{ID: "a", Author: "Ada", StoragePath: "a.jpg", MimeType: "image/jpeg", Width: 400, Height: 300, UploadedBy: &user.ID, FromPublicService: true, ServiceName: "Instagram"},
		{ID: "b", Author: "Ada", StoragePath: "b.jpg", MimeType: "image/jpeg", Width: 400, Height: 300, UploadedBy: &user.ID},
		{ID: "c", Author: "Bob", StoragePath: "c.jpg", MimeType: "image/jpeg", Width: 400, Height: 300, UploadedBy: &other.ID, FromPublicService: true, ServiceName: "Flickr"},
	}
	for _, img := range images {
		if err := st.CreateImage(ctx, img); err != nil {
			t.Fatalf("create image %s: %v", img.ID, err)
		}
	}

	cfg, err := st.GetAppConfig(ctx)
	if err != nil {
		t.Fatalf("get config: %v", err)
	}
	if cfg.DefaultImageVisibility != VisibilityAll {
		t.Fatalf("default visibility = %q, want all", cfg.DefaultImageVisibility)
	}

	policy, err := CompileImageRequest(cfg, ImageRequest{
		Visibility: VisibilityPublic,
		Service:    "insta",
		UploadedBy: []int64{user.ID},
	})
	if err != nil {
		t.Fatalf("compile request: %v", err)
	}

	got, total, err := st.ImagesByAuthor(ctx, "", 50, 0, policy)
	if err != nil {
		t.Fatalf("images by author: %v", err)
	}
	if total != 1 || len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("ImagesByAuthor = %+v total=%d, want only image a", got, total)
	}

	random, err := st.RandomImages(ctx, 10, "", nil, policy)
	if err != nil {
		t.Fatalf("random images: %v", err)
	}
	if len(random) != 1 || random[0].ID != "a" {
		t.Fatalf("RandomImages = %+v, want only image a", random)
	}

	adminList, adminTotal, err := st.ListImages(ctx, ImageFilter{
		Visibility: VisibilityPrivate,
		UploadedBy: []int64{user.ID},
	})
	if err != nil {
		t.Fatalf("list images: %v", err)
	}
	if adminTotal != 1 || len(adminList) != 1 || adminList[0].ID != "b" {
		t.Fatalf("ListImages = %+v total=%d, want only image b", adminList, adminTotal)
	}

	fromPublic := true
	service := "Behance"
	if err := st.UpdateImage(ctx, "b", nil, nil, &fromPublic, &service); err != nil {
		t.Fatalf("update image: %v", err)
	}
	updated, err := st.GetImage(ctx, "b")
	if err != nil {
		t.Fatalf("get image: %v", err)
	}
	if !updated.FromPublicService || updated.ServiceName != service {
		t.Fatalf("updated image = %+v", updated)
	}
}
