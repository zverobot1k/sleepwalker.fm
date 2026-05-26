package postgres

import "time"

type OAuthState struct {
	State        string    `gorm:"primaryKey;column:state"`
	CodeVerifier string    `gorm:"column:code_verifier;not null"`
	ExpiresAt    time.Time `gorm:"column:expires_at;not null"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime"`
}

func (OAuthState) TableName() string {
	return "oauth_state"
}

type SpotifyToken struct {
	UserID       string    `gorm:"primaryKey;column:user_id"`
	AccessToken  string    `gorm:"column:access_token;not null"`
	RefreshToken string    `gorm:"column:refresh_token;not null"`
	Scope        string    `gorm:"column:scope;not null"`
	TokenType    string    `gorm:"column:token_type;not null"`
	ExpiresAt    time.Time `gorm:"column:expires_at;not null"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (SpotifyToken) TableName() string {
	return "spotify_tokens"
}
