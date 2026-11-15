package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"artdream/internal/imageproc"
	"artdream/internal/store"
	"artdream/internal/web"

	"github.com/gin-gonic/gin"
)

// @title ArtDream API
// @version 1.0
// @description REST API for uploading, browsing, voting on and ranking ArtDream artwork.
// @description Authentication uses the session token returned by POST /auth/login. Send it as `Authorization: Bearer <token>`.
// @BasePath /api/v1
// @schemes http https
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Session token prefixed with "Bearer ".
func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("artdream ")

	dataDir := env("DATA_DIR", "./data")
	dbPath := env("DB_PATH", filepath.Join(dataDir, "artdream.db"))
	uploadDir := env("UPLOAD_DIR", filepath.Join(dataDir, "images"))
	cacheDir := env("CACHE_DIR", filepath.Join(dataDir, "cache"))
	addr := env("ADDR", ":8080")

	frontendDir := os.Getenv("FRONTEND_DIR")
	if frontendDir == "" {
		for _, candidate := range []string{"../frontend/dist", "frontend/dist"} {
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				frontendDir = candidate
				break
			}
		}
	}

	if env("GIN_MODE", "") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}

	st, err := store.Open(dbPath)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer st.Close()

	renderer, err := imageproc.New(cacheDir)
	if err != nil {
		log.Fatalf("image cache: %v", err)
	}

	origins := strings.Split(env("ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173"), ",")
	srv := web.NewServer(st, renderer, dataDir, uploadDir, frontendDir, origins)
	router := srv.Router()

	httpServer := &http.Server{
		Addr:              addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("listening on %s (data: %s)", addr, dataDir)
		if frontendDir != "" {
			log.Printf("serving SPA from %s", frontendDir)
		}
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("http server: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
	log.Println("bye")
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
