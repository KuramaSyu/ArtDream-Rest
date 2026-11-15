package web

import (
	"encoding/json"
	"fmt"
	"math"
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
	for _, k := range []string{"metadata", "meta", "context"} {
		if rawMeta, ok := raw[k]; ok {
			_ = json.Unmarshal(rawMeta, &r.Metadata)
			r.MetadataProvided = true
			break
		}
	}
	return nil
}

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

	v := &store.Vote{
		ModeID:       mode.ID,
		ModeName:     mode.Name,
		LeftImageID:  req.LeftImageID,
		RightImageID: req.RightImageID,
		MinValue:     minV,
		MaxValue:     maxV,
		UserID:       &user.ID,
		Username:     user.Username,
		DurationMs:   req.DurationMs,
		Metadata:     encodeMetadata(req.Metadata),
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
	c.JSON(http.StatusCreated, voteToDTO(v, images))
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

	// deranked is an internal sort key; it is never serialized publicly.
	deranked bool
}

type rankResponse struct {
	Time       string      `json:"time"`
	From       *string     `json:"from,omitempty"`
	To         *string     `json:"to,omitempty"`
	Amount     int         `json:"amount"`
	Offset     int         `json:"offset"`
	Total      int         `json:"total"`
	TotalVotes int         `json:"totalVotes"`
	Winners    []rankEntry `json:"winners"`
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
		Score:         math.Round(st.score*1000) / 1000,
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

func (s *Server) rankImages(c *gin.Context) {
	rng, err := parseRange(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	amount := parseAmount(c, 10, 100)
	offset := parseOffset(c)
	ctx := c.Request.Context()

	policy, err := imageRequestPolicy(c)
	if err != nil {
		asStoreError(c, err)
		return
	}

	votes, _, err := s.Store.ListVotes(ctx, store.VoteFilter{
		From:       rng.From,
		To:         rng.To,
		Mode:       trimTo(c.Query("mode"), 64),
		Author:     trimTo(c.Query("author"), 120),
		TitleRegex: trimTo(c.Query("titleRegex"), 200),
		Limit:      -1,
	})
	if err != nil {
		asStoreError(c, err)
		return
	}

	images, err := s.Store.GetImagesByIDs(ctx, collectVoteImageIDs(votes))
	if err != nil {
		serverError(c, err)
		return
	}

	stats, totalVotes := computeRankStats(votes, images, policy.Hidden)

	modRows, err := moderationRows(c)
	if err != nil {
		serverError(c, err)
		return
	}
	window := store.ModerationWindowForLabel(rng.Label)

	entries := make([]rankEntry, 0, len(stats))
	for _, st := range stats {
		mod, hasMod := store.ModerationFor(modRows, st.image.ID, window)
		if hasMod && mod.Hidden {
			continue // hidden for this ranking period
		}
		entry := rankEntryFor(st)
		entry.deranked = hasMod && mod.Deranked
		entries = append(entries, entry)
	}
	sortRankEntries(entries)
	for i := range entries {
		entries[i].Rank = i + 1
	}

	total := len(entries)
	if offset > len(entries) {
		offset = len(entries)
	}
	entries = entries[offset:]
	if len(entries) > amount {
		entries = entries[:amount]
	}

	resp := rankResponse{
		Time:       rng.Label,
		Amount:     amount,
		Offset:     offset,
		Total:      total,
		TotalVotes: totalVotes,
		Winners:    entries,
	}
	if rng.From != nil {
		f := time.Unix(*rng.From, 0).UTC().Format(time.RFC3339)
		resp.From = &f
	}
	if rng.To != nil {
		t := time.Unix(*rng.To, 0).UTC().Format(time.RFC3339)
		resp.To = &t
	}
	c.JSON(http.StatusOK, resp)
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
