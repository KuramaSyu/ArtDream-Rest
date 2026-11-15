package web

import (
	"net/http"

	"artdream/internal/store"

	"github.com/gin-gonic/gin"
)

type frontendConfigRequest struct {
	RankingAnonymized      *bool   `json:"rankingAnonymized"`
	DefaultImageVisibility *string `json:"defaultImageVisibility"`
}

func frontendConfigResponse(config store.AppConfig) gin.H {
	visibility := config.DefaultImageVisibility
	if visibility == "" {
		visibility = store.VisibilityAll
	}
	return gin.H{
		"rankingAnonymized":      config.RankingAnonymized,
		"defaultImageVisibility": visibility,
		"updatedAt":              config.UpdatedAt,
	}
}

// frontendConfig exposes display decisions that are enforced only in the UI
// plus the server-side public/private default for image requests.
func (s *Server) frontendConfig(c *gin.Context) {
	config, err := s.Store.GetAppConfig(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, frontendConfigResponse(config))
}

func (s *Server) updateFrontendConfig(c *gin.Context) {
	var req frontendConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil ||
		(req.RankingAnonymized == nil && req.DefaultImageVisibility == nil) {
		badRequest(c, "Provide rankingAnonymized and/or defaultImageVisibility.")
		return
	}
	ctx := c.Request.Context()
	if req.RankingAnonymized != nil {
		if err := s.Store.SetRankingAnonymized(ctx, *req.RankingAnonymized); err != nil {
			serverError(c, err)
			return
		}
	}
	if req.DefaultImageVisibility != nil {
		if err := s.Store.SetDefaultImageVisibility(ctx, *req.DefaultImageVisibility); err != nil {
			badRequest(c, err.Error())
			return
		}
	}
	config, err := s.Store.GetAppConfig(ctx)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, frontendConfigResponse(config))
}
