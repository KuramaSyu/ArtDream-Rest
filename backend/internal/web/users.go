package web

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"artdream/internal/store"

	"github.com/gin-gonic/gin"
)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{2,31}$`)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type createUserRequest struct {
	Username    string `json:"username"`
	Password    string `json:"password"`
	DisplayName string `json:"displayName"`
	IsAdmin     bool   `json:"isAdmin"`
}

type updateUserRequest struct {
	DisplayName *string `json:"displayName"`
	Password    *string `json:"password"`
	IsAdmin     *bool   `json:"isAdmin"`
}

type updateMeRequest struct {
	DisplayName     *string `json:"displayName"`
	Password        *string `json:"password"`
	CurrentPassword string  `json:"currentPassword"`
}

type sessionResponse struct {
	Token string      `json:"token"`
	User  *store.User `json:"user"`
}

// needsBootstrap godoc
// @Summary Check whether the instance needs a first administrator
// @Description Reports whether any account exists. The first account must be created through /users/bootstrap.
// @Tags auth
// @Produce json
// @Success 200 {object} NeedsBootstrapResponse
// @Failure 500 {object} apiError
// @Router /users/needs-bootstrap [get]
// @Router /config [get]
func (s *Server) needsBootstrap(c *gin.Context) {
	n, err := s.Store.CountUsers(c.Request.Context())
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"needsBootstrap": n == 0, "userCount": n})
}

// bootstrap godoc
// @Summary Create the first administrator account
// @Description Only succeeds while the instance has no accounts. Signs in the new administrator and returns a session token.
// @Tags auth
// @Accept json
// @Produce json
// @Param payload body createUserRequest true "Account details (isAdmin is forced to true)"
// @Success 201 {object} sessionResponse
// @Failure 400 {object} apiError
// @Failure 409 {object} apiError
// @Failure 500 {object} apiError
// @Router /users/bootstrap [post]
func (s *Server) bootstrap(c *gin.Context) {
	s.bootstrapMu.Lock()
	defer s.bootstrapMu.Unlock()

	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Please provide a username and password.")
		return
	}
	req.Username = trimTo(req.Username, 32)
	req.DisplayName = trimTo(req.DisplayName, 80)
	if err := validateCredentials(req.Username, req.Password); err != nil {
		badRequest(c, err.Error())
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}

	ctx := c.Request.Context()
	n, err := s.Store.CountUsers(ctx)
	if err != nil {
		serverError(c, err)
		return
	}
	if n > 0 {
		conflict(c, "This ArtDream already has an account. Please sign in.")
		return
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		serverError(c, err)
		return
	}
	user := &store.User{
		Username:     req.Username,
		DisplayName:  req.DisplayName,
		PasswordHash: hash,
		IsAdmin:      true,
	}
	if err := s.Store.CreateUser(ctx, user); err != nil {
		asStoreError(c, err)
		return
	}
	token, err := s.startSession(c, user)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusCreated, sessionResponse{Token: token, User: user})
}

// register creates a regular (non-admin) account. The first account must be
// created through bootstrap so the instance always has an administrator.
// register godoc
// @Summary Register a regular account
// @Description Creates a non-admin account. Requires that an administrator already exists.
// @Tags auth
// @Accept json
// @Produce json
// @Param payload body createUserRequest true "Account details"
// @Success 201 {object} sessionResponse
// @Failure 400 {object} apiError
// @Failure 409 {object} apiError
// @Failure 500 {object} apiError
// @Router /auth/register [post]
func (s *Server) register(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Please provide a username and password.")
		return
	}
	req.Username = trimTo(req.Username, 32)
	req.DisplayName = trimTo(req.DisplayName, 80)
	if err := validateCredentials(req.Username, req.Password); err != nil {
		badRequest(c, err.Error())
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}

	ctx := c.Request.Context()
	n, err := s.Store.CountUsers(ctx)
	if err != nil {
		serverError(c, err)
		return
	}
	if n == 0 {
		conflict(c, "This ArtDream has no administrator yet. Use the launch page to create the first account.")
		return
	}

	hash, err := hashPassword(req.Password)
	if err != nil {
		serverError(c, err)
		return
	}
	user := &store.User{
		Username:     req.Username,
		DisplayName:  req.DisplayName,
		PasswordHash: hash,
		IsAdmin:      false,
	}
	if err := s.Store.CreateUser(ctx, user); err != nil {
		asStoreError(c, err)
		return
	}
	token, err := s.startSession(c, user)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusCreated, sessionResponse{Token: token, User: user})
}

// login godoc
// @Summary Sign in
// @Description Exchanges a username and password for a session token.
// @Tags auth
// @Accept json
// @Produce json
// @Param payload body loginRequest true "Credentials"
// @Success 200 {object} sessionResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 500 {object} apiError
// @Router /auth/login [post]
func (s *Server) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Please provide a username and password.")
		return
	}
	ctx := c.Request.Context()
	user, err := s.Store.GetUserByUsername(ctx, strings.TrimSpace(req.Username))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			unauthorized(c, "Wrong username or password.")
			return
		}
		serverError(c, err)
		return
	}
	if !checkPassword(user.PasswordHash, req.Password) {
		unauthorized(c, "Wrong username or password.")
		return
	}
	_ = s.Store.DeleteExpiredSessions(ctx)
	token, err := s.startSession(c, user)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, sessionResponse{Token: token, User: user})
}

func (s *Server) startSession(c *gin.Context, user *store.User) (string, error) {
	token, err := newSessionToken()
	if err != nil {
		return "", err
	}
	if err := s.Store.CreateSession(c.Request.Context(), token, user.ID, s.SessionTTL); err != nil {
		return "", err
	}
	return token, nil
}

// logout godoc
// @Summary Sign out
// @Description Deletes the current session token.
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} OKResponse
// @Router /auth/logout [post]
func (s *Server) logout(c *gin.Context) {
	token, _ := c.Get(ctxTokenKey)
	if t, ok := token.(string); ok {
		_ = s.Store.DeleteSession(c.Request.Context(), t)
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// me godoc
// @Summary Get the current account
// @Tags auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} UserResponse
// @Failure 401 {object} apiError
// @Router /auth/me [get]
func (s *Server) me(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"user": currentUser(c)})
}

// updateMe godoc
// @Summary Update the current account
// @Description Changes the display name and/or password. A password change requires the current password.
// @Tags auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body updateMeRequest true "Fields to update"
// @Success 200 {object} UserResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Router /auth/me [patch]
func (s *Server) updateMe(c *gin.Context) {
	var req updateMeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Nothing to update.")
		return
	}
	user := currentUser(c)
	ctx := c.Request.Context()

	var displayName, passwordHash *string
	if req.DisplayName != nil {
		name := trimTo(*req.DisplayName, 80)
		if name == "" {
			badRequest(c, "Display name cannot be empty.")
			return
		}
		displayName = &name
	}
	if req.Password != nil {
		if len(*req.Password) < 8 {
			badRequest(c, "Password must be at least 8 characters.")
			return
		}
		if !checkPassword(user.PasswordHash, req.CurrentPassword) {
			badRequest(c, "Current password is incorrect.")
			return
		}
		hash, err := hashPassword(*req.Password)
		if err != nil {
			serverError(c, err)
			return
		}
		passwordHash = &hash
	}
	if displayName == nil && passwordHash == nil {
		badRequest(c, "Nothing to update.")
		return
	}
	if err := s.Store.UpdateUser(ctx, user.ID, displayName, passwordHash, nil); err != nil {
		serverError(c, err)
		return
	}
	updated, err := s.Store.GetUserByID(ctx, user.ID)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": updated})
}

// listUsers godoc
// @Summary List accounts
// @Tags users
// @Security BearerAuth
// @Produce json
// @Param amount query int false "Page size (default 200, max 500)"
// @Param offset query int false "Pagination offset"
// @Success 200 {object} UsersResponse
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Router /users [get]
func (s *Server) listUsers(c *gin.Context) {
	users, total, err := s.Store.ListUsers(c.Request.Context(), store.UserFilter{
		Limit:  parseAmount(c, 200, 500),
		Offset: parseOffset(c),
	})
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": users, "total": total})
}

// createUser godoc
// @Summary Create an account
// @Tags users
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param payload body createUserRequest true "New account"
// @Success 201 {object} UserResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 409 {object} apiError
// @Router /users [post]
func (s *Server) createUser(c *gin.Context) {
	var req createUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Please provide a username and password.")
		return
	}
	req.Username = trimTo(req.Username, 32)
	req.DisplayName = trimTo(req.DisplayName, 80)
	if err := validateCredentials(req.Username, req.Password); err != nil {
		badRequest(c, err.Error())
		return
	}
	if req.DisplayName == "" {
		req.DisplayName = req.Username
	}
	hash, err := hashPassword(req.Password)
	if err != nil {
		serverError(c, err)
		return
	}
	creator := currentUser(c)
	user := &store.User{
		Username:     req.Username,
		DisplayName:  req.DisplayName,
		PasswordHash: hash,
		IsAdmin:      req.IsAdmin,
		CreatedBy:    &creator.ID,
	}
	if err := s.Store.CreateUser(c.Request.Context(), user); err != nil {
		asStoreError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"user": user})
}

// updateUser godoc
// @Summary Update an account
// @Description Changes the display name, password and/or administrator flag. At least one administrator must remain.
// @Tags users
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param id path int true "User ID"
// @Param payload body updateUserRequest true "Fields to update"
// @Success 200 {object} UserResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 404 {object} apiError
// @Failure 409 {object} apiError
// @Router /users/{id} [patch]
func (s *Server) updateUser(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	var req updateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		badRequest(c, "Nothing to update.")
		return
	}
	ctx := c.Request.Context()
	target, err := s.Store.GetUserByID(ctx, id)
	if err != nil {
		asStoreError(c, err)
		return
	}

	var displayName, passwordHash *string
	if req.DisplayName != nil {
		name := trimTo(*req.DisplayName, 80)
		if name == "" {
			badRequest(c, "Display name cannot be empty.")
			return
		}
		displayName = &name
	}
	if req.Password != nil {
		if len(*req.Password) < 8 {
			badRequest(c, "Password must be at least 8 characters.")
			return
		}
		hash, err := hashPassword(*req.Password)
		if err != nil {
			serverError(c, err)
			return
		}
		passwordHash = &hash
	}
	if req.IsAdmin != nil && !*req.IsAdmin && target.IsAdmin {
		admins, err := s.countAdmins(ctx)
		if err != nil {
			serverError(c, err)
			return
		}
		if admins <= 1 {
			conflict(c, "At least one administrator must remain.")
			return
		}
	}
	if err := s.Store.UpdateUser(ctx, id, displayName, passwordHash, req.IsAdmin); err != nil {
		serverError(c, err)
		return
	}
	updated, err := s.Store.GetUserByID(ctx, id)
	if err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": updated})
}

// deleteUser godoc
// @Summary Delete an account
// @Description Deletes an account. You cannot delete yourself, and at least one administrator must remain.
// @Tags users
// @Security BearerAuth
// @Produce json
// @Param id path int true "User ID"
// @Success 200 {object} DeleteUserResponse
// @Failure 400 {object} apiError
// @Failure 401 {object} apiError
// @Failure 403 {object} apiError
// @Failure 404 {object} apiError
// @Failure 409 {object} apiError
// @Router /users/{id} [delete]
func (s *Server) deleteUser(c *gin.Context) {
	id, ok := parseIDParam(c, "id")
	if !ok {
		return
	}
	ctx := c.Request.Context()
	target, err := s.Store.GetUserByID(ctx, id)
	if err != nil {
		asStoreError(c, err)
		return
	}
	me := currentUser(c)
	if me.ID == target.ID {
		badRequest(c, "You cannot delete your own account.")
		return
	}
	if target.IsAdmin {
		admins, err := s.countAdmins(ctx)
		if err != nil {
			serverError(c, err)
			return
		}
		if admins <= 1 {
			conflict(c, "At least one administrator must remain.")
			return
		}
	}
	if err := s.Store.DeleteUser(ctx, id); err != nil {
		serverError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": id})
}

func (s *Server) countAdmins(ctx context.Context) (int, error) {
	users, _, err := s.Store.ListUsers(ctx, store.UserFilter{Limit: 500})
	if err != nil {
		return 0, err
	}
	n := 0
	for _, u := range users {
		if u.IsAdmin {
			n++
		}
	}
	return n, nil
}

func validateCredentials(username, password string) error {
	if !usernamePattern.MatchString(username) {
		return fmt.Errorf("username must be 3-32 characters (letters, digits, _ . -) and start with a letter or digit")
	}
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if len(password) > 200 {
		return fmt.Errorf("password is too long")
	}
	return nil
}
