package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"artdream/internal/imageproc"
	"artdream/internal/store"

	"github.com/gin-gonic/gin"
)

// Server bundles the dependencies of the REST API and the optional SPA host.
type Server struct {
	Store          *store.Store
	Renderer       *imageproc.Renderer
	DataDir        string
	UploadDir      string
	FrontendDir    string
	MaxUploadBytes int64
	SessionTTL     time.Duration
	AllowedOrigins []string

	rankCache   *rankingCache
	bootstrapMu sync.Mutex
}

// NewServer constructs a Server with sane defaults.
func NewServer(st *store.Store, renderer *imageproc.Renderer, dataDir, uploadDir, frontendDir string, origins []string) *Server {
	return &Server{
		Store:          st,
		Renderer:       renderer,
		DataDir:        dataDir,
		UploadDir:      uploadDir,
		FrontendDir:    frontendDir,
		MaxUploadBytes: 30 << 20, // 30 MiB
		SessionTTL:     30 * 24 * time.Hour,
		AllowedOrigins: origins,
		rankCache:      newRankingCache(),
	}
}

type apiError struct {
	Error errBody `json:"error"`
}

type errBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func fail(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, apiError{errBody{Code: code, Message: message}})
}

func badRequest(c *gin.Context, message string) {
	fail(c, http.StatusBadRequest, "bad_request", message)
}

func notFound(c *gin.Context, message string) { fail(c, http.StatusNotFound, "not_found", message) }

func forbidden(c *gin.Context, message string) { fail(c, http.StatusForbidden, "forbidden", message) }

func unauthorized(c *gin.Context, message string) {
	fail(c, http.StatusUnauthorized, "unauthorized", message)
}

func conflict(c *gin.Context, message string) { fail(c, http.StatusConflict, "conflict", message) }

func serverError(c *gin.Context, err error) {
	log.Printf("api error: %v", err)
	fail(c, http.StatusInternalServerError, "internal_error", "Something went wrong on our side.")
}

func currentUser(c *gin.Context) *store.User {
	v, ok := c.Get(ctxUserKey)
	if !ok {
		return nil
	}
	u, _ := v.(*store.User)
	return u
}

func isAdmin(c *gin.Context) bool {
	u := currentUser(c)
	return u != nil && u.IsAdmin
}

func parseIDParam(c *gin.Context, name string) (int64, bool) {
	id, err := strconv.ParseInt(c.Param(name), 10, 64)
	if err != nil || id <= 0 {
		badRequest(c, "Invalid "+name+".")
		return 0, false
	}
	return id, true
}

func parseAmount(c *gin.Context, fallback, maxV int) int {
	raw := c.Query("amount")
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return fallback
	}
	if n > maxV {
		return maxV
	}
	return n
}

func parseOffset(c *gin.Context) int {
	n, err := strconv.Atoi(c.Query("offset"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func trimTo(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len([]rune(s)) > maxLen {
		r := []rune(s)
		s = string(r[:maxLen])
	}
	return s
}

// rangeWindow describes an inclusive unix-second time window.
type rangeWindow struct {
	Label string
	From  *int64
	To    *int64
}

const (
	rangeDay   = 24 * time.Hour
	rangeWeek  = 7 * 24 * time.Hour
	rangeMonth = 30 * 24 * time.Hour
	rangeYear  = 365 * 24 * time.Hour
)

// parseRange reads ?time=day|week|month|year|all|custom. Custom ranges accept
// ?seconds=N or an explicit ?from=&to= pair (RFC3339 or unix seconds).
func parseRange(c *gin.Context) (rangeWindow, error) {
	label := strings.ToLower(strings.TrimSpace(c.DefaultQuery("time", "all")))
	now := time.Now().UTC()
	to := now.Unix()
	from := int64(0)

	switch label {
	case "", "all":
		return rangeWindow{Label: "all"}, nil
	case "day":
		from = now.Add(-rangeDay).Unix()
	case "week":
		from = now.Add(-rangeWeek).Unix()
	case "month":
		from = now.Add(-rangeMonth).Unix()
	case "year":
		from = now.Add(-rangeYear).Unix()
	case "custom":
		if raw := c.Query("seconds"); raw != "" {
			seconds, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || seconds <= 0 {
				return rangeWindow{}, fmt.Errorf("seconds must be a positive integer")
			}
			from = now.Add(-time.Duration(seconds) * time.Second).Unix()
		} else {
			var err error
			from, err = parseTimeParam(c.Query("from"), now.Add(-rangeDay).Unix())
			if err != nil {
				return rangeWindow{}, fmt.Errorf("invalid from: %w", err)
			}
			to, err = parseTimeParam(c.Query("to"), now.Unix())
			if err != nil {
				return rangeWindow{}, fmt.Errorf("invalid to: %w", err)
			}
			if from > to {
				return rangeWindow{}, fmt.Errorf("from must be before to")
			}
		}
		return rangeWindow{Label: "custom", From: &from, To: &to}, nil
	default:
		return rangeWindow{}, fmt.Errorf("time must be one of day, week, month, year, all, custom")
	}

	return rangeWindow{Label: label, From: &from, To: &to}, nil
}

func parseTimeParam(raw string, fallback int64) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return n, nil
	}
	t, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		return 0, err
	}
	return t.UTC().Unix(), nil
}

// filterFromQuery builds the shared admin filter set from query parameters.
func filterFromQuery(c *gin.Context) (store.VoteFilter, store.ImageFilter, rangeWindow, error) {
	rng, err := parseRange(c)
	if err != nil {
		return store.VoteFilter{}, store.ImageFilter{}, rng, err
	}
	amount := parseAmount(c, 50, 500)
	offset := parseOffset(c)
	vf := store.VoteFilter{
		From:       rng.From,
		To:         rng.To,
		Username:   trimTo(c.Query("user"), 120),
		Mode:       trimTo(c.Query("mode"), 64),
		ImageID:    trimTo(c.Query("imageId"), 64),
		Author:     trimTo(c.Query("author"), 120),
		TitleRegex: trimTo(c.Query("titleRegex"), 200),
		Limit:      amount,
		Offset:     offset,
	}
	if vf.Username == "" {
		vf.Username = trimTo(c.Query("username"), 120)
	}
	if vf.Author == "" {
		vf.Author = trimTo(c.Query("authorName"), 120)
	}
	uploadedBy, err := parseUploaderIDs(c)
	if err != nil {
		return store.VoteFilter{}, store.ImageFilter{}, rng, err
	}
	visibility, err := store.NormalizeVisibility(c.Query("visibility"))
	if err != nil {
		return store.VoteFilter{}, store.ImageFilter{}, rng, err
	}
	imgf := store.ImageFilter{
		Author:         vf.Author,
		TitleRegex:     vf.TitleRegex,
		Visibility:     visibility,
		Service:        trimTo(c.Query("service"), 120),
		UploadedBy:     uploadedBy,
		IncludeDeleted: c.Query("includeDeleted") == "true",
		Limit:          amount,
		Offset:         offset,
	}
	return vf, imgf, rng, nil
}

func queryPatterns(c *gin.Context, keys ...string) []string {
	var raw []string
	for _, key := range keys {
		raw = append(raw, c.QueryArray(key)...)
	}
	out := make([]string, 0, len(raw))
	for _, value := range raw {
		for _, part := range strings.Split(value, ",") {
			if part = strings.TrimSpace(part); part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func truthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// parseUploaderIDs reads uploadedBy/owner plus the mine convenience flag.
// Values may be numeric account ids or account usernames; "me" resolves to the
// signed-in account. This lets a client request only their own uploads or a
// hand-picked group of friends.
func parseUploaderIDs(c *gin.Context) ([]int64, error) {
	tokens := make([]string, 0)
	for _, key := range []string{"uploadedBy", "owner", "uploaders"} {
		for _, value := range c.QueryArray(key) {
			for _, part := range strings.Split(value, ",") {
				if part = strings.TrimSpace(part); part != "" {
					tokens = append(tokens, part)
				}
			}
		}
	}
	if truthy(c.Query("mine")) {
		tokens = append(tokens, "me")
	}
	if len(tokens) == 0 {
		return nil, nil
	}

	seen := make(map[int64]struct{}, len(tokens))
	ids := make([]int64, 0, len(tokens))
	add := func(id int64) {
		if id <= 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, token := range tokens {
		token = strings.TrimPrefix(strings.TrimSpace(token), "@")
		if strings.EqualFold(token, "me") {
			user := currentUser(c)
			if user == nil {
				return nil, fmt.Errorf("sign in to use uploadedBy=me or mine=true")
			}
			add(user.ID)
			continue
		}
		if id, err := strconv.ParseInt(token, 10, 64); err == nil {
			add(id)
			continue
		}
		user, err := sStore(c).GetUserByUsername(c.Request.Context(), token)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return nil, fmt.Errorf("unknown uploader %q", token)
			}
			return nil, err
		}
		add(user.ID)
	}
	return ids, nil
}

// imageRequestFromQuery reads the per-request public/private, provenance,
// uploader and regex filters shared by the public image endpoints.
func imageRequestFromQuery(c *gin.Context) (store.ImageRequest, error) {
	uploadedBy, err := parseUploaderIDs(c)
	if err != nil {
		return store.ImageRequest{}, err
	}
	visibility := trimTo(c.Query("visibility"), 16)
	if visibility != "" {
		if _, err := store.NormalizeVisibility(visibility); err != nil {
			return store.ImageRequest{}, err
		}
	}
	return store.ImageRequest{
		Visibility:     visibility,
		Service:        trimTo(c.Query("service"), 120),
		UploadedBy:     uploadedBy,
		IncludeAuthors: queryPatterns(c, "includeAuthors", "includeAuthor"),
		ExcludeAuthors: queryPatterns(c, "excludeAuthors", "excludeAuthor", "blacklistAuthors"),
		IncludeTitles:  queryPatterns(c, "includeTitles", "includeTitle"),
		ExcludeTitles:  queryPatterns(c, "excludeTitles", "excludeTitle", "blacklistTitles"),
	}, nil
}

// --- DTOs -----------------------------------------------------------------

type imageDTO struct {
	ID                string     `json:"id"`
	URL               string     `json:"url"`
	Author            string     `json:"author"`
	Title             string     `json:"title"`
	OriginalFilename  string     `json:"originalFilename,omitempty"`
	MimeType          string     `json:"mimeType"`
	Width             int        `json:"width"`
	Height            int        `json:"height"`
	SizeBytes         int64      `json:"sizeBytes"`
	UploadedBy        *int64     `json:"uploadedBy,omitempty"`
	FromPublicService bool       `json:"fromPublicService"`
	ServiceName       string     `json:"serviceName,omitempty"`
	Visibility        string     `json:"visibility"`
	CreatedAt         time.Time  `json:"createdAt"`
	DeletedAt         *time.Time `json:"deletedAt,omitempty"`
}

func imageToDTO(img *store.Image) imageDTO {
	if img == nil {
		return imageDTO{}
	}
	return imageDTO{
		ID:                img.ID,
		URL:               "/api/v1/images/" + img.ID,
		Author:            img.Author,
		Title:             img.Title,
		OriginalFilename:  img.OriginalFilename,
		MimeType:          img.MimeType,
		Width:             img.Width,
		Height:            img.Height,
		SizeBytes:         img.SizeBytes,
		UploadedBy:        img.UploadedBy,
		FromPublicService: img.FromPublicService,
		ServiceName:       img.ServiceName,
		Visibility:        imageVisibility(img.FromPublicService),
		CreatedAt:         img.CreatedAt,
		DeletedAt:         img.DeletedAt,
	}
}

func imageVisibility(fromPublicService bool) string {
	if fromPublicService {
		return store.VisibilityPublic
	}
	return store.VisibilityPrivate
}

func imagesToDTO(images []store.Image) []imageDTO {
	out := make([]imageDTO, 0, len(images))
	for i := range images {
		out = append(out, imageToDTO(&images[i]))
	}
	return out
}

type voteUserDTO struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
}

type voteDTO struct {
	ID            int64           `json:"id"`
	ModeID        int64           `json:"modeId"`
	ModeName      string          `json:"modeName"`
	LeftImageID   string          `json:"leftImageId"`
	RightImageID  string          `json:"rightImageId"`
	LeftImage     *imageDTO       `json:"leftImage,omitempty"`
	RightImage    *imageDTO       `json:"rightImage,omitempty"`
	WinnerImageID *string         `json:"winnerImageId,omitempty"`
	LoserImageID  *string         `json:"loserImageId,omitempty"`
	WinnerImage   *imageDTO       `json:"winnerImage,omitempty"`
	LoserImage    *imageDTO       `json:"loserImage,omitempty"`
	Winner        string          `json:"winner"`
	Value         *int            `json:"value,omitempty"`
	MinValue      int             `json:"minValue"`
	MaxValue      int             `json:"maxValue"`
	Weight        float64         `json:"weight"`
	UserID        *int64          `json:"userId,omitempty"`
	Username      string          `json:"username"`
	User          *voteUserDTO    `json:"user,omitempty"`
	Metadata      json.RawMessage `json:"metadata"`
	DurationMs    *int64          `json:"durationMs,omitempty"`
	CreatedAt     time.Time       `json:"createdAt"`
}

func voteToDTO(v *store.Vote, images map[string]*store.Image) voteDTO {
	dto := voteDTO{
		ID:            v.ID,
		ModeID:        v.ModeID,
		ModeName:      v.ModeName,
		LeftImageID:   v.LeftImageID,
		RightImageID:  v.RightImageID,
		WinnerImageID: v.WinnerImageID,
		LoserImageID:  v.LoserImageID,
		Value:         v.Value,
		MinValue:      v.MinValue,
		MaxValue:      v.MaxValue,
		Weight:        v.Weight,
		UserID:        v.UserID,
		Username:      v.Username,
		DurationMs:    v.DurationMs,
		CreatedAt:     v.CreatedAt,
	}
	if img := images[v.LeftImageID]; img != nil {
		d := imageToDTO(img)
		dto.LeftImage = &d
	}
	if img := images[v.RightImageID]; img != nil {
		d := imageToDTO(img)
		dto.RightImage = &d
	}
	if v.WinnerImageID != nil {
		if img := images[*v.WinnerImageID]; img != nil {
			d := imageToDTO(img)
			dto.WinnerImage = &d
		}
		if *v.WinnerImageID == v.LeftImageID {
			dto.Winner = "left"
		} else {
			dto.Winner = "right"
		}
	} else {
		dto.Winner = "tie"
	}
	if v.LoserImageID != nil {
		if img := images[*v.LoserImageID]; img != nil {
			d := imageToDTO(img)
			dto.LoserImage = &d
		}
	}
	if v.UserID != nil {
		dto.User = &voteUserDTO{ID: *v.UserID, Username: v.Username}
	}
	dto.Metadata = metadataJSON(v.Metadata)
	return dto
}

func metadataJSON(raw string) json.RawMessage {
	raw = strings.TrimSpace(raw)
	if raw == "" || !json.Valid([]byte(raw)) {
		return json.RawMessage("{}")
	}
	return json.RawMessage(raw)
}

func votesToDTO(votes []store.Vote, images map[string]*store.Image) []voteDTO {
	out := make([]voteDTO, 0, len(votes))
	for i := range votes {
		out = append(out, voteToDTO(&votes[i], images))
	}
	return out
}

func collectVoteImageIDs(votes []store.Vote) []string {
	ids := make([]string, 0, len(votes)*2)
	for _, v := range votes {
		ids = append(ids, v.LeftImageID, v.RightImageID)
	}
	return ids
}

func loadImagesForVotes(c *gin.Context, votes []store.Vote) (map[string]*store.Image, error) {
	return sStore(c).GetImagesByIDs(c.Request.Context(), collectVoteImageIDs(votes))
}

// tiny helper to reach the store without threading it through every function.
func sStore(c *gin.Context) *store.Store {
	s := c.MustGet(ctxServerKey).(*Server)
	return s.Store
}

// visibilityPolicy loads and compiles the admin-managed regex lists. It is
// applied to public listings, random pairs and rankings only; library/admin
// endpoints that manage individual images stay unaffected.
func visibilityPolicy(c *gin.Context) (*store.VisibilityPolicy, error) {
	cfg, err := sStore(c).GetAppConfig(c.Request.Context())
	if err != nil {
		return nil, err
	}
	return store.CompileVisibility(cfg)
}

// imageRequestPolicy layers request filters on top of the admin-managed rules.
// It is used by the public image listings/rankings and defaults to the
// admin-configured public/private mode when the request omits visibility.
func imageRequestPolicy(c *gin.Context) (*store.VisibilityPolicy, error) {
	cfg, err := sStore(c).GetAppConfig(c.Request.Context())
	if err != nil {
		return nil, err
	}
	req, err := imageRequestFromQuery(c)
	if err != nil {
		return nil, err
	}
	return store.CompileImageRequest(cfg, req)
}

func moderationRows(c *gin.Context) ([]store.ImageModeration, error) {
	return sStore(c).ListImageModeration(c.Request.Context())
}

func rendererOf(c *gin.Context) *imageproc.Renderer {
	return c.MustGet(ctxServerKey).(*Server).Renderer
}

func asStoreError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		notFound(c, "Not found.")
	case errors.Is(err, store.ErrConflict):
		conflict(c, "That name is already taken.")
	case errors.Is(err, store.ErrInvalidFilter):
		badRequest(c, "Invalid filter. Check visibility, service, uploader and regex filters.")
	default:
		serverError(c, err)
	}
}
