package web

import (
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Router wires all REST routes and, when configured, the built SPA.
func (s *Server) Router() *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery(), s.cors())
	r.MaxMultipartMemory = 8 << 20

	api := r.Group("/api/v1")
	api.Use(s.attachServer())

	api.GET("/health", s.health)
	api.GET("/config", s.needsBootstrap)
	api.GET("/frontend-config", s.frontendConfig)
	api.GET("/users/needs-bootstrap", s.needsBootstrap)
	api.POST("/users/bootstrap", s.bootstrap)
	api.POST("/auth/login", s.login)
	api.POST("/auth/register", s.register)
	api.POST("/images", s.optionalAuth(), s.uploadImage)

	// Optional auth lets the request filters use mine=true / uploadedBy=me
	// without making the public listings login-only.
	api.GET("/images/author", s.optionalAuth(), s.imagesByAuthor)
	api.GET("/images/random", s.optionalAuth(), s.randomImages)
	api.GET("/images/rank", s.optionalAuth(), s.rankImages)
	api.GET("/rank/modes", s.listVoteModes)
	api.GET("/images/:id", s.serveImage)
	api.GET("/images/:id/meta", s.imageMeta)

	authed := api.Group("", s.authRequired())
	{
		authed.GET("/auth/me", s.me)
		authed.PATCH("/auth/me", s.updateMe)
		authed.POST("/auth/logout", s.logout)
		authed.POST("/images/claim", s.claimImages)
		authed.POST("/rank/vote", s.castVote)
		authed.GET("/rank/me", s.myVotes)
	}

	admin := authed.Group("", s.adminRequired())
	{
		admin.GET("/users", s.listUsers)
		admin.POST("/users", s.createUser)
		admin.PATCH("/users/:id", s.updateUser)
		admin.DELETE("/users/:id", s.deleteUser)

		admin.GET("/admin/images", s.listImagesAdmin)
		admin.PATCH("/admin/images/:id", s.updateImageAdmin)
		admin.DELETE("/admin/images/:id", s.deleteImageAdmin)
		admin.DELETE("/admin/images", s.bulkDeleteImagesAdmin)

		admin.GET("/admin/votes", s.listVotesAdmin)
		admin.PATCH("/admin/votes/:id", s.updateVoteAdmin)
		admin.DELETE("/admin/votes/:id", s.deleteVoteAdmin)
		admin.DELETE("/admin/votes", s.bulkDeleteVotesAdmin)

		admin.GET("/admin/rank", s.adminRankImages)
		admin.GET("/admin/rank/filters", s.getRankFilters)
		admin.PATCH("/admin/rank/filters", s.updateRankFilters)
		admin.PATCH("/admin/images/:id/moderation", s.updateImageModeration)
		admin.DELETE("/admin/images/:id/moderation", s.deleteImageModeration)

		admin.PATCH("/admin/frontend-config", s.updateFrontendConfig)
		admin.POST("/admin/rank/modes", s.createVoteMode)
		admin.PATCH("/admin/rank/modes/:id", s.updateVoteMode)
		admin.DELETE("/admin/rank/modes/:id", s.deleteVoteMode)
	}

	r.NoRoute(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			notFound(c, "Unknown API route.")
			return
		}
		s.serveSPA(c)
	})

	return r
}

// health godoc
// @Summary Health check
// @Description Returns the API status and the current server time.
// @Tags system
// @Produce json
// @Success 200 {object} HealthResponse
// @Router /health [get]
func (s *Server) health(c *gin.Context) {
	c.JSON(http.StatusOK, HealthResponse{Status: "ok", Time: time.Now().UTC()})
}

func (s *Server) serveSPA(c *gin.Context) {
	if s.FrontendDir == "" {
		notFound(c, "Unknown route. Build the frontend or set FRONTEND_DIR to serve the SPA.")
		return
	}
	clean := path.Clean("/" + c.Request.URL.Path)
	file := filepath.Join(s.FrontendDir, filepath.FromSlash(clean))
	if info, err := os.Stat(file); err == nil && !info.IsDir() {
		c.File(file)
		return
	}
	// A missing asset should 404 rather than return index.html.
	if strings.Contains(path.Base(clean), ".") && clean != "/" {
		notFound(c, "Asset not found.")
		return
	}
	index := filepath.Join(s.FrontendDir, "index.html")
	if _, err := os.Stat(index); err != nil {
		notFound(c, "Frontend build not found. Run `npm run build` in frontend/.")
		return
	}
	c.File(index)
}
