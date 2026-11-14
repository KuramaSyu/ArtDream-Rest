package store

import "time"

// User is an account that can upload images, vote and (optionally) administer
// the platform.
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	DisplayName  string    `json:"displayName"`
	PasswordHash string    `json:"-"`
	IsAdmin      bool      `json:"isAdmin"`
	CreatedAt    time.Time `json:"createdAt"`
	CreatedBy    *int64    `json:"createdBy,omitempty"`
}

// Image is an uploaded artwork. StoragePath points at the pristine upload on
// disk; derivative formats are rendered on demand and cached separately.
type Image struct {
	ID               string     `json:"id"`
	Author           string     `json:"author"`
	Title            string     `json:"title"`
	OriginalFilename string     `json:"originalFilename,omitempty"`
	StoragePath      string     `json:"-"`
	MimeType         string     `json:"mimeType"`
	Width            int        `json:"width"`
	Height           int        `json:"height"`
	SizeBytes        int64      `json:"sizeBytes"`
	ClaimToken       string     `json:"-"`
	UploadedBy       *int64     `json:"uploadedBy,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	DeletedAt        *time.Time `json:"deletedAt,omitempty"`
}

// VoteMode describes a voting scale. Binary modes use Winner/Loser; value
// modes interpret numbers between MinValue and MaxValue (low = left image,
// high = right image).
type VoteMode struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	MinValue    int       `json:"minValue"`
	MaxValue    int       `json:"maxValue"`
	IsBinary    bool      `json:"isBinary"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Vote is a single comparison between two images.
type Vote struct {
	ID            int64     `json:"id"`
	ModeID        int64     `json:"modeId"`
	ModeName      string    `json:"modeName"`
	LeftImageID   string    `json:"leftImageId"`
	RightImageID  string    `json:"rightImageId"`
	WinnerImageID *string   `json:"winnerImageId,omitempty"`
	LoserImageID  *string   `json:"loserImageId,omitempty"`
	Value         *int      `json:"value,omitempty"`
	MinValue      int       `json:"minValue"`
	MaxValue      int       `json:"maxValue"`
	Weight        float64   `json:"weight"`
	UserID        *int64    `json:"userId,omitempty"`
	Username      string    `json:"username"`
	Metadata      string    `json:"metadata"`
	DurationMs    *int64    `json:"durationMs,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
}

// UserFilter is used by the admin user listing.
type UserFilter struct {
	Limit  int
	Offset int
}

// ImageFilter is shared by the public author listing, random selection and
// the admin image manager. Author is always a case-insensitive substring
// match when used through the admin endpoints.
type ImageFilter struct {
	Author         string
	TitleRegex     string
	IncludeDeleted bool
	Limit          int
	Offset         int
}

// VoteFilter narrows a vote listing. Author and TitleRegex are matched
// against the two images referenced by a vote; Username is the voting user.
// Times are unix seconds and inclusive.
type VoteFilter struct {
	From       *int64
	To         *int64
	Username   string
	Mode       string
	ImageID    string
	Author     string
	TitleRegex string
	Limit      int
	Offset     int
}
