package web

import (
	"time"

	"artdream/internal/store"
)

// The types in this file exist so swag can describe the JSON envelopes the
// handlers return. They are not used at runtime; keep them in sync with the
// gin.H payloads in the corresponding handlers.

// HealthResponse is the payload of GET /health.
type HealthResponse struct {
	Status string    `json:"status" example:"ok"`
	Time   time.Time `json:"time"`
}

// NeedsBootstrapResponse reports whether the instance still needs its first
// administrator account.
type NeedsBootstrapResponse struct {
	NeedsBootstrap bool `json:"needsBootstrap"`
	UserCount      int  `json:"userCount"`
}

// FrontendConfigResponse is the UI configuration returned by the config
// endpoints.
type FrontendConfigResponse struct {
	RankingAnonymized      bool      `json:"rankingAnonymized"`
	DefaultImageVisibility string    `json:"defaultImageVisibility" example:"all"`
	RankRotationEnabled    bool      `json:"rankRotationEnabled"`
	RankRotationPercent    int       `json:"rankRotationPercent" example:"60"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

// UserResponse wraps a single account.
type UserResponse struct {
	User *store.User `json:"user"`
}

// UsersResponse is the paginated account listing.
type UsersResponse struct {
	Items []store.User `json:"items"`
	Total int          `json:"total"`
}

// ImageListResponse is the paginated image listing.
type ImageListResponse struct {
	Items []imageDTO `json:"items"`
	Total int        `json:"total"`
}

// AuthorImageListResponse is the author-scoped image listing.
type AuthorImageListResponse struct {
	Items  []imageDTO `json:"items"`
	Total  int        `json:"total"`
	Author string     `json:"author"`
}

// ClaimResponse reports which anonymous uploads were attached to the account.
type ClaimResponse struct {
	Claimed []string `json:"claimed"`
	Count   int      `json:"count"`
}

// OKResponse is a generic success acknowledgement.
type OKResponse struct {
	OK bool `json:"ok" example:"true"`
}

// DeleteImageResponse reports a deleted image ID.
type DeleteImageResponse struct {
	Deleted string `json:"deleted"`
}

// DeleteUserResponse reports a deleted user ID.
type DeleteUserResponse struct {
	Deleted int64 `json:"deleted"`
}

// DeleteVoteResponse reports a deleted vote ID.
type DeleteVoteResponse struct {
	Deleted int64 `json:"deleted"`
}

// DeleteCountResponse reports how many rows a bulk delete removed.
type DeleteCountResponse struct {
	Deleted int64 `json:"deleted"`
}

// DeletedCountResponse reports how many images a bulk delete removed.
type DeletedCountResponse struct {
	Deleted int `json:"deleted"`
}

// VoteListResponse is the paginated vote listing for the current user.
type VoteListResponse struct {
	Items []VoteDTO `json:"items"`
	Total int       `json:"total"`
}

// AdminVoteListResponse is the paginated vote listing for administrators.
type AdminVoteListResponse struct {
	Items []VoteDTO `json:"items"`
	Total int       `json:"total"`
	Time  string    `json:"time"`
}

// ModerationDeleteResponse reports a deleted moderation override.
type ModerationDeleteResponse struct {
	Deleted    string `json:"deleted"`
	TimeWindow string `json:"timeWindow"`
}

// VotePayload documents the canonical vote body. The handler also accepts a
// number of alternative key spellings for convenience; see the description of
// the vote endpoints.
type VotePayload struct {
	Mode         string         `json:"mode,omitempty" example:"duel"`
	ModeID       int64          `json:"modeId,omitempty" example:"1"`
	LeftImageID  string         `json:"leftImageId" example:"4a1f2c9d8e7b6a5c4d3e2f10"`
	RightImageID string         `json:"rightImageId" example:"9b8a7c6d5e4f3a2b1c0d9e8f"`
	Winner       string         `json:"winner,omitempty" example:"left"`
	Value        *float64       `json:"value,omitempty" example:"2"`
	MinValue     *int           `json:"minValue,omitempty" example:"1"`
	MaxValue     *int           `json:"maxValue,omitempty" example:"5"`
	Metadata     map[string]any `json:"metadata,omitempty"`
	DurationMs   *int64         `json:"durationMs,omitempty" example:"1234"`
	// AnonymousID is the device-local secret used when no account is signed in.
	AnonymousID string `json:"anonymousId,omitempty" example:"6f1c..."`
}

// VoteDTO mirrors voteDTO for swag. The runtime DTO uses json.RawMessage for
// Metadata, which swag cannot resolve, so the schema uses a plain map here.
type VoteDTO struct {
	ID            int64          `json:"id"`
	ModeID        int64          `json:"modeId"`
	ModeName      string         `json:"modeName"`
	LeftImageID   string         `json:"leftImageId"`
	RightImageID  string         `json:"rightImageId"`
	LeftImage     *imageDTO      `json:"leftImage,omitempty"`
	RightImage    *imageDTO      `json:"rightImage,omitempty"`
	WinnerImageID *string        `json:"winnerImageId,omitempty"`
	LoserImageID  *string        `json:"loserImageId,omitempty"`
	WinnerImage   *imageDTO      `json:"winnerImage,omitempty"`
	LoserImage    *imageDTO      `json:"loserImage,omitempty"`
	Winner        string         `json:"winner" example:"left"`
	Value         *int           `json:"value,omitempty"`
	MinValue      int            `json:"minValue"`
	MaxValue      int            `json:"maxValue"`
	Weight        float64        `json:"weight"`
	UserID        *int64         `json:"userId,omitempty"`
	Username      string         `json:"username"`
	User          *voteUserDTO   `json:"user,omitempty"`
	Metadata      map[string]any `json:"metadata"`
	DurationMs    *int64         `json:"durationMs,omitempty"`
	VoteSpeed     float64        `json:"voteSpeed" example:"7.25"`
	CreatedAt     time.Time      `json:"createdAt"`
}
