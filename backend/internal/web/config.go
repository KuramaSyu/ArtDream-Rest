package web

import (
	"net/http"

	"artdream/internal/store"

	"github.com/gin-gonic/gin"
)

type frontendConfigRequest struct {
	RankingAnonymized      *bool   `json:"rankingAnonymized"`
	DefaultImageVisibility *string `json:"defaultImageVisibility"`
	RankRotationEnabled    *bool   `json:"rankRotationEnabled"`
	RankRotationPercent    *int    `json:"rankRotationPercent"`
}

func frontendConfigResponse(config store.AppConfig) gin.H {
	visibility := config.DefaultImageVisibility
	if visibility == "" {
		visibility = store.VisibilityAll
	}
	percent := config.RankRotationPercent
	if percent < 0 || percent > 100 {
		percent = 60
	}
	return gin.H{
		"rankingAnonymized":      config.RankingAnonymized,
		"defaultImageVisibility": visibility,
		"rankRotationEnabled":    config.RankRotationEnabled,
		"rankRotationPercent":    percent,
		"updatedAt":              config.UpdatedAt,
	}
}

// frontendConfig exposes display decisions that are enforced only in the UI
// plus the server-side public/private default for image requests.
// frontendConfig godoc
// @Summary Get frontend configuration
// @Description Returns display-only settings plus the server-side default image visibility.
// @Tags config
// @Produce json
// @Success 200 {object} FrontendConfigResponse
// @Failure 500 {object} apiError
// @Router /frontend-config [get]
func (s *Server) frontendConfig(c *gin.Context) {
	config, err := s.Store.GetAppConfig(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, frontendConfigResponse(config))
}

// updateFrontendConfig godoc
// @Summary Update frontend configuration
// @Description Updates the ranking anonymization switch and/or the default image visibility.
// @Tags config
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body frontendConfigRequest true "Config fields to update"
// @Success 200 {object} FrontendConfigResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 500 {object} apiError
// @Router /admin/frontend-config [patch]
func (s *Server) updateFrontendConfig(c *gin.Context) {
	var req frontendConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil ||
		(req.RankingAnonymized == nil && req.DefaultImageVisibility == nil &&
			req.RankRotationEnabled == nil && req.RankRotationPercent == nil) {
		badRequest(c, "Provide rankingAnonymized, defaultImageVisibility, rankRotationEnabled and/or rankRotationPercent.")
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
	if req.RankRotationEnabled != nil {
		if err := s.Store.SetRankRotationEnabled(ctx, *req.RankRotationEnabled); err != nil {
			serverError(c, err)
			return
		}
	}
	if req.RankRotationPercent != nil {
		if err := s.Store.SetRankRotationPercent(ctx, *req.RankRotationPercent); err != nil {
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
