package domain

import (
	"fmt"
	"time"
)

type SpotifyProfile struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Email       string `json:"email"`
}

type OAuthState struct {
	State     string
	Verifier  string
	ExpiresAt time.Time
	CreatedAt time.Time
}

type SpotifyTokens struct {
	UserID       string
	AccessToken  string
	RefreshToken string
	Scope        string
	TokenType    string
	ExpiresAt    time.Time
	UpdatedAt    time.Time
}

// APIError — ошибка от внешнего API
type APIError struct {
	StatusCode int
	Message    string
	RetryAfter string
}

func (e APIError) Error() string {
	return fmt.Sprintf("api error: %d - %s", e.StatusCode, e.Message)
}

func NewError(code int, msg string) APIError {
	return APIError{StatusCode: code, Message: msg}
}
