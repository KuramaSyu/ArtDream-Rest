package web

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"artdream/internal/store"

	"github.com/gin-gonic/gin"
)

type adminRankEntry struct {
	Rank          int      `json:"rank"`
	ScoreRank     int      `json:"scoreRank"`
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

	Hidden           bool   `json:"hidden"`
	Deranked         bool   `json:"deranked"`
	Excluded         bool   `json:"excluded"`
	Override         bool   `json:"override"`
	Note             string `json:"note,omitempty"`
	ModerationWindow string `json:"moderationWindow"`
}

type adminRankResponse struct {
	Time          string           `json:"time"`
	Window        string           `json:"window"`
	From          *string          `json:"from,omitempty"`
	To            *string          `json:"to,omitempty"`
	Amount        int              `json:"amount"`
	Offset        int              `json:"offset"`
	Total         int              `json:"total"`
	TotalVotes    int              `json:"totalVotes"`
	HiddenCount   int              `json:"hiddenCount"`
	DerankedCount int              `json:"derankedCount"`
	ExcludedCount int              `json:"excludedCount"`
	Winners       []adminRankEntry `json:"winners"`
}

// adminRankImages returns exact per-image numbers for any period, including
// the moderation state that applies to that period. Unlike the public
// ranking, hidden images are included (flagged) unless includeHidden=false.
func (s *Server) adminRankImages(c *gin.Context) {
	rng, err := parseRange(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	amount := parseAmount(c, 100, 500)
	offset := parseOffset(c)
	ctx := c.Request.Context()

	policy, err := visibilityPolicy(c)
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

	votes, _, err := s.Store.ListVotes(ctx, store.VoteFilter{
		From:  rng.From,
		To:    rng.To,
		Mode:  trimTo(c.Query("mode"), 64),
		Limit: -1,
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

	stats, totalVotes := computeRankStats(votes, images, nil)

	// Optional view filters (do not change moderation state).
	author := strings.ToLower(trimTo(c.Query("author"), 120))
	var titleRe *regexp.Regexp
	if pattern := trimTo(c.Query("titleRegex"), 200); pattern != "" {
		titleRe, err = regexp.Compile("(?i)" + pattern)
		if err != nil {
			badRequest(c, "Invalid title regex.")
			return
		}
	}
	filtered := make([]*rankStats, 0, len(stats))
	for _, st := range stats {
		if author != "" && !strings.Contains(strings.ToLower(st.image.Author), author) {
			continue
		}
		if titleRe != nil && !titleRe.MatchString(st.image.Title) {
			continue
		}
		filtered = append(filtered, st)
	}

	// Pure performance ordering ignores hide/derank decisions.
	pure := append([]*rankStats(nil), filtered...)
	sortRankStats(pure)
	scoreRanks := make(map[string]int, len(pure))
	for i, st := range pure {
		scoreRanks[st.image.ID] = i + 1
	}

	includeHidden := strings.ToLower(c.DefaultQuery("includeHidden", "true")) != "false"
	entries := make([]adminRankEntry, 0, len(filtered))
	hiddenCount, derankedCount, excludedCount := 0, 0, 0
	for _, st := range filtered {
		excluded := policy.Hidden(st.image)
		mod, hasMod := store.ModerationFor(modRows, st.image.ID, window)
		hidden := excluded || (hasMod && mod.Hidden)
		deranked := hasMod && mod.Deranked
		if excluded {
			excludedCount++
		}
		if hidden {
			hiddenCount++
		}
		if deranked {
			derankedCount++
		}
		if hidden && !includeHidden {
			continue
		}

		entry := rankEntryFor(st)
		override := false
		for i := range modRows {
			if modRows[i].ImageID == st.image.ID && modRows[i].TimeWindow == window {
				override = true
				break
			}
		}
		out := adminRankEntry{
			ScoreRank:        scoreRanks[st.image.ID],
			ID:               entry.ID,
			URL:              entry.URL,
			Author:           entry.Author,
			Title:            entry.Title,
			Width:            entry.Width,
			Height:           entry.Height,
			Wins:             entry.Wins,
			Losses:           entry.Losses,
			Ties:             entry.Ties,
			Votes:            entry.Votes,
			Score:            entry.Score,
			WinRate:          entry.WinRate,
			WinPercentage:    entry.WinPercentage,
			Image:            entry.Image,
			Hidden:           hidden,
			Deranked:         deranked,
			Excluded:         excluded,
			Override:         override,
			ModerationWindow: window,
		}
		if hasMod {
			out.Note = mod.Note
		}
		entries = append(entries, out)
	}

	sort.Slice(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.Hidden != b.Hidden {
			return !a.Hidden
		}
		if a.Deranked != b.Deranked {
			return !a.Deranked
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

	resp := adminRankResponse{
		Time:          rng.Label,
		Window:        window,
		Amount:        amount,
		Offset:        offset,
		Total:         total,
		TotalVotes:    totalVotes,
		HiddenCount:   hiddenCount,
		DerankedCount: derankedCount,
		ExcludedCount: excludedCount,
		Winners:       entries,
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

// --- ranking filter lists (allow/deny regexes) ----------------------------

type rankFiltersDTO struct {
	ExcludeAuthors []string `json:"excludeAuthors"`
	ExcludeTitles  []string `json:"excludeTitles"`
	IncludeAuthors []string `json:"includeAuthors"`
	IncludeTitles  []string `json:"includeTitles"`
}

type rankFiltersRequest struct {
	ExcludeAuthors *[]string `json:"excludeAuthors"`
	ExcludeTitles  *[]string `json:"excludeTitles"`
	IncludeAuthors *[]string `json:"includeAuthors"`
	IncludeTitles  *[]string `json:"includeTitles"`
}

func filtersFromConfig(cfg store.AppConfig) rankFiltersDTO {
	out := rankFiltersDTO{
		ExcludeAuthors: cfg.ExcludeAuthors,
		ExcludeTitles:  cfg.ExcludeTitles,
		IncludeAuthors: cfg.IncludeAuthors,
		IncludeTitles:  cfg.IncludeTitles,
	}
	if out.ExcludeAuthors == nil {
		out.ExcludeAuthors = []string{}
	}
	if out.ExcludeTitles == nil {
		out.ExcludeTitles = []string{}
	}
	if out.IncludeAuthors == nil {
		out.IncludeAuthors = []string{}
	}
	if out.IncludeTitles == nil {
		out.IncludeTitles = []string{}
	}
	return out
}

func (s *Server) getRankFilters(c *gin.Context) {
	cfg, err := s.Store.GetAppConfig(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, filtersFromConfig(cfg))
}

func (s *Server) updateRankFilters(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var req rankFiltersRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Could not parse the filter payload.")
		return
	}
	ctx := c.Request.Context()
	cfg, err := s.Store.GetAppConfig(ctx)
	if err != nil {
		serverError(c, err)
		return
	}
	if req.ExcludeAuthors != nil {
		cfg.ExcludeAuthors = *req.ExcludeAuthors
	}
	if req.ExcludeTitles != nil {
		cfg.ExcludeTitles = *req.ExcludeTitles
	}
	if req.IncludeAuthors != nil {
		cfg.IncludeAuthors = *req.IncludeAuthors
	}
	if req.IncludeTitles != nil {
		cfg.IncludeTitles = *req.IncludeTitles
	}

	// Normalize first so the error message can point at the offending list.
	lists := []struct {
		name    string
		entries []string
	}{
		{"excludeAuthors", cfg.ExcludeAuthors},
		{"excludeTitles", cfg.ExcludeTitles},
		{"includeAuthors", cfg.IncludeAuthors},
		{"includeTitles", cfg.IncludeTitles},
	}
	for _, list := range lists {
		if _, err := store.NormalizePatternList(list.entries); err != nil {
			badRequest(c, fmt.Sprintf("%s: %v", list.name, err))
			return
		}
	}
	if err := s.Store.SetFilterLists(ctx, cfg); err != nil {
		badRequest(c, err.Error())
		return
	}
	updated, err := s.Store.GetAppConfig(ctx)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, filtersFromConfig(updated))
}

// --- per-timeslot hide / derank ------------------------------------------

type moderationRequest struct {
	TimeWindow string  `json:"timeWindow"`
	Hidden     *bool   `json:"hidden"`
	Deranked   *bool   `json:"deranked"`
	Note       *string `json:"note"`
}

func (s *Server) updateImageModeration(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		badRequest(c, "Invalid image id.")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var req moderationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Could not parse the moderation payload.")
		return
	}
	window, err := store.NormalizeModerationWindow(req.TimeWindow)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	ctx := c.Request.Context()
	if _, err := s.Store.GetImage(ctx, id); err != nil {
		asStoreError(c, err)
		return
	}

	rows, err := s.Store.ListImageModeration(ctx)
	if err != nil {
		serverError(c, err)
		return
	}
	mod := store.ImageModeration{ImageID: id, TimeWindow: window}
	for i := range rows {
		if rows[i].ImageID == id && rows[i].TimeWindow == window {
			mod = rows[i]
			break
		}
	}
	if req.Hidden != nil {
		mod.Hidden = *req.Hidden
	}
	if req.Deranked != nil {
		mod.Deranked = *req.Deranked
	}
	if req.Note != nil {
		mod.Note = trimTo(*req.Note, 400)
	}
	mod.UpdatedAt = time.Now().UTC()
	if user := currentUser(c); user != nil {
		uid := user.ID
		mod.UpdatedBy = &uid
	}
	if err := s.Store.UpsertImageModeration(ctx, &mod); err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, mod)
}

func (s *Server) deleteImageModeration(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		badRequest(c, "Invalid image id.")
		return
	}
	window, err := store.NormalizeModerationWindow(c.Query("timeWindow"))
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	if err := s.Store.DeleteImageModeration(c.Request.Context(), id, window); err != nil {
		asStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id, "timeWindow": window})
}
