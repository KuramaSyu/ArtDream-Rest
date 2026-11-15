package web

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"artdream/internal/store"

	"github.com/gin-gonic/gin"
)

// voteRequest accepts a deliberately forgiving set of key spellings so the
// API stays friendly to experiments (leftImageId, left-img, left_img, ...).
type voteRequest struct {
	Mode             string
	ModeID           int64
	LeftImageID      string
	RightImageID     string
	Winner           string
	WinnerProvided   bool
	Value            *float64
	MinValue         *int
	MaxValue         *int
	Metadata         map[string]any
	MetadataProvided bool
	DurationMs       *int64
	AnonymousID      string
}

func firstString(m map[string]json.RawMessage, keys ...string) (string, bool) {
	for _, k := range keys {
		if raw, ok := m[k]; ok {
			var s string
			if json.Unmarshal(raw, &s) == nil {
				return s, true
			}
			var n int64
			if json.Unmarshal(raw, &n) == nil {
				return strconv.FormatInt(n, 10), true
			}
		}
	}
	return "", false
}

func firstInt(m map[string]json.RawMessage, keys ...string) (*int, bool) {
	for _, k := range keys {
		if raw, ok := m[k]; ok {
			var f float64
			if json.Unmarshal(raw, &f) == nil {
				n := int(math.Round(f))
				return &n, true
			}
		}
	}
	return nil, false
}

func firstInt64(m map[string]json.RawMessage, keys ...string) (*int64, bool) {
	for _, k := range keys {
		if raw, ok := m[k]; ok {
			var f float64
			if json.Unmarshal(raw, &f) == nil {
				n := int64(math.Round(f))
				return &n, true
			}
		}
	}
	return nil, false
}

func firstFloat(m map[string]json.RawMessage, keys ...string) (*float64, bool) {
	for _, k := range keys {
		if raw, ok := m[k]; ok {
			var f float64
			if json.Unmarshal(raw, &f) == nil {
				return &f, true
			}
		}
	}
	return nil, false
}

func (r *voteRequest) UnmarshalJSON(data []byte) error {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	if v, ok := firstString(raw, "mode", "modeName", "mode_name"); ok {
		r.Mode = v
	}
	if v, ok := firstString(raw, "modeId", "mode_id", "mode-id"); ok {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			r.ModeID = n
		}
	}
	if v, ok := firstString(raw, "leftImageId", "leftImageID", "left_image_id", "left-img", "left_img", "left", "imageA", "image-a", "a"); ok {
		r.LeftImageID = v
	}
	if v, ok := firstString(raw, "rightImageId", "rightImageID", "right_image_id", "right-img", "right_img", "right", "imageB", "image-b", "b"); ok {
		r.RightImageID = v
	}
	if v, ok := firstString(raw, "winner", "winnerSide", "winner_side", "winnerImageId", "winner_image_id", "winner-img"); ok {
		r.Winner = v
		r.WinnerProvided = true
	}
	if v, ok := firstFloat(raw, "value", "score", "rating"); ok {
		r.Value = v
	}
	if v, ok := firstInt(raw, "minValue", "min_value", "min-value", "min", "minimum"); ok {
		r.MinValue = v
	}
	if v, ok := firstInt(raw, "maxValue", "max_value", "max-value", "max", "maximum"); ok {
		r.MaxValue = v
	}
	if v, ok := firstInt64(raw, "durationMs", "duration_ms", "duration-ms", "timeMs", "time_ms", "elapsed"); ok {
		r.DurationMs = v
	}
	if v, ok := firstString(raw, "anonymousId", "anonymous_id", "voterId", "voter_id", "anonymousToken", "voterToken"); ok {
		r.AnonymousID = v
	}
	for _, k := range []string{"metadata", "meta", "context"} {
		if rawMeta, ok := raw[k]; ok {
			_ = json.Unmarshal(rawMeta, &r.Metadata)
			r.MetadataProvided = true
			break
		}
	}
	return nil
}

// listVoteModes godoc
// @Summary List vote modes
// @Description Returns the available voting scales (binary duel modes and value-scaled modes).
// @Tags rank
// @Produce json
// @Success 200 {array} store.VoteMode
// @Failure 500 {object} apiError
// @Router /rank/modes [get]
func (s *Server) listVoteModes(c *gin.Context) {
	modes, err := s.Store.ListVoteModes(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, modes)
}

func resolveVoteMode(c *gin.Context, modeID int64, modeName string) (*store.VoteMode, error) {
	ctx := c.Request.Context()
	if modeID > 0 {
		return sStore(c).GetVoteModeByID(ctx, modeID)
	}
	name := strings.TrimSpace(modeName)
	if name != "" {
		if id, err := strconv.ParseInt(name, 10, 64); err == nil && id > 0 {
			return sStore(c).GetVoteModeByID(ctx, id)
		}
		return sStore(c).GetVoteModeByName(ctx, name)
	}
	// Infer a sensible default when the client omits the mode.
	return sStore(c).GetVoteModeByName(ctx, "duel")
}

// castVote godoc
// @Summary Cast a vote
// @Description Compares two images. Authentication is optional; anonymous voters send a device-local `anonymousId` so their history can be claimed after sign-in. The body accepts the canonical field names shown below as well as several aliases (e.g. `left`, `right`, `imageA`, `imageB`, `score`). Binary modes use `winner` (left/right/tie); value-scaled modes use `value`.
// @Tags rank
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body VotePayload true "Vote details"
// @Success 201 {object} VoteDTO
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 404 {object} apiError
// @Failure 500 {object} apiError
// @Router /rank/vote [post]
func (s *Server) castVote(c *gin.Context) {
	user := currentUser(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	var req voteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Could not parse the vote payload.")
		return
	}
	req.LeftImageID = strings.TrimSpace(req.LeftImageID)
	req.RightImageID = strings.TrimSpace(req.RightImageID)
	if req.LeftImageID == "" || req.RightImageID == "" {
		badRequest(c, "Both leftImageId and rightImageId are required.")
		return
	}
	if req.LeftImageID == req.RightImageID {
		badRequest(c, "An image cannot compete against itself.")
		return
	}
	ctx := c.Request.Context()
	images, err := s.Store.GetImagesByIDs(ctx, []string{req.LeftImageID, req.RightImageID})
	if err != nil {
		serverError(c, err)
		return
	}
	if images[req.LeftImageID] == nil || images[req.RightImageID] == nil {
		notFound(c, "One of the images does not exist (or was deleted).")
		return
	}
	policy, err := visibilityPolicy(c)
	if err != nil {
		asStoreError(c, err)
		return
	}
	if policy.Hidden(images[req.LeftImageID]) || policy.Hidden(images[req.RightImageID]) {
		notFound(c, "One of the images is not available for voting.")
		return
	}

	mode, err := resolveVoteMode(c, req.ModeID, req.Mode)
	if err != nil {
		if err == store.ErrNotFound {
			badRequest(c, "Unknown vote mode. Use GET /rank/modes to see the available modes.")
			return
		}
		serverError(c, err)
		return
	}

	minV, maxV := mode.MinValue, mode.MaxValue
	if req.MinValue != nil {
		minV = *req.MinValue
	}
	if req.MaxValue != nil {
		maxV = *req.MaxValue
	}
	if maxV <= minV {
		badRequest(c, "maxValue must be greater than minValue.")
		return
	}

	anonymousID, validAnonymousID := normalizeAnonymousID(req.AnonymousID)
	if req.AnonymousID != "" && !validAnonymousID {
		badRequest(c, "anonymousId must be 16-128 letters, digits, dashes or underscores.")
		return
	}
	v := &store.Vote{
		ModeID:       mode.ID,
		ModeName:     mode.Name,
		LeftImageID:  req.LeftImageID,
		RightImageID: req.RightImageID,
		MinValue:     minV,
		MaxValue:     maxV,
		AnonymousID:  anonymousID,
		DurationMs:   req.DurationMs,
		Metadata:     encodeMetadata(req.Metadata),
	}
	if user != nil {
		v.UserID = &user.ID
		v.Username = user.Username
	} else {
		if anonymousID == "" {
			badRequest(c, "anonymousId is required when voting without an account.")
			return
		}
	}
	if req.MetadataProvided && len(v.Metadata) > 8<<10 {
		badRequest(c, "Metadata is too large (max 8 KiB).")
		return
	}
	if err := applyVoteOutcome(v, mode.IsBinary, req.Winner, req.Value); err != nil {
		badRequest(c, err.Error())
		return
	}
	if err := s.Store.CreateVote(ctx, v); err != nil {
		serverError(c, err)
		return
	}
	s.invalidateRankingCache()
	c.JSON(http.StatusCreated, voteToDTO(v, images))
}

func normalizeAnonymousID(raw string) (string, bool) {
	raw = strings.TrimSpace(raw)
	if len(raw) < 16 || len(raw) > 128 {
		return "", false
	}
	for _, ch := range raw {
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
		case ch >= '0' && ch <= '9':
		case ch == '-' || ch == '_':
		default:
			return "", false
		}
	}
	return raw, true
}

type voteClaimRequest struct {
	AnonymousID  string   `json:"anonymousId"`
	AnonymousIDs []string `json:"anonymousIds"`
	VoterID      string   `json:"voterId"`
	VoterIDs     []string `json:"voterIds"`
	Token        string   `json:"token"`
	Tokens       []string `json:"tokens"`
}

// claimVotes godoc
// @Summary Claim anonymous votes
// @Description Moves votes cast from this device's anonymous identities under the signed-in account. Clients send the device-local anonymous ids they kept in local storage.
// @Tags rank
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body voteClaimRequest true "Anonymous voter ids to claim"
// @Success 200 {object} map[string]int64
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 500 {object} apiError
// @Router /rank/claim [post]
func (s *Server) claimVotes(c *gin.Context) {
	user := currentUser(c)
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var req voteClaimRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Could not parse the claim payload.")
		return
	}
	rawIDs := append([]string{req.AnonymousID, req.VoterID, req.Token}, req.AnonymousIDs...)
	rawIDs = append(rawIDs, req.VoterIDs...)
	rawIDs = append(rawIDs, req.Tokens...)
	seen := make(map[string]struct{}, len(rawIDs))
	ids := make([]string, 0, len(rawIDs))
	for _, raw := range rawIDs {
		id, ok := normalizeAnonymousID(raw)
		if !ok {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			badRequest(c, "anonymousId must be 16-128 letters, digits, dashes or underscores.")
			return
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		badRequest(c, "Provide at least one anonymousId.")
		return
	}
	if len(ids) > 50 {
		badRequest(c, "At most 50 anonymous identities can be claimed at once.")
		return
	}
	claimed, err := s.Store.ClaimAnonymousVotes(c.Request.Context(), user.ID, user.Username, ids)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"claimed": claimed, "count": claimed})
}

// applyVoteOutcome derives winner/loser/value/weight from either a categorical
// winner ("left", "right", "tie" or an image id) or a numeric AHP value.
func applyVoteOutcome(v *store.Vote, binary bool, winner string, value *float64) error {
	if binary {
		side := strings.ToLower(strings.TrimSpace(winner))
		switch side {
		case "left", "l", v.LeftImageID:
			id := v.LeftImageID
			loser := v.RightImageID
			v.WinnerImageID = &id
			v.LoserImageID = &loser
			v.Weight = 1
			min := v.MinValue
			v.Value = &min
		case "right", "r", v.RightImageID:
			id := v.RightImageID
			loser := v.LeftImageID
			v.WinnerImageID = &id
			v.LoserImageID = &loser
			v.Weight = 1
			max := v.MaxValue
			v.Value = &max
		case "tie", "draw", "same", "":
			v.WinnerImageID = nil
			v.LoserImageID = nil
			v.Weight = 0
			v.Value = nil
		default:
			return fmt.Errorf("winner must be left, right, tie or one of the image ids")
		}
		return nil
	}

	if value == nil {
		return fmt.Errorf("value is required for value-scaled modes")
	}
	minV, maxV := float64(v.MinValue), float64(v.MaxValue)
	if *value < minV || *value > maxV {
		return fmt.Errorf("value must be between %d and %d", v.MinValue, v.MaxValue)
	}
	n := int(math.Round(*value))
	v.Value = &n

	mid := (minV + maxV) / 2
	delta := *value - mid
	halfRange := (maxV - minV) / 2
	if math.Abs(delta) < 0.5 {
		v.WinnerImageID = nil
		v.LoserImageID = nil
		v.Weight = 0
		return nil
	}
	weight := math.Abs(delta) / halfRange
	if weight > 1 {
		weight = 1
	}
	if weight < 0 {
		weight = 0
	}
	v.Weight = math.Round(weight*10000) / 10000
	if delta < 0 {
		id := v.LeftImageID
		loser := v.RightImageID
		v.WinnerImageID = &id
		v.LoserImageID = &loser
	} else {
		id := v.RightImageID
		loser := v.LeftImageID
		v.WinnerImageID = &id
		v.LoserImageID = &loser
	}
	return nil
}

func encodeMetadata(m map[string]any) string {
	if len(m) == 0 {
		return "{}"
	}
	b, err := json.Marshal(m)
	if err != nil {
		return "{}"
	}
	return string(b)
}

// --- ranking --------------------------------------------------------------

type rankEntry struct {
	Rank          int      `json:"rank"`
	ID            string   `json:"id"`
	URL           string   `json:"url"`
	Author        string   `json:"author"`
	Title         string   `json:"title"`
	Width         int      `json:"width"`
	Height        int      `json:"height"`
	Wins          int      `json:"wins"`
	Losses        int      `json:"losses"`
	Ties          int      `json:"ties"`
	Votes         int      `json:"votes"`
	Score         float64  `json:"score"`
	WinRate       float64  `json:"winRate"`
	WinPercentage float64  `json:"winPercentage"`
	Image         imageDTO `json:"image"`
	// Rotated marks a deterministic fallback image added by rank rotation.
	Rotated bool `json:"rotated,omitempty"`
	// Random marks an image selected by the smart endpoint's random fallback.
	Random bool `json:"random,omitempty"`

	// deranked is an internal sort key; it is never serialized publicly.
	deranked bool
}

type rankRotationInfo struct {
	Enabled      bool    `json:"enabled"`
	Percent      int     `json:"percent"`
	PreviousFrom *string `json:"previousFrom,omitempty"`
	PreviousTo   *string `json:"previousTo,omitempty"`
	Required     int     `json:"required"`
	Changed      int     `json:"changed"`
	Satisfied    bool    `json:"satisfied"`
}

type rankResponse struct {
	Time string `json:"time"`
	// Strategy is the aggregation strategy that produced the scores.
	// AHP (default) returns normalized priority weights summing to 1.
	Strategy string `json:"strategy"`
	// Source is set by the smart endpoint: day, week, month, year or random.
	Source     string            `json:"source,omitempty"`
	From       *string           `json:"from,omitempty"`
	To         *string           `json:"to,omitempty"`
	Amount     int               `json:"amount"`
	Offset     int               `json:"offset"`
	Total      int               `json:"total"`
	TotalVotes int               `json:"totalVotes"`
	Winners    []rankEntry       `json:"winners"`
	Rotation   *rankRotationInfo `json:"rotation,omitempty"`
}

// rankStats is the shared aggregation result for public and admin rankings.
type rankStats struct {
	image  *store.Image
	wins   int
	losses int
	ties   int
	votes  int
	score  float64
}

// computeRankStats turns a vote stream into per-image totals. skip removes
// images hidden by the admin allow/deny lists; nil means "count everything".
func computeRankStats(votes []store.Vote, images map[string]*store.Image, skip func(*store.Image) bool) ([]*rankStats, int) {
	byID := make(map[string]*rankStats, len(images))
	ensure := func(img *store.Image) *rankStats {
		st := byID[img.ID]
		if st == nil {
			st = &rankStats{image: img}
			byID[img.ID] = st
		}
		return st
	}

	totalVotes := 0
	for i := range votes {
		v := &votes[i]
		left, right := images[v.LeftImageID], images[v.RightImageID]
		if left == nil || right == nil {
			continue // image deleted: its history no longer counts
		}
		if skip != nil && (skip(left) || skip(right)) {
			continue // image hidden by an admin filter
		}
		totalVotes++
		ls, rs := ensure(left), ensure(right)
		ls.votes++
		rs.votes++
		switch {
		case v.WinnerImageID == nil:
			ls.ties++
			rs.ties++
			ls.score += 0.5
			rs.score += 0.5
		case *v.WinnerImageID == left.ID:
			ls.wins++
			rs.losses++
			ls.score += v.Weight
		default:
			rs.wins++
			ls.losses++
			rs.score += v.Weight
		}
	}

	out := make([]*rankStats, 0, len(byID))
	for _, st := range byID {
		out = append(out, st)
	}
	return out, totalVotes
}

func winRateOf(st *rankStats) float64 {
	if st.votes == 0 {
		return 0
	}
	return (float64(st.wins) + 0.5*float64(st.ties)) / float64(st.votes)
}

func rankEntryFor(st *rankStats) rankEntry {
	dto := imageToDTO(st.image)
	rate := winRateOf(st)
	return rankEntry{
		ID:            st.image.ID,
		URL:           dto.URL,
		Author:        st.image.Author,
		Title:         st.image.Title,
		Width:         st.image.Width,
		Height:        st.image.Height,
		Wins:          st.wins,
		Losses:        st.losses,
		Ties:          st.ties,
		Votes:         st.votes,
		Score:         math.Round(st.score*1e6) / 1e6,
		WinRate:       math.Round(rate*1000) / 1000,
		WinPercentage: math.Round(rate*1000) / 10,
		Image:         dto,
	}
}

func sortRankStats(stats []*rankStats) {
	sort.Slice(stats, func(i, j int) bool {
		a, b := stats[i], stats[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if a.wins != b.wins {
			return a.wins > b.wins
		}
		ra, rb := winRateOf(a), winRateOf(b)
		if ra != rb {
			return ra > rb
		}
		if a.votes != b.votes {
			return a.votes > b.votes
		}
		return a.image.Title < b.image.Title
	})
}

func sortRankEntries(entries []rankEntry) {
	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		// Deranked images remain visible but always trail regular entries.
		if a.deranked != b.deranked {
			return !a.deranked
		}
		if a.Score != b.Score {
			return a.Score > b.Score
		}
		if a.Wins != b.Wins {
			return a.Wins > b.Wins
		}
		if a.WinRate != b.WinRate {
			return a.WinRate > b.WinRate
		}
		if a.Votes != b.Votes {
			return a.Votes > b.Votes
		}
		return a.Title < b.Title
	})
}

// rankImages godoc
// @Summary Get the public ranking
// @Description Aggregates votes into a per-image leaderboard for the requested time window, honoring admin hide/derank decisions. Authentication is optional; it enables the mine/uploadedBy filters.
// @Tags rank
// @Produce json
// @Param time query string false "Time window" Enums(day, week, month, year, all, custom)
// @Param seconds query int false "Custom window size in seconds (when time=custom)"
// @Param from query string false "Custom range start (RFC3339 or unix seconds)"
// @Param to query string false "Custom range end (RFC3339 or unix seconds)"
// @Param amount query int false "Page size (default 10, max 100)"
// @Param offset query int false "Pagination offset"
// @Param strategy query string false "Ranking strategy (default ahp)" Enums(ahp, legacy)
// @Param rankingStrategy query string false "Alias for strategy"
// @Param mode query string false "Vote mode name filter"
// @Param author query string false "Author filter"
// @Param titleRegex query string false "Title regex filter"
// @Param visibility query string false "Visibility filter" Enums(all, public, private)
// @Param service query string false "Public service name filter"
// @Param uploadedBy query []string false "Uploader IDs or usernames (comma separated, 'me' allowed)" collectionFormat(csv)
// @Param mine query bool false "Only images uploaded by the signed-in user"
// @Param includeAuthors query []string false "Author allow-list regexes" collectionFormat(csv)
// @Param excludeAuthors query []string false "Author deny-list regexes" collectionFormat(csv)
// @Param includeTitles query []string false "Title allow-list regexes" collectionFormat(csv)
// @Param excludeTitles query []string false "Title deny-list regexes" collectionFormat(csv)
// @Param exclude query []string false "Image IDs to exclude from the fallback pool" collectionFormat(csv)
// @Param rotation query bool false "Override the configured rotation of quiet ranking pages"
// @Param rankRotation query bool false "Alias for rotation"
// @Param rotationPercent query int false "Percentage of a page that should change between periods (0-100)"
// @Param rankRotationPercent query int false "Alias for rotationPercent"
// @Param changePercent query int false "Alias for rotationPercent"
// @Success 200 {object} rankResponse
// @Failure 400 {object} apiError
// @Failure 500 {object} apiError
// @Router /images/rank [get]
func (s *Server) rankImages(c *gin.Context) {
	rng, err := parseRange(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	amount := parseAmount(c, 10, 100)
	offset := parseOffset(c)
	ctx := c.Request.Context()

	cfg, err := s.Store.GetAppConfig(ctx)
	if err != nil {
		serverError(c, err)
		return
	}
	req, err := imageRequestFromQuery(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	policy, err := store.CompileImageRequest(cfg, req)
	if err != nil {
		asStoreError(c, err)
		return
	}

	modRows, err := moderationRows(c)
	if err != nil {
		serverError(c, err)
		return
	}
	window := store.ModerationWindowForLabel(rng.Label)
	exclude := excludeIDSet(c)
	strategy, err := resolveRankingStrategy(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	baseFilter := store.VoteFilter{
		Mode:       trimTo(c.Query("mode"), 64),
		Author:     trimTo(c.Query("author"), 120),
		TitleRegex: trimTo(c.Query("titleRegex"), 200),
		Limit:      -1,
	}
	allEntries, totalVotes, err := s.rankEntriesForWindow(ctx, rng, baseFilter, policy, modRows, window, exclude, strategy)
	if err != nil {
		asStoreError(c, err)
		return
	}
	total := len(allEntries)
	currentPage := sliceRankEntries(allEntries, offset, amount)

	rotationEnabled, rotationPercent, err := rankRotationSettings(c, cfg)
	if err != nil {
		badRequest(c, err.Error())
		return
	}

	resp := rankResponse{
		Time:       rng.Label,
		Strategy:   strategy.Name(),
		Amount:     amount,
		Offset:     offset,
		Total:      total,
		TotalVotes: totalVotes,
	}
	if rng.From != nil {
		resp.From = formatRangeTime(rng.From)
	}
	if rng.To != nil {
		resp.To = formatRangeTime(rng.To)
	}

	if rotationEnabled {
		info := &rankRotationInfo{
			Enabled: true,
			Percent: rotationPercent,
		}

		var pool []store.Image
		poolLoaded := false
		loadPool := func() error {
			if poolLoaded {
				return nil
			}
			pool, err = s.rankCandidatePool(ctx, c, req, cfg, policy, modRows, window, exclude)
			poolLoaded = true
			return err
		}

		var baseline map[string]struct{}
		var previousPage []rankEntry
		if previousRange, ok := previousRangeWindow(rng); ok {
			info.PreviousFrom = formatRangeTime(previousRange.From)
			info.PreviousTo = formatRangeTime(previousRange.To)
			previousEntries, _, err := s.rankEntriesForWindow(ctx, previousRange, baseFilter, policy, modRows, window, exclude, strategy)
			if err != nil {
				asStoreError(c, err)
				return
			}
			previousPage = sliceRankEntries(previousEntries, offset, amount)
			baseline = rankIDSet(previousPage)

			// Reconstruct the likely previous rotated page so quiet periods do
			// not keep selecting the same fallback images.
			if len(previousPage) < amount {
				if err := loadPool(); err != nil {
					serverError(c, err)
					return
				}
				ordered := rotationOrder(pool, previousRange, amount, offset)
				for i := range ordered {
					if len(previousPage) >= amount {
						break
					}
					img := &ordered[i]
					if _, seen := baseline[img.ID]; seen {
						continue
					}
					baseline[img.ID] = struct{}{}
					previousPage = append(previousPage, syntheticRankEntry(img))
				}
			}
		} else {
			baseline = make(map[string]struct{})
		}

		if err := loadPool(); err != nil {
			serverError(c, err)
			return
		}
		currentPage, info.Changed, info.Required = applyRankRotation(
			currentPage, allEntries, offset, amount, baseline, pool, rng, rotationPercent,
		)
		// If the only possible candidates are already in the previous page,
		// showing the previous page is better than returning nothing at all.
		if len(currentPage) == 0 && len(previousPage) > 0 {
			currentPage = previousPage
			info.Changed = countRankOutside(currentPage, baseline)
			info.Required = percentRequired(len(currentPage), rotationPercent)
		}
		info.Satisfied = info.Changed >= info.Required
		resp.Rotation = info
	}

	for i := range currentPage {
		currentPage[i].Rank = offset + i + 1
	}
	if minimum := offset + len(currentPage); total < minimum {
		total = minimum
	}
	resp.Total = total
	resp.Winners = currentPage
	c.JSON(http.StatusOK, resp)
}

// --- rank rotation --------------------------------------------------------

// rankEntriesForWindow computes the fully sorted ranking for one time window.
// The filter and visibility policy are reused for the previous period so every
// endpoint parameter (author, title regex, mode, visibility, uploader, regex
// lists, excluded ids) applies consistently to both sides of the comparison.
func (s *Server) rankEntriesForWindow(ctx context.Context, rng rangeWindow, filter store.VoteFilter, policy *store.VisibilityPolicy, modRows []store.ImageModeration, window string, exclude map[string]struct{}, strategy rankingStrategy) ([]rankEntry, int, error) {
	filter.From = rng.From
	filter.To = rng.To
	cacheKey := rankingCacheKey(strategy, rng, filter, policy, modRows, window, exclude)
	if entries, totalVotes, ok := s.rankCache.Get(cacheKey); ok {
		return entries, totalVotes, nil
	}
	votes, _, err := s.Store.ListVotes(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	images, err := s.Store.GetImagesByIDs(ctx, collectVoteImageIDs(votes))
	if err != nil {
		return nil, 0, err
	}
	skip := func(img *store.Image) bool {
		if policy.Hidden(img) {
			return true
		}
		_, excluded := exclude[img.ID]
		return excluded
	}
	stats, totalVotes := strategy.Compute(votes, images, skip)

	entries := make([]rankEntry, 0, len(stats))
	for _, st := range stats {
		mod, hasMod := store.ModerationFor(modRows, st.image.ID, window)
		if hasMod && mod.Hidden {
			continue
		}
		entry := rankEntryFor(st)
		entry.deranked = hasMod && mod.Deranked
		entries = append(entries, entry)
	}
	sortRankEntries(entries)
	s.rankCache.Set(cacheKey, entries, totalVotes)
	return entries, totalVotes, nil
}

// rankCandidatePool returns every live image matching the request filters. It
// is used only when rotation is enabled and supplies zero-vote fallbacks when
// a period has too few votes to produce a fresh page.
func (s *Server) rankCandidatePool(ctx context.Context, c *gin.Context, req store.ImageRequest, cfg store.AppConfig, policy *store.VisibilityPolicy, modRows []store.ImageModeration, window string, exclude map[string]struct{}) ([]store.Image, error) {
	visibility := req.Visibility
	if visibility == "" {
		visibility = cfg.DefaultImageVisibility
	}
	if visibility == "" {
		visibility = store.VisibilityAll
	}
	images, _, err := s.Store.ListImages(ctx, store.ImageFilter{
		Author:     trimTo(c.Query("author"), 120),
		TitleRegex: trimTo(c.Query("titleRegex"), 200),
		Visibility: visibility,
		Service:    req.Service,
		UploadedBy: req.UploadedBy,
	})
	if err != nil {
		return nil, err
	}
	pool := make([]store.Image, 0, len(images))
	for i := range images {
		img := &images[i]
		if _, skipped := exclude[img.ID]; skipped {
			continue
		}
		if policy.Hidden(img) {
			continue
		}
		if mod, ok := store.ModerationFor(modRows, img.ID, window); ok && (mod.Hidden || mod.Deranked) {
			continue
		}
		pool = append(pool, *img)
	}
	return pool, nil
}

// applyRankRotation fills/replaces a page until at least percent of the
// returned entries are absent from the previous period's page.
func applyRankRotation(currentPage, allEntries []rankEntry, offset, amount int, baseline map[string]struct{}, pool []store.Image, rng rangeWindow, percent int) ([]rankEntry, int, int) {
	if baseline == nil {
		baseline = make(map[string]struct{})
	}
	final := append([]rankEntry(nil), currentPage...)
	used := rankIDSet(final)

	candidates := make([]rankEntry, 0, len(pool)+len(allEntries))
	// Prefer images that were already ranked in the current period but sit
	// beyond this page; they keep real win/loss numbers.
	if start := offset + amount; start < len(allEntries) {
		for i := start; i < len(allEntries); i++ {
			entry := allEntries[i]
			if _, stale := baseline[entry.ID]; stale {
				continue
			}
			if _, exists := used[entry.ID]; exists {
				continue
			}
			used[entry.ID] = struct{}{}
			candidates = append(candidates, entry)
		}
	}
	// Then fall back to the deterministic, window-seeded library rotation.
	ordered := rotationOrder(pool, rng, amount, offset)
	for i := range ordered {
		img := &ordered[i]
		if _, stale := baseline[img.ID]; stale {
			continue
		}
		if _, exists := used[img.ID]; exists {
			continue
		}
		used[img.ID] = struct{}{}
		candidates = append(candidates, syntheticRankEntry(img))
	}

	candidateIndex := 0
	for len(final) < amount && candidateIndex < len(candidates) {
		final = append(final, candidates[candidateIndex])
		candidateIndex++
	}
	sortRankEntries(final)
	required := percentRequired(len(final), percent)
	changed := countRankOutside(final, baseline)

	// Replace the lowest-ranked entries that are still identical to the
	// previous period until the minimum change share is met.
	for changed < required {
		replaceAt := -1
		for i := len(final) - 1; i >= 0; i-- {
			if _, stale := baseline[final[i].ID]; stale {
				replaceAt = i
				break
			}
		}
		if replaceAt < 0 || candidateIndex >= len(candidates) {
			break
		}
		final[replaceAt] = candidates[candidateIndex]
		candidateIndex++
		changed++
	}
	sortRankEntries(final)
	return final, changed, required
}

func rankRotationSettings(c *gin.Context, cfg store.AppConfig) (bool, int, error) {
	enabled := cfg.RankRotationEnabled
	for _, key := range []string{"rotation", "rankRotation"} {
		if value, ok := parseOptionalBool(c.Query(key)); ok {
			enabled = value
			break
		}
	}

	percent := cfg.RankRotationPercent
	if percent < 0 || percent > 100 {
		percent = 60
	}
	raw := firstNonEmpty(c.Query("rotationPercent"), c.Query("rankRotationPercent"), c.Query("changePercent"))
	if raw != "" {
		n, err := strconv.Atoi(strings.TrimSpace(raw))
		if err != nil || n < 0 || n > 100 {
			return false, 0, fmt.Errorf("rotationPercent must be between 0 and 100")
		}
		percent = n
	}
	return enabled, percent, nil
}

func parseOptionalBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func excludeIDSet(c *gin.Context) map[string]struct{} {
	out := make(map[string]struct{})
	for _, value := range c.QueryArray("exclude") {
		for _, id := range strings.Split(value, ",") {
			if id = strings.TrimSpace(id); id != "" {
				out[id] = struct{}{}
			}
		}
	}
	return out
}

func previousRangeWindow(rng rangeWindow) (rangeWindow, bool) {
	if rng.From == nil || rng.To == nil {
		return rangeWindow{}, false
	}
	duration := *rng.To - *rng.From
	if duration <= 0 {
		return rangeWindow{}, false
	}
	previousTo := *rng.From - 1
	previousFrom := previousTo - duration
	return rangeWindow{Label: rng.Label, From: &previousFrom, To: &previousTo}, true
}

func sliceRankEntries(entries []rankEntry, offset, amount int) []rankEntry {
	if offset < 0 {
		offset = 0
	}
	if offset > len(entries) {
		offset = len(entries)
	}
	page := entries[offset:]
	if len(page) > amount {
		page = page[:amount]
	}
	return append([]rankEntry(nil), page...)
}

func rankIDSet(entries []rankEntry) map[string]struct{} {
	out := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		out[entry.ID] = struct{}{}
	}
	return out
}

func countRankOutside(entries []rankEntry, baseline map[string]struct{}) int {
	changed := 0
	for _, entry := range entries {
		if _, stale := baseline[entry.ID]; !stale {
			changed++
		}
	}
	return changed
}

func percentRequired(count, percent int) int {
	if count <= 0 || percent <= 0 {
		return 0
	}
	if percent > 100 {
		percent = 100
	}
	return int(math.Ceil(float64(count) * float64(percent) / 100.0))
}

// rotationOrder returns a stable cyclic ordering of the candidate pool. The
// start advances by the page size for every equivalent time window, which
// makes consecutive quiet periods use different slices of the library instead
// of re-shuffling from scratch.
func rotationOrder(pool []store.Image, rng rangeWindow, amount, offset int) []store.Image {
	if len(pool) == 0 {
		return nil
	}
	if amount < 1 {
		amount = 1
	}
	ordered := append([]store.Image(nil), pool...)
	var index int64
	if rng.From != nil && rng.To != nil {
		if duration := *rng.To - *rng.From; duration > 0 {
			index = *rng.From / duration
		}
	} else if rng.From != nil {
		index = *rng.From
	}
	if index < 0 {
		index = -index
	}
	start := int((index*int64(amount) + int64(offset)) % int64(len(ordered)))
	if start < 0 {
		start += len(ordered)
	}
	out := make([]store.Image, 0, len(ordered))
	out = append(out, ordered[start:]...)
	out = append(out, ordered[:start]...)
	return out
}

func syntheticRankEntry(img *store.Image) rankEntry {
	entry := rankEntryFor(&rankStats{image: img})
	entry.Rotated = true
	return entry
}

func formatRangeTime(v *int64) *string {
	if v == nil {
		return nil
	}
	formatted := time.Unix(*v, 0).UTC().Format(time.RFC3339)
	return &formatted
}

// smartImages godoc
// @Summary Smart image feed with ranking fallback
// @Description Returns the strongest available ranking for the same filters: day, then week, month, year, and finally random images when no ranking exists at the requested offset. The default AHP strategy returns normalized priority weights (score, sum 1).
// @Tags rank
// @Produce json
// @Param amount query int false "Page size (default 10, max 100)"
// @Param offset query int false "Pagination offset"
// @Param strategy query string false "Ranking strategy (default ahp)" Enums(ahp, legacy)
// @Param rankingStrategy query string false "Alias for strategy"
// @Param mode query string false "Vote mode name filter"
// @Param author query string false "Author filter"
// @Param titleRegex query string false "Title regex filter"
// @Param visibility query string false "Visibility filter" Enums(all, public, private)
// @Param service query string false "Public service name filter"
// @Param uploadedBy query []string false "Uploader IDs or usernames (comma separated, 'me' allowed)" collectionFormat(csv)
// @Param mine query bool false "Only images uploaded by the signed-in user"
// @Param includeAuthors query []string false "Author allow-list regexes" collectionFormat(csv)
// @Param excludeAuthors query []string false "Author deny-list regexes" collectionFormat(csv)
// @Param includeTitles query []string false "Title allow-list regexes" collectionFormat(csv)
// @Param excludeTitles query []string false "Title deny-list regexes" collectionFormat(csv)
// @Param exclude query []string false "Image IDs to exclude from every ladder step" collectionFormat(csv)
// @Success 200 {object} rankResponse
// @Failure 400 {object} apiError
// @Failure 500 {object} apiError
// @Router /images/smart [get]
func (s *Server) smartImages(c *gin.Context) {
	amount := parseAmount(c, 10, 100)
	offset := parseOffset(c)
	now := time.Now().UTC()
	ctx := c.Request.Context()

	cfg, err := s.Store.GetAppConfig(ctx)
	if err != nil {
		serverError(c, err)
		return
	}
	req, err := imageRequestFromQuery(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	policy, err := store.CompileImageRequest(cfg, req)
	if err != nil {
		asStoreError(c, err)
		return
	}
	modRows, err := moderationRows(c)
	if err != nil {
		serverError(c, err)
		return
	}
	exclude := excludeIDSet(c)
	strategy, err := resolveRankingStrategy(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	baseFilter := store.VoteFilter{
		Mode:       trimTo(c.Query("mode"), 64),
		Author:     trimTo(c.Query("author"), 120),
		TitleRegex: trimTo(c.Query("titleRegex"), 200),
		Limit:      -1,
	}

	for _, label := range []string{"day", "week", "month", "year"} {
		rng := rollingRangeForLabel(label, now)
		window := store.ModerationWindowForLabel(label)
		entries, totalVotes, err := s.rankEntriesForWindow(ctx, rng, baseFilter, policy, modRows, window, exclude, strategy)
		if err != nil {
			asStoreError(c, err)
			return
		}
		page := sliceRankEntries(entries, offset, amount)
		if len(page) == 0 {
			continue
		}
		for i := range page {
			page[i].Rank = offset + i + 1
		}
		resp := rankResponse{
			Time:       label,
			Strategy:   strategy.Name(),
			Source:     label,
			From:       formatRangeTime(rng.From),
			To:         formatRangeTime(rng.To),
			Amount:     amount,
			Offset:     offset,
			Total:      len(entries),
			TotalVotes: totalVotes,
			Winners:    page,
		}
		c.JSON(http.StatusOK, resp)
		return
	}

	// No ranking exists even for the whole year: return a stable daily random
	// page that still honors the same filters, exclusions and pagination.
	pool, err := s.rankCandidatePool(ctx, c, req, cfg, policy, modRows, store.ModerationAll, exclude)
	if err != nil {
		serverError(c, err)
		return
	}
	page := smartRandomPage(pool, now, offset, amount)
	for i := range page {
		page[i].Rank = offset + i + 1
	}
	c.JSON(http.StatusOK, rankResponse{
		Time:     "random",
		Strategy: strategy.Name(),
		Source:   "random",
		Amount:   amount,
		Offset:   offset,
		Total:    len(pool),
		Winners:  page,
	})
}

func rollingRangeForLabel(label string, now time.Time) rangeWindow {
	to := now.Unix()
	var from int64
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "day":
		from = now.Add(-rangeDay).Unix()
	case "week":
		from = now.Add(-rangeWeek).Unix()
	case "month":
		from = now.Add(-rangeMonth).Unix()
	case "year":
		from = now.Add(-rangeYear).Unix()
	default:
		from = 0
	}
	return rangeWindow{Label: label, From: &from, To: &to}
}

func smartRandomPage(pool []store.Image, now time.Time, offset, amount int) []rankEntry {
	if len(pool) == 0 || offset < 0 || offset >= len(pool) || amount <= 0 {
		return []rankEntry{}
	}
	shuffled := append([]store.Image(nil), pool...)
	// Stable per UTC day: a page keeps its order during the day, while the
	// random fallback gets a fresh order tomorrow.
	seed := now.Unix() / int64((24 * time.Hour).Seconds())
	random := rand.New(rand.NewSource(seed))
	random.Shuffle(len(shuffled), func(i, j int) {
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	})

	window := rollingRangeForLabel("day", now)
	ordered := rotationOrder(shuffled, window, amount, offset)
	limit := amount
	if remaining := len(pool) - offset; limit > remaining {
		limit = remaining
	}
	if limit > len(ordered) {
		limit = len(ordered)
	}
	out := make([]rankEntry, 0, limit)
	for i := 0; i < limit; i++ {
		img := &ordered[i]
		entry := syntheticRankEntry(img)
		entry.Rotated = false
		entry.Random = true
		out = append(out, entry)
	}
	return out
}

func (s *Server) myVotes(c *gin.Context) {
	user := currentUser(c)
	rng, err := parseRange(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	votes, total, err := s.Store.ListVotes(c.Request.Context(), store.VoteFilter{
		From:     rng.From,
		To:       rng.To,
		Username: user.Username,
		Limit:    parseAmount(c, 50, 200),
		Offset:   parseOffset(c),
	})
	if err != nil {
		asStoreError(c, err)
		return
	}
	images, err := s.Store.GetImagesByIDs(c.Request.Context(), collectVoteImageIDs(votes))
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": votesToDTO(votes, images), "total": total})
}
