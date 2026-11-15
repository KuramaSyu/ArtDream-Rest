package web

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"artdream/internal/imageproc"
	"artdream/internal/store"

	"github.com/gin-gonic/gin"
)

type updateImageRequest struct {
	Author            *string `json:"author"`
	Title             *string `json:"title"`
	FromPublicService *bool   `json:"fromPublicService"`
	ServiceName       *string `json:"serviceName"`
}

type uploadResponse struct {
	Image      imageDTO `json:"image"`
	ClaimToken string   `json:"claimToken,omitempty"`
}

type claimItem struct {
	ID         string `json:"id"`
	ImageID    string `json:"imageId"`
	Token      string `json:"token"`
	ClaimToken string `json:"claimToken"`
}

type claimRequest struct {
	Claims []claimItem `json:"claims"`
	claimItem
}

// uploadImage godoc
// @Summary Upload an image
// @Description Accepts a landscape image (JPEG, PNG, GIF, WebP, BMP, TIFF). Authentication is optional: anonymous uploads receive a claimToken that can later attach the image to an account.
// @Tags images
// @Accept multipart/form-data
// @Produce json
// @Param image formData file true "Image file (the `file` field is also accepted)"
// @Param author formData string true "Artist/author name"
// @Param title formData string false "Title"
// @Param fromPublicService formData bool false "Image comes from a public service"
// @Param serviceName formData string false "Public service name"
// @Success 201 {object} uploadResponse
// @Failure 400 {object} apiError
// @Failure 413 {object} apiError
// @Failure 500 {object} apiError
// @Router /images [post]
func (s *Server) uploadImage(c *gin.Context) {
	user := currentUser(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, s.MaxUploadBytes)

	fileHeader, err := c.FormFile("image")
	if err != nil {
		fileHeader, err = c.FormFile("file")
	}
	if err != nil {
		badRequest(c, "Attach an image in the `image` form field.")
		return
	}
	if fileHeader.Size <= 0 {
		badRequest(c, "The uploaded file is empty.")
		return
	}
	if fileHeader.Size > s.MaxUploadBytes {
		fail(c, http.StatusRequestEntityTooLarge, "too_large", "Images must be smaller than 30 MiB.")
		return
	}

	f, err := fileHeader.Open()
	if err != nil {
		serverError(c, err)
		return
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, s.MaxUploadBytes+1))
	if err != nil {
		serverError(c, err)
		return
	}
	if int64(len(data)) > s.MaxUploadBytes {
		fail(c, http.StatusRequestEntityTooLarge, "too_large", "Images must be smaller than 30 MiB.")
		return
	}

	cfg, format, err := imageproc.DecodeConfig(data)
	if err != nil {
		badRequest(c, "That file does not look like a supported image (JPEG, PNG, GIF, WebP, BMP, TIFF).")
		return
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > 20000 || cfg.Height > 20000 ||
		int64(cfg.Width)*int64(cfg.Height) > 120_000_000 {
		badRequest(c, "Image dimensions are outside the supported range (max 20000px and 120 megapixels).")
		return
	}
	// The submit page currently only accepts landscape artwork. Enforcing it
	// in the API keeps other clients honest too.
	if cfg.Width <= cfg.Height {
		badRequest(c, "Please submit a landscape image (width must be greater than height).")
		return
	}

	author := trimTo(c.PostForm("author"), 120)
	if author == "" {
		badRequest(c, "Tell us who made this image (author/username is required).")
		return
	}
	title := trimTo(c.PostForm("title"), 200)
	fromPublicService := truthy(firstNonEmpty(
		c.PostForm("fromPublicService"),
		c.PostForm("publicService"),
		c.PostForm("public"),
	))
	serviceName := ""
	if fromPublicService {
		serviceName = trimTo(c.PostForm("serviceName"), 120)
	}

	id, err := newImageID()
	if err != nil {
		serverError(c, err)
		return
	}
	mime := mimeForFormat(format)
	ext := extForFormat(format)
	dest := filepath.Join(s.UploadDir, id+"."+ext)
	if err := os.MkdirAll(s.UploadDir, 0o755); err != nil {
		serverError(c, err)
		return
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		serverError(c, err)
		return
	}

	img := &store.Image{
		ID:                id,
		Author:            author,
		Title:             title,
		OriginalFilename:  fileHeader.Filename,
		StoragePath:       dest,
		MimeType:          mime,
		Width:             cfg.Width,
		Height:            cfg.Height,
		SizeBytes:         int64(len(data)),
		FromPublicService: fromPublicService,
		ServiceName:       serviceName,
	}
	claimToken := ""
	if user != nil {
		img.UploadedBy = &user.ID
	} else {
		// Anonymous uploads can be attached to an account later with this
		// secret token, stored on the submitter's device.
		claimToken, err = newClaimToken()
		if err != nil {
			serverError(c, err)
			return
		}
		img.ClaimToken = claimToken
	}
	if err := s.Store.CreateImage(c.Request.Context(), img); err != nil {
		_ = os.Remove(dest)
		serverError(c, err)
		return
	}
	// Warm the sizes the UI asks for most. This turns the first gallery /
	// ranking / landing view into a cache hit instead of a full decode +
	// resize at request time.
	go s.warmImageVariants(img)
	c.JSON(http.StatusCreated, uploadResponse{Image: imageToDTO(img), ClaimToken: claimToken})
}

// claimImages links images uploaded anonymously from this device to the
// signed-in account. Clients send the claim tokens returned at upload time.
// claimImages godoc
// @Summary Claim anonymous uploads
// @Description Attaches images uploaded anonymously from this device to the signed-in account using their claim tokens.
// @Tags images
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body claimRequest true "One or more image ID/claim-token pairs"
// @Success 200 {object} ClaimResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 500 {object} apiError
// @Router /images/claim [post]
func (s *Server) claimImages(c *gin.Context) {
	user := currentUser(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var req claimRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Could not parse the claim payload.")
		return
	}
	claims := make([]store.ImageClaim, 0, len(req.Claims)+1)
	for _, item := range req.Claims {
		id := strings.TrimSpace(firstNonEmpty(item.ID, item.ImageID))
		token := strings.TrimSpace(firstNonEmpty(item.Token, item.ClaimToken))
		if id != "" && token != "" {
			claims = append(claims, store.ImageClaim{ID: id, Token: token})
		}
	}
	if id := strings.TrimSpace(firstNonEmpty(req.ID, req.ImageID)); id != "" {
		token := strings.TrimSpace(firstNonEmpty(req.Token, req.ClaimToken))
		if token != "" {
			claims = append(claims, store.ImageClaim{ID: id, Token: token})
		}
	}
	if len(claims) == 0 {
		badRequest(c, "Provide at least one id and claim token.")
		return
	}
	if len(claims) > 200 {
		badRequest(c, "At most 200 submissions can be claimed at once.")
		return
	}
	claimed, err := s.Store.ClaimImages(c.Request.Context(), user.ID, claims)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"claimed": claimed, "count": len(claimed)})
}

// warmImageVariants renders the common display sizes in the background.
// Renderer writes are atomic, so concurrent requests for the same size are
// safe and simply wait for / reuse the finished file.
func (s *Server) warmImageVariants(img *store.Image) {
	options := []imageproc.RenderOptions{
		{Format: "webp", Width: 360, Quality: 78, SourceMime: img.MimeType},
		{Format: "webp", Width: 640, Quality: 78, SourceMime: img.MimeType},
		{Format: "webp", Height: 720, Quality: 82, SourceMime: img.MimeType},
		{Format: "webp", Height: 1080, Quality: 85, SourceMime: img.MimeType},
	}
	for _, option := range options {
		if _, _, err := s.Renderer.Render(img.StoragePath, img.ID, option); err != nil {
			log.Printf("warm image %s: %v", img.ID, err)
			return
		}
	}
}

func newClaimToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// imagesByAuthor godoc
// @Summary List images by author
// @Description Lists images for one author, or across artists when uploadedBy/mine is supplied. Authentication is optional but required for mine/uploadedBy=me.
// @Tags images
// @Produce json
// @Param author query string false "Author name (required unless uploadedBy/mine is set)"
// @Param amount query int false "Page size (default 100, max 500)"
// @Param offset query int false "Pagination offset"
// @Param visibility query string false "Visibility filter" Enums(all, public, private)
// @Param service query string false "Public service name filter"
// @Param uploadedBy query []string false "Uploader IDs or usernames (comma separated, 'me' allowed)" collectionFormat(csv)
// @Param mine query bool false "Only images uploaded by the signed-in user"
// @Param includeAuthors query []string false "Author allow-list regexes" collectionFormat(csv)
// @Param excludeAuthors query []string false "Author deny-list regexes" collectionFormat(csv)
// @Param includeTitles query []string false "Title allow-list regexes" collectionFormat(csv)
// @Param excludeTitles query []string false "Title deny-list regexes" collectionFormat(csv)
// @Success 200 {object} AuthorImageListResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 500 {object} apiError
// @Router /images/author [get]
func (s *Server) imagesByAuthor(c *gin.Context) {
	author := trimTo(c.Query("author"), 120)
	req, err := imageRequestFromQuery(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	if author == "" && len(req.UploadedBy) == 0 {
		badRequest(c, "Provide an author, or use uploadedBy/mine to list uploads across artists.")
		return
	}
	cfg, err := s.Store.GetAppConfig(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	policy, err := store.CompileImageRequest(cfg, req)
	if err != nil {
		asStoreError(c, err)
		return
	}
	images, total, err := s.Store.ImagesByAuthor(c.Request.Context(), author, parseAmount(c, 100, 500), parseOffset(c), policy)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": imagesToDTO(images), "total": total, "author": author})
}

// randomImages godoc
// @Summary Get random images
// @Description Returns a random selection of visible images, optionally filtered by author. Authentication is optional; it enables the mine/uploadedBy filters.
// @Tags images
// @Produce json
// @Param amount query int false "Number of images (default 1, max 50)"
// @Param author query string false "Author filter"
// @Param exclude query []string false "Image IDs to exclude" collectionFormat(csv)
// @Param visibility query string false "Visibility filter" Enums(all, public, private)
// @Param service query string false "Public service name filter"
// @Param uploadedBy query []string false "Uploader IDs or usernames (comma separated, 'me' allowed)" collectionFormat(csv)
// @Param mine query bool false "Only images uploaded by the signed-in user"
// @Success 200 {array} imageDTO
// @Failure 400 {object} apiError
// @Failure 500 {object} apiError
// @Router /images/random [get]
func (s *Server) randomImages(c *gin.Context) {
	amount := parseAmount(c, 1, 50)
	var exclude []string
	for _, id := range strings.Split(c.Query("exclude"), ",") {
		if id = strings.TrimSpace(id); id != "" {
			exclude = append(exclude, id)
		}
	}
	policy, err := imageRequestPolicy(c)
	if err != nil {
		asStoreError(c, err)
		return
	}
	images, err := s.Store.RandomImages(c.Request.Context(), amount, trimTo(c.Query("author"), 120), exclude, policy)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, imagesToDTO(images))
}

// imageMeta godoc
// @Summary Get image metadata
// @Tags images
// @Produce json
// @Param id path string true "Image ID"
// @Success 200 {object} imageDTO
// @Failure 404 {object} apiError
// @Failure 500 {object} apiError
// @Router /images/{id}/meta [get]
func (s *Server) imageMeta(c *gin.Context) {
	id := c.Param("id")
	img, err := s.Store.GetImage(c.Request.Context(), id)
	if err != nil {
		asStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, imageToDTO(img))
}

// serveImage godoc
// @Summary Fetch an image
// @Description Serves a rendered variant by default (WebP, capped at 1920px long edge / 1080px tall). Use original=true for the pristine upload.
// @Tags images
// @Produce image/webp
// @Param id path string true "Image ID"
// @Param original query bool false "Return the original file instead of a rendered variant"
// @Param download query bool false "Send the image as an attachment"
// @Param format query string false "Output format" Enums(webp, png, jpg, jpeg)
// @Param w query int false "Target width (1-8192)"
// @Param h query int false "Target height (1-8192)"
// @Param quality query int false "Encoding quality (1-100)"
// @Success 200 {file} binary "Image bytes"
// @Failure 304 "Not modified"
// @Failure 400 {object} apiError
// @Failure 404 {object} apiError
// @Failure 500 {object} apiError
// @Router /images/{id} [get]
func (s *Server) serveImage(c *gin.Context) {
	id := c.Param("id")
	img, err := s.Store.GetImage(c.Request.Context(), id)
	if err != nil {
		asStoreError(c, err)
		return
	}

	original := c.Query("original") == "true" || c.Query("original") == "1"
	opts := imageproc.RenderOptions{SourceMime: img.MimeType}
	if format := strings.ToLower(strings.TrimSpace(c.Query("format"))); format != "" {
		switch format {
		case "webp", "png", "jpg", "jpeg":
			opts.Format = format
		default:
			badRequest(c, "format must be one of webp, png, jpg.")
			return
		}
	}
	if raw := c.Query("w"); raw != "" {
		w, err := strconv.Atoi(raw)
		if err != nil || w < 1 || w > 8192 {
			badRequest(c, "w must be between 1 and 8192.")
			return
		}
		opts.Width = w
	}
	if raw := c.Query("h"); raw != "" {
		h, err := strconv.Atoi(raw)
		if err != nil || h < 1 || h > 8192 {
			badRequest(c, "h must be between 1 and 8192.")
			return
		}
		opts.Height = h
	}
	if raw := c.Query("quality"); raw != "" {
		q, err := strconv.Atoi(raw)
		if err != nil || q < 1 || q > 100 {
			badRequest(c, "quality must be between 1 and 100.")
			return
		}
		opts.Quality = q
	}

	if original {
		// Original bytes are opt-in only (original=true), e.g. the explicit
		// "Original" download in the library. Normal page requests default to
		// a capped WebP so a 30 MP upload never crosses the wire unfiltered.
		info, err := os.Stat(img.StoragePath)
		if err != nil {
			serverError(c, err)
			return
		}
		etag := fmt.Sprintf(`"%s-original-%d"`, img.ID, info.Size())
		if match := c.GetHeader("If-None-Match"); match == etag || strings.Contains(match, etag) {
			c.Status(http.StatusNotModified)
			return
		}
		data, err := os.ReadFile(img.StoragePath)
		if err != nil {
			serverError(c, err)
			return
		}
		c.Header("ETag", etag)
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		if c.Query("download") == "true" {
			name := img.Title
			if name == "" {
				name = img.ID
			}
			c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"`, sanitizeFilename(name), extForFormat(strings.TrimPrefix(strings.ToLower(img.MimeType), "image/"))))
		}
		c.Data(http.StatusOK, img.MimeType, data)
		return
	}

	// Default display request: cap large originals to a 1920px long edge /
	// 1080p tall frame. Smaller uploads keep their native size (the WebP fast
	// path avoids re-encoding them).
	if opts.Width == 0 && opts.Height == 0 {
		if img.Width > 1920 {
			opts.Width = 1920
		} else if img.Height > 1080 {
			opts.Height = 1080
		}
	}

	etag := fmt.Sprintf(`"%s-%s-%d-%d-%d-%d"`, img.ID, normalFormat(opts.Format), opts.Width, opts.Height, opts.Quality, img.SizeBytes)
	if match := c.GetHeader("If-None-Match"); match == etag || strings.Contains(match, etag) {
		c.Status(http.StatusNotModified)
		return
	}

	data, contentType, err := s.Renderer.Render(img.StoragePath, img.ID, opts)
	if err != nil {
		serverError(c, err)
		return
	}
	c.Header("ETag", etag)
	c.Header("Cache-Control", "public, max-age=31536000, immutable")
	if c.Query("download") == "true" {
		name := img.Title
		if name == "" {
			name = img.ID
		}
		c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"`, sanitizeFilename(name), fileExt(contentType)))
	}
	c.Data(http.StatusOK, contentType, data)
}

// listImagesAdmin godoc
// @Summary List images (admin)
// @Description Lists images including private and optionally deleted entries, using the shared admin filters.
// @Tags images
// @Security BearerAuth
// @Produce json
// @Param amount query int false "Page size (default 50, max 500)"
// @Param offset query int false "Pagination offset"
// @Param author query string false "Author substring filter"
// @Param authorName query string false "Alias for author"
// @Param titleRegex query string false "Title regex filter"
// @Param visibility query string false "Visibility filter" Enums(all, public, private)
// @Param service query string false "Public service name filter"
// @Param uploadedBy query []string false "Uploader IDs or usernames (comma separated, 'me' allowed)" collectionFormat(csv)
// @Param mine query bool false "Only images uploaded by the signed-in user"
// @Param includeDeleted query bool false "Include soft-deleted images"
// @Param time query string false "Time window (accepted for symmetry with the vote filters)" Enums(day, week, month, year, all, custom)
// @Success 200 {object} ImageListResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 500 {object} apiError
// @Router /admin/images [get]
func (s *Server) listImagesAdmin(c *gin.Context) {
	_, imgf, _, err := filterFromQuery(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	images, total, err := s.Store.ListImages(c.Request.Context(), imgf)
	if err != nil {
		asStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": imagesToDTO(images), "total": total})
}

// updateImageAdmin godoc
// @Summary Update image metadata (admin)
// @Description Updates author, title and public-service provenance. Setting fromPublicService=false clears serviceName.
// @Tags images
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path string true "Image ID"
// @Param payload body updateImageRequest true "Fields to update"
// @Success 200 {object} imageDTO
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 404 {object} apiError
// @Failure 500 {object} apiError
// @Router /admin/images/{id} [patch]
func (s *Server) updateImageAdmin(c *gin.Context) {
	id := c.Param("id")
	var req updateImageRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Provide author, title, fromPublicService and/or serviceName.")
		return
	}
	var author, title *string
	if req.Author != nil {
		a := trimTo(*req.Author, 120)
		if a == "" {
			badRequest(c, "Author cannot be empty.")
			return
		}
		author = &a
	}
	if req.Title != nil {
		t := trimTo(*req.Title, 200)
		title = &t
	}
	var fromPublicService *bool
	if req.FromPublicService != nil {
		v := *req.FromPublicService
		fromPublicService = &v
	}
	var serviceName *string
	if req.ServiceName != nil {
		s := trimTo(*req.ServiceName, 120)
		serviceName = &s
	}
	// A private image cannot claim a public service name.
	if fromPublicService != nil && !*fromPublicService {
		empty := ""
		serviceName = &empty
	}
	if author == nil && title == nil && fromPublicService == nil && serviceName == nil {
		badRequest(c, "Nothing to update.")
		return
	}
	if err := s.Store.UpdateImage(c.Request.Context(), id, author, title, fromPublicService, serviceName); err != nil {
		serverError(c, err)
		return
	}
	img, err := s.Store.GetImage(c.Request.Context(), id)
	if err != nil {
		asStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, imageToDTO(img))
}

// deleteImageAdmin godoc
// @Summary Delete an image (admin)
// @Description Deletes the image, its cache variants and (unless keepVotes=true) its votes.
// @Tags images
// @Security BearerAuth
// @Produce json
// @Param id path string true "Image ID"
// @Param keepVotes query bool false "Keep the votes that reference this image"
// @Success 200 {object} DeleteImageResponse
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 404 {object} apiError
// @Failure 500 {object} apiError
// @Router /admin/images/{id} [delete]
func (s *Server) deleteImageAdmin(c *gin.Context) {
	id := c.Param("id")
	keepVotes := c.Query("keepVotes") == "true"
	if err := s.deleteImage(c.Request.Context(), id, !keepVotes); err != nil {
		asStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// bulkDeleteImagesAdmin godoc
// @Summary Bulk delete images (admin)
// @Description Deletes the images named by ids, or every image matching the shared admin filters when ids is omitted.
// @Tags images
// @Security BearerAuth
// @Produce json
// @Param ids query []string false "Explicit image IDs; takes precedence over the filters" collectionFormat(csv)
// @Param keepVotes query bool false "Keep votes that reference the deleted images"
// @Param author query string false "Author substring filter"
// @Param titleRegex query string false "Title regex filter"
// @Param visibility query string false "Visibility filter" Enums(all, public, private)
// @Param service query string false "Public service name filter"
// @Param uploadedBy query []string false "Uploader IDs or usernames (comma separated, 'me' allowed)" collectionFormat(csv)
// @Param mine query bool false "Only images uploaded by the signed-in user"
// @Param includeDeleted query bool false "Include soft-deleted images"
// @Success 200 {object} DeletedCountResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 500 {object} apiError
// @Router /admin/images [delete]
func (s *Server) bulkDeleteImagesAdmin(c *gin.Context) {
	_, imgf, _, err := filterFromQuery(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	// An explicit id list is the most precise way to delete a selection.
	explicit := c.Query("ids")
	var ids []string
	for _, id := range strings.Split(explicit, ",") {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}

	keepVotes := c.Query("keepVotes") == "true"
	deleted := 0
	if len(ids) > 0 {
		for _, id := range ids {
			if err := s.deleteImage(c.Request.Context(), id, !keepVotes); err != nil {
				if err == store.ErrNotFound {
					continue
				}
				asStoreError(c, err)
				return
			}
			deleted++
		}
		c.JSON(http.StatusOK, gin.H{"deleted": deleted})
		return
	}

	imgf.Limit = 0
	imgf.Offset = 0
	images, _, err := s.Store.ListImages(c.Request.Context(), imgf)
	if err != nil {
		asStoreError(c, err)
		return
	}
	for i := range images {
		if err := s.deleteImage(c.Request.Context(), images[i].ID, !keepVotes); err != nil {
			asStoreError(c, err)
			return
		}
		deleted++
	}
	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}

func (s *Server) deleteImage(ctx context.Context, id string, deleteVotes bool) error {
	img, err := s.Store.GetImage(ctx, id)
	if err != nil {
		return err
	}
	if deleteVotes {
		_ = s.Store.DeleteVotesForImage(ctx, id)
	}
	if err := s.Store.DeleteImage(ctx, id); err != nil {
		return err
	}
	_ = s.Store.DeleteModerationForImage(ctx, id)
	_ = os.Remove(img.StoragePath)
	_ = s.Renderer.RemoveForImage(id)
	return nil
}

func newImageID() (string, error) {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func mimeForFormat(format string) string {
	switch format {
	case "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "webp":
		return "image/webp"
	case "bmp":
		return "image/bmp"
	case "tiff":
		return "image/tiff"
	default:
		return "application/octet-stream"
	}
}

func extForFormat(format string) string {
	switch format {
	case "jpeg":
		return "jpg"
	case "tiff":
		return "tif"
	default:
		return format
	}
}

func normalFormat(f string) string {
	if f == "" {
		return "webp"
	}
	if f == "jpeg" {
		return "jpg"
	}
	return f
}

func fileExt(contentType string) string {
	switch contentType {
	case "image/png":
		return "png"
	case "image/jpeg":
		return "jpg"
	default:
		return "webp"
	}
}

func sanitizeFilename(name string) string {
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == ' ':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.TrimSpace(b.String())
	if out == "" {
		return "artdream-image"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}
