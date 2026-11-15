package store

import "time"

// The row types below are the GORM persistence layer. They deliberately keep
// every timestamp as unix seconds (INTEGER) so existing databases stay
// compatible, while the public models keep time.Time for the API layer.

type userRow struct {
	ID           int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Username     string `gorm:"column:username;not null;type:text COLLATE NOCASE;uniqueIndex:users_username_key"`
	DisplayName  string `gorm:"column:display_name;not null;default:''"`
	PasswordHash string `gorm:"column:password_hash;not null"`
	IsAdmin      int64  `gorm:"column:is_admin;not null;default:0"`
	CreatedAt    int64  `gorm:"column:created_at;not null;index:users_created_idx"`
	CreatedBy    *int64 `gorm:"column:created_by"`
}

func (userRow) TableName() string { return "users" }

type sessionRow struct {
	Token     string `gorm:"column:token;primaryKey"`
	UserID    int64  `gorm:"column:user_id;not null;index:sessions_user_idx"`
	CreatedAt int64  `gorm:"column:created_at;not null"`
	ExpiresAt int64  `gorm:"column:expires_at;not null;index:sessions_expires_idx"`
}

func (sessionRow) TableName() string { return "sessions" }

type imageRow struct {
	ID                string `gorm:"column:id;primaryKey"`
	Author            string `gorm:"column:author;not null;type:text COLLATE NOCASE;index:images_author_idx"`
	Title             string `gorm:"column:title;not null;default:''"`
	OriginalFilename  string `gorm:"column:original_filename;not null;default:''"`
	StoragePath       string `gorm:"column:storage_path;not null"`
	MimeType          string `gorm:"column:mime_type;not null"`
	Width             int    `gorm:"column:width;not null;default:0"`
	Height            int    `gorm:"column:height;not null;default:0"`
	SizeBytes         int64  `gorm:"column:size_bytes;not null;default:0"`
	ClaimToken        string `gorm:"column:claim_token;not null;default:''"`
	UploadedBy        *int64 `gorm:"column:uploaded_by"`
	CreatedAt         int64  `gorm:"column:created_at;not null;index:images_created_idx"`
	DeletedAt         *int64 `gorm:"column:deleted_at"`
	FromPublicService int64  `gorm:"column:from_public_service;not null;default:0"`
	ServiceName       string `gorm:"column:service_name;not null;default:''"`
}

func (imageRow) TableName() string { return "images" }

type appConfigRow struct {
	Key       string `gorm:"column:key;primaryKey"`
	Value     string `gorm:"column:value;not null"`
	UpdatedAt int64  `gorm:"column:updated_at;not null"`
}

func (appConfigRow) TableName() string { return "app_config" }

type moderationRow struct {
	ImageID    string `gorm:"column:image_id;primaryKey"`
	TimeWindow string `gorm:"column:time_window;primaryKey;index:image_moderation_window_idx"`
	Hidden     int64  `gorm:"column:hidden;not null;default:0"`
	Deranked   int64  `gorm:"column:deranked;not null;default:0"`
	Note       string `gorm:"column:note;not null;default:''"`
	UpdatedAt  int64  `gorm:"column:updated_at;not null"`
	UpdatedBy  *int64 `gorm:"column:updated_by"`
}

func (moderationRow) TableName() string { return "image_moderation" }

type voteModeRow struct {
	ID          int64  `gorm:"column:id;primaryKey;autoIncrement"`
	Name        string `gorm:"column:name;not null;type:text COLLATE NOCASE;uniqueIndex:vote_modes_name_key"`
	Description string `gorm:"column:description;not null;default:''"`
	MinValue    int    `gorm:"column:min_value;not null;default:1"`
	MaxValue    int    `gorm:"column:max_value;not null;default:5"`
	IsBinary    int64  `gorm:"column:is_binary;not null;default:0"`
	CreatedAt   int64  `gorm:"column:created_at;not null"`
}

func (voteModeRow) TableName() string { return "vote_modes" }

type voteRow struct {
	ID            int64   `gorm:"column:id;primaryKey;autoIncrement"`
	ModeID        int64   `gorm:"column:mode_id;not null;index:votes_mode_idx"`
	ModeName      string  `gorm:"column:mode_name;not null"`
	LeftImageID   string  `gorm:"column:left_image_id;not null;index:votes_left_idx"`
	RightImageID  string  `gorm:"column:right_image_id;not null;index:votes_right_idx"`
	WinnerImageID *string `gorm:"column:winner_image_id"`
	LoserImageID  *string `gorm:"column:loser_image_id"`
	Value         *int    `gorm:"column:value"`
	MinValue      int     `gorm:"column:min_value;not null"`
	MaxValue      int     `gorm:"column:max_value;not null"`
	Weight        float64 `gorm:"column:weight;not null;default:1"`
	UserID        *int64  `gorm:"column:user_id;index:votes_user_idx"`
	Username      string  `gorm:"column:username;not null;default:''"`
	AnonymousID   string  `gorm:"column:anonymous_id;not null;default:'';index:votes_anonymous_idx"`
	Metadata      string  `gorm:"column:metadata;not null;default:'{}'"`
	DurationMs    *int64  `gorm:"column:duration_ms"`
	VoteSpeed     float64 `gorm:"column:vote_speed;not null;default:5.5"`
	CreatedAt     int64   `gorm:"column:created_at;not null;index:votes_created_idx"`
}

func (voteRow) TableName() string { return "votes" }

// --- row <-> model conversion ----------------------------------------------

func unixTime(v int64) time.Time { return time.Unix(v, 0).UTC() }

func unixTimePtr(v *int64) *time.Time {
	if v == nil {
		return nil
	}
	t := unixTime(*v)
	return &t
}

func unixValue(t time.Time) int64 { return t.UTC().Unix() }

func unixValuePtr(t *time.Time) *int64 {
	if t == nil {
		return nil
	}
	v := unixValue(*t)
	return &v
}

func boolValue(v int64) bool { return v != 0 }

func boolInt64(v bool) int64 {
	if v {
		return 1
	}
	return 0
}

func (r *userRow) toModel() *User {
	return &User{
		ID:           r.ID,
		Username:     r.Username,
		DisplayName:  r.DisplayName,
		PasswordHash: r.PasswordHash,
		IsAdmin:      boolValue(r.IsAdmin),
		CreatedAt:    unixTime(r.CreatedAt),
		CreatedBy:    r.CreatedBy,
	}
}

func userToRow(u *User) *userRow {
	return &userRow{
		ID:           u.ID,
		Username:     u.Username,
		DisplayName:  u.DisplayName,
		PasswordHash: u.PasswordHash,
		IsAdmin:      boolInt64(u.IsAdmin),
		CreatedAt:    unixValue(u.CreatedAt),
		CreatedBy:    u.CreatedBy,
	}
}

func (r *imageRow) toModel() *Image {
	return &Image{
		ID:                r.ID,
		Author:            r.Author,
		Title:             r.Title,
		OriginalFilename:  r.OriginalFilename,
		StoragePath:       r.StoragePath,
		MimeType:          r.MimeType,
		Width:             r.Width,
		Height:            r.Height,
		SizeBytes:         r.SizeBytes,
		ClaimToken:        r.ClaimToken,
		UploadedBy:        r.UploadedBy,
		FromPublicService: boolValue(r.FromPublicService),
		ServiceName:       r.ServiceName,
		CreatedAt:         unixTime(r.CreatedAt),
		DeletedAt:         unixTimePtr(r.DeletedAt),
	}
}

func imageToRow(img *Image) *imageRow {
	return &imageRow{
		ID:                img.ID,
		Author:            img.Author,
		Title:             img.Title,
		OriginalFilename:  img.OriginalFilename,
		StoragePath:       img.StoragePath,
		MimeType:          img.MimeType,
		Width:             img.Width,
		Height:            img.Height,
		SizeBytes:         img.SizeBytes,
		ClaimToken:        img.ClaimToken,
		UploadedBy:        img.UploadedBy,
		FromPublicService: boolInt64(img.FromPublicService),
		ServiceName:       img.ServiceName,
		CreatedAt:         unixValue(img.CreatedAt),
		DeletedAt:         unixValuePtr(img.DeletedAt),
	}
}

func (r *voteModeRow) toModel() *VoteMode {
	return &VoteMode{
		ID:          r.ID,
		Name:        r.Name,
		Description: r.Description,
		MinValue:    r.MinValue,
		MaxValue:    r.MaxValue,
		IsBinary:    boolValue(r.IsBinary),
		CreatedAt:   unixTime(r.CreatedAt),
	}
}

func voteModeToRow(m *VoteMode) *voteModeRow {
	return &voteModeRow{
		ID:          m.ID,
		Name:        m.Name,
		Description: m.Description,
		MinValue:    m.MinValue,
		MaxValue:    m.MaxValue,
		IsBinary:    boolInt64(m.IsBinary),
		CreatedAt:   unixValue(m.CreatedAt),
	}
}

func (r *voteRow) toModel() *Vote {
	speed := r.VoteSpeed
	if speed <= 0 {
		speed = VoteSpeedNeutral
	}
	return &Vote{
		ID:            r.ID,
		ModeID:        r.ModeID,
		ModeName:      r.ModeName,
		LeftImageID:   r.LeftImageID,
		RightImageID:  r.RightImageID,
		WinnerImageID: r.WinnerImageID,
		LoserImageID:  r.LoserImageID,
		Value:         r.Value,
		MinValue:      r.MinValue,
		MaxValue:      r.MaxValue,
		Weight:        r.Weight,
		UserID:        r.UserID,
		Username:      r.Username,
		AnonymousID:   r.AnonymousID,
		Metadata:      r.Metadata,
		DurationMs:    r.DurationMs,
		VoteSpeed:     speed,
		CreatedAt:     unixTime(r.CreatedAt),
	}
}

func voteToRow(v *Vote) *voteRow {
	speed := v.VoteSpeed
	if speed <= 0 {
		speed = VoteSpeedNeutral
	}
	return &voteRow{
		ID:            v.ID,
		ModeID:        v.ModeID,
		ModeName:      v.ModeName,
		LeftImageID:   v.LeftImageID,
		RightImageID:  v.RightImageID,
		WinnerImageID: v.WinnerImageID,
		LoserImageID:  v.LoserImageID,
		Value:         v.Value,
		MinValue:      v.MinValue,
		MaxValue:      v.MaxValue,
		Weight:        v.Weight,
		UserID:        v.UserID,
		Username:      v.Username,
		AnonymousID:   v.AnonymousID,
		Metadata:      v.Metadata,
		DurationMs:    v.DurationMs,
		VoteSpeed:     speed,
		CreatedAt:     unixValue(v.CreatedAt),
	}
}

func (r *moderationRow) toModel() *ImageModeration {
	return &ImageModeration{
		ImageID:    r.ImageID,
		TimeWindow: r.TimeWindow,
		Hidden:     boolValue(r.Hidden),
		Deranked:   boolValue(r.Deranked),
		Note:       r.Note,
		UpdatedAt:  unixTime(r.UpdatedAt),
		UpdatedBy:  r.UpdatedBy,
	}
}

func moderationToRow(m *ImageModeration) *moderationRow {
	return &moderationRow{
		ImageID:    m.ImageID,
		TimeWindow: m.TimeWindow,
		Hidden:     boolInt64(m.Hidden),
		Deranked:   boolInt64(m.Deranked),
		Note:       m.Note,
		UpdatedAt:  unixValue(m.UpdatedAt),
		UpdatedBy:  m.UpdatedBy,
	}
}