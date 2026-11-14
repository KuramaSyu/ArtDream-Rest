package web

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"artdream/internal/store"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

const (
	ctxUserKey   = "artdream.user"
	ctxTokenKey  = "artdream.token"
	ctxServerKey = "artdream.server"
)

func hashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

func checkPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

func newSessionToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// cors is a small origin-checking middleware so the Vite dev server can talk
// to the API without pulling in another dependency.
func (s *Server) cors() gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(s.AllowedOrigins))
	allowAll := false
	for _, o := range s.AllowedOrigins {
		o = strings.TrimSpace(o)
		if o == "*" {
			allowAll = true
			continue
		}
		if o != "" {
			allowed[strings.ToLower(o)] = struct{}{}
		}
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" {
			_, ok := allowed[strings.ToLower(origin)]
			if allowAll || ok {
				c.Header("Access-Control-Allow-Origin", origin)
				c.Header("Vary", "Origin")
				c.Header("Access-Control-Allow-Credentials", "true")
				c.Header("Access-Control-Allow-Methods", "GET, POST, PATCH, PUT, DELETE, OPTIONS")
				c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Session-Token")
				c.Header("Access-Control-Expose-Headers", "Content-Length, Content-Type, ETag")
				c.Header("Access-Control-Max-Age", "86400")
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func (s *Server) attachServer() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Set(ctxServerKey, s)
		c.Next()
	}
}

func (s *Server) authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			unauthorized(c, "Please sign in first.")
			return
		}
		user, err := s.Store.GetSessionUser(c.Request.Context(), token)
		if err != nil {
			if err == store.ErrNotFound {
				unauthorized(c, "Your session expired. Please sign in again.")
				return
			}
			serverError(c, err)
			return
		}
		c.Set(ctxUserKey, user)
		c.Set(ctxTokenKey, token)
		c.Next()
	}
}

// optionalAuth attaches the session user when a valid token is present, but
// lets anonymous requests through. Anonymous uploads are later connected to
// an account through a per-upload claim token.
func (s *Server) optionalAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := bearerToken(c)
		if token == "" {
			c.Next()
			return
		}
		user, err := s.Store.GetSessionUser(c.Request.Context(), token)
		if err != nil {
			if err != store.ErrNotFound {
				serverError(c, err)
				return
			}
			c.Next()
			return
		}
		c.Set(ctxUserKey, user)
		c.Set(ctxTokenKey, token)
		c.Next()
	}
}

func (s *Server) adminRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if u == nil || !u.IsAdmin {
			forbidden(c, "Administrator access required.")
			return
		}
		c.Next()
	}
}

func bearerToken(c *gin.Context) string {
	if h := c.GetHeader("Authorization"); h != "" {
		parts := strings.SplitN(h, " ", 2)
		if len(parts) == 2 && strings.EqualFold(parts[0], "bearer") {
			return strings.TrimSpace(parts[1])
		}
	}
	if h := c.GetHeader("X-Session-Token"); h != "" {
		return strings.TrimSpace(h)
	}
	return ""
}
