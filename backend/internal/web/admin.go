package web

import (
	"net/http"
	"strconv"
	"strings"

	"artdream/internal/store"

	"github.com/gin-gonic/gin"
)

// listVotesAdmin godoc
// @Summary List votes (admin)
// @Description Lists votes with the shared admin filters, including images that were deleted after the vote.
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param time query string false "Time window" Enums(day, week, month, year, all, custom)
// @Param seconds query int false "Custom window size in seconds (when time=custom)"
// @Param from query string false "Custom range start (RFC3339 or unix seconds)"
// @Param to query string false "Custom range end (RFC3339 or unix seconds)"
// @Param amount query int false "Page size (default 50, max 500)"
// @Param offset query int false "Pagination offset"
// @Param user query string false "Voting username filter"
// @Param username query string false "Alias for user"
// @Param mode query string false "Vote mode name filter"
// @Param imageId query string false "Image ID that must appear in the vote"
// @Param author query string false "Author filter"
// @Param authorName query string false "Alias for author"
// @Param titleRegex query string false "Title regex filter"
// @Success 200 {object} AdminVoteListResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 500 {object} apiError
// @Router /admin/votes [get]
func (s *Server) listVotesAdmin(c *gin.Context) {
	vf, _, rng, err := filterFromQuery(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	ctx := c.Request.Context()
	votes, total, err := s.Store.ListVotes(ctx, vf)
	if err != nil {
		asStoreError(c, err)
		return
	}
	images, err := s.Store.GetImagesIncludingDeleted(ctx, collectVoteImageIDs(votes))
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items": votesToDTO(votes, images),
		"total": total,
		"time":  rng.Label,
	})
}

// updateVoteAdmin godoc
// @Summary Update a vote (admin)
// @Description Reassigns images or changes the mode/outcome of a vote. The body accepts the same fields and aliases as POST /rank/vote.
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Vote ID"
// @Param payload body VotePayload true "Vote fields to update"
// @Success 200 {object} VoteDTO
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 404 {object} apiError
// @Failure 500 {object} apiError
// @Router /admin/votes/{id} [patch]
func (s *Server) updateVoteAdmin(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	var req voteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Could not parse the update payload.")
		return
	}
	ctx := c.Request.Context()
	v, err := s.Store.GetVote(ctx, id)
	if err != nil {
		asStoreError(c, err)
		return
	}

	leftID, rightID := v.LeftImageID, v.RightImageID
	if strings.TrimSpace(req.LeftImageID) != "" {
		leftID = strings.TrimSpace(req.LeftImageID)
	}
	if strings.TrimSpace(req.RightImageID) != "" {
		rightID = strings.TrimSpace(req.RightImageID)
	}
	if leftID == rightID {
		badRequest(c, "An image cannot compete against itself.")
		return
	}
	images, err := s.Store.GetImagesIncludingDeleted(ctx, []string{leftID, rightID})
	if err != nil {
		serverError(c, err)
		return
	}
	if images[leftID] == nil || images[rightID] == nil {
		badRequest(c, "Both image ids must reference existing images.")
		return
	}
	v.LeftImageID = leftID
	v.RightImageID = rightID

	var mode *store.VoteMode
	if req.ModeID > 0 || strings.TrimSpace(req.Mode) != "" {
		mode, err = resolveVoteMode(c, req.ModeID, req.Mode)
	} else {
		mode, err = s.Store.GetVoteModeByID(ctx, v.ModeID)
	}
	if err != nil {
		if err == store.ErrNotFound {
			badRequest(c, "Unknown vote mode.")
			return
		}
		serverError(c, err)
		return
	}

	v.ModeID = mode.ID
	v.ModeName = mode.Name
	if req.MinValue != nil {
		v.MinValue = *req.MinValue
	} else if v.MinValue == 0 && v.MaxValue == 0 {
		v.MinValue, v.MaxValue = mode.MinValue, mode.MaxValue
	}
	if req.MaxValue != nil {
		v.MaxValue = *req.MaxValue
	}
	if v.MaxValue <= v.MinValue {
		badRequest(c, "maxValue must be greater than minValue.")
		return
	}
	if req.MetadataProvided {
		if len(encodeMetadata(req.Metadata)) > 8<<10 {
			badRequest(c, "Metadata is too large (max 8 KiB).")
			return
		}
		v.Metadata = encodeMetadata(req.Metadata)
	}
	if req.DurationMs != nil {
		v.DurationMs = req.DurationMs
	}

	winner := req.Winner
	value := req.Value
	if mode.IsBinary {
		if !req.WinnerProvided && value != nil {
			mid := float64(v.MinValue+v.MaxValue) / 2
			switch {
			case *value < mid-0.5:
				winner = "left"
			case *value > mid+0.5:
				winner = "right"
			default:
				winner = "tie"
			}
		}
		if !req.WinnerProvided && winner == "" && v.WinnerImageID != nil {
			if *v.WinnerImageID == v.LeftImageID {
				winner = "left"
			} else {
				winner = "right"
			}
		}
	} else {
		if value == nil && v.Value != nil {
			n := float64(*v.Value)
			value = &n
		}
		if value == nil && req.WinnerProvided {
			mid := float64(v.MinValue+v.MaxValue) / 2
			switch strings.ToLower(strings.TrimSpace(winner)) {
			case "left", v.LeftImageID:
				n := float64(v.MinValue)
				value = &n
			case "right", v.RightImageID:
				n := float64(v.MaxValue)
				value = &n
			default:
				value = &mid
			}
		}
	}

	if err := applyVoteOutcome(v, mode.IsBinary, winner, value); err != nil {
		badRequest(c, err.Error())
		return
	}
	if err := s.Store.UpdateVote(ctx, v); err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, voteToDTO(v, images))
}

// deleteVoteAdmin godoc
// @Summary Delete a vote (admin)
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param id path int true "Vote ID"
// @Success 200 {object} DeleteVoteResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 404 {object} apiError
// @Failure 500 {object} apiError
// @Router /admin/votes/{id} [delete]
func (s *Server) deleteVoteAdmin(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	if err := s.Store.DeleteVote(c.Request.Context(), id); err != nil {
		asStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

// bulkDeleteVotesAdmin godoc
// @Summary Bulk delete votes (admin)
// @Description Deletes the votes named by ids, or every vote matching the shared admin filters when ids is omitted.
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param ids query []string false "Explicit vote IDs; takes precedence over the filters" collectionFormat(csv)
// @Param time query string false "Time window" Enums(day, week, month, year, all, custom)
// @Param seconds query int false "Custom window size in seconds (when time=custom)"
// @Param from query string false "Custom range start (RFC3339 or unix seconds)"
// @Param to query string false "Custom range end (RFC3339 or unix seconds)"
// @Param user query string false "Voting username filter"
// @Param mode query string false "Vote mode name filter"
// @Param imageId query string false "Image ID that must appear in the vote"
// @Param author query string false "Author filter"
// @Param titleRegex query string false "Title regex filter"
// @Success 200 {object} DeleteCountResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 500 {object} apiError
// @Router /admin/votes [delete]
func (s *Server) bulkDeleteVotesAdmin(c *gin.Context) {
	vf, _, _, err := filterFromQuery(c)
	if err != nil {
		badRequest(c, err.Error())
		return
	}
	ctx := c.Request.Context()

	ids := make([]int64, 0)
	for _, raw := range strings.Split(c.Query("ids"), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if id, err := parseInt64(raw); err == nil && id > 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) > 0 {
		n, err := s.Store.DeleteVotesByIDs(ctx, ids)
		if err != nil {
			serverError(c, err)
			return
		}
		c.JSON(http.StatusOK, gin.H{"deleted": n})
		return
	}

	vf.Limit = -1
	vf.Offset = 0
	votes, _, err := s.Store.ListVotes(ctx, vf)
	if err != nil {
		asStoreError(c, err)
		return
	}
	toDelete := make([]int64, 0, len(votes))
	for _, v := range votes {
		toDelete = append(toDelete, v.ID)
	}
	n, err := s.Store.DeleteVotesByIDs(ctx, toDelete)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": n})
}

// --- vote mode administration ---------------------------------------------

type voteModeRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	MinValue    *int    `json:"minValue"`
	MaxValue    *int    `json:"maxValue"`
	IsBinary    *bool   `json:"isBinary"`
}

// createVoteMode godoc
// @Summary Create a vote mode (admin)
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body voteModeRequest true "Mode definition"
// @Success 201 {object} store.VoteMode
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 409 {object} apiError
// @Failure 500 {object} apiError
// @Router /admin/rank/modes [post]
func (s *Server) createVoteMode(c *gin.Context) {
	var req voteModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Provide a mode name.")
		return
	}
	name := ""
	if req.Name != nil {
		name = strings.ToLower(trimTo(*req.Name, 64))
	}
	if name == "" {
		badRequest(c, "name is required.")
		return
	}
	minV, maxV, binary := 1, 5, false
	if req.MinValue != nil {
		minV = *req.MinValue
	}
	if req.MaxValue != nil {
		maxV = *req.MaxValue
	}
	if req.IsBinary != nil {
		binary = *req.IsBinary
	}
	if maxV <= minV {
		badRequest(c, "maxValue must be greater than minValue.")
		return
	}
	desc := ""
	if req.Description != nil {
		desc = trimTo(*req.Description, 400)
	}
	mode := &store.VoteMode{Name: name, Description: desc, MinValue: minV, MaxValue: maxV, IsBinary: binary}
	if err := s.Store.CreateVoteMode(c.Request.Context(), mode); err != nil {
		asStoreError(c, err)
		return
	}
	c.JSON(http.StatusCreated, mode)
}

// updateVoteMode godoc
// @Summary Update a vote mode (admin)
// @Tags admin
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "Vote mode ID"
// @Param payload body voteModeRequest true "Fields to update"
// @Success 200 {object} store.VoteMode
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 404 {object} apiError
// @Failure 409 {object} apiError
// @Failure 500 {object} apiError
// @Router /admin/rank/modes/{id} [patch]
func (s *Server) updateVoteMode(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req voteModeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Nothing to update.")
		return
	}
	ctx := c.Request.Context()
	mode, err := s.Store.GetVoteModeByID(ctx, id)
	if err != nil {
		asStoreError(c, err)
		return
	}
	if req.Name != nil {
		name := strings.ToLower(trimTo(*req.Name, 64))
		if name == "" {
			badRequest(c, "name cannot be empty.")
			return
		}
		mode.Name = name
	}
	if req.Description != nil {
		mode.Description = trimTo(*req.Description, 400)
	}
	if req.MinValue != nil {
		mode.MinValue = *req.MinValue
	}
	if req.MaxValue != nil {
		mode.MaxValue = *req.MaxValue
	}
	if req.IsBinary != nil {
		mode.IsBinary = *req.IsBinary
	}
	if mode.MaxValue <= mode.MinValue {
		badRequest(c, "maxValue must be greater than minValue.")
		return
	}
	if err := s.Store.UpdateVoteMode(ctx, mode); err != nil {
		asStoreError(c, err)
		return
	}
	c.JSON(http.StatusOK, mode)
}

// deleteVoteMode godoc
// @Summary Delete a vote mode (admin)
// @Description Refuses to delete a mode that still has votes.
// @Tags admin
// @Security BearerAuth
// @Produce json
// @Param id path int true "Vote mode ID"
// @Success 200 {object} DeleteVoteResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 404 {object} apiError
// @Failure 409 {object} apiError
// @Failure 500 {object} apiError
// @Router /admin/rank/modes/{id} [delete]
func (s *Server) deleteVoteMode(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	n, err := s.Store.CountVotesForMode(ctx, id)
	if err != nil {
		serverError(c, err)
		return
	}
	if n > 0 {
		conflict(c, "This mode still has votes. Delete or migrate them first.")
		return
	}
	if err := s.Store.DeleteVoteMode(ctx, id); err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}
