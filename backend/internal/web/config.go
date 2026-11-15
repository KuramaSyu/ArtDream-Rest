package web

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type frontendConfigRequest struct {
	RankingAnonymized *bool `json:"rankingAnonymized"`
}

// frontendConfig exposes display decisions that are enforced only in the UI.
// Image/ranking payloads keep delivering full data (including authors).
func (s *Server) frontendConfig(c *gin.Context) {
	config, err := s.Store.GetAppConfig(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"rankingAnonymized": config.RankingAnonymized,
		"updatedAt":         config.UpdatedAt,
	})
}

func (s *Server) updateFrontendConfig(c *gin.Context) {
	var req frontendConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.RankingAnonymized == nil {
		badRequest(c, "Provide rankingAnonymized (boolean).")
		return
	}
	ctx := c.Request.Context()
	if err := s.Store.SetRankingAnonymized(ctx, *req.RankingAnonymized); err != nil {
		serverError(c, err)
		return
	}
	config, err := s.Store.GetAppConfig(ctx)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"rankingAnonymized": config.RankingAnonymized,
		"updatedAt":         config.UpdatedAt,
	})
}
