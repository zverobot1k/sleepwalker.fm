package token

import (
	"context"
	"fmt"
	"time"

	"sleepwalker.fm/internal/domain"
	"sleepwalker.fm/internal/repository/postgres"
)

const RefreshBuffer = 5 * time.Minute

// OAuthRefresher exchanges a stored refresh token for a new access token.
type OAuthRefresher interface {
	RefreshAccessToken(ctx context.Context, userID string) (domain.SpotifyTokens, error)
}

// Service manages Spotify access tokens server-side (refresh tokens never leave the backend).
type Service struct {
	repo  *postgres.TokenRepo
	oauth OAuthRefresher
}

func NewService(repo *postgres.TokenRepo, oauth OAuthRefresher) *Service {
	return &Service{repo: repo, oauth: oauth}
}

func (s *Service) NeedsRefresh(tokens domain.SpotifyTokens) bool {
	if tokens.AccessToken == "" {
		return true
	}
	return time.Now().Add(RefreshBuffer).After(tokens.ExpiresAt)
}

func (s *Service) GetTokens(ctx context.Context, userID string) (domain.SpotifyTokens, error) {
	if userID == "" {
		return domain.SpotifyTokens{}, fmt.Errorf("missing user id")
	}
	return s.repo.GetByUserID(ctx, userID)
}

// EnsureFresh returns a valid access token, refreshing proactively when within RefreshBuffer of expiry.
func (s *Service) EnsureFresh(ctx context.Context, userID string) (domain.SpotifyTokens, error) {
	tokens, err := s.GetTokens(ctx, userID)
	if err != nil {
		return domain.SpotifyTokens{}, fmt.Errorf("user tokens not found")
	}
	if tokens.AccessToken == "" {
		return domain.SpotifyTokens{}, fmt.Errorf("no access token")
	}
	if !s.NeedsRefresh(tokens) {
		return tokens, nil
	}
	if s.oauth == nil {
		return tokens, nil
	}
	return s.oauth.RefreshAccessToken(ctx, userID)
}

// GetAccessToken returns a valid access token for API calls.
func (s *Service) GetAccessToken(ctx context.Context, userID string) (string, error) {
	tokens, err := s.EnsureFresh(ctx, userID)
	if err != nil {
		return "", err
	}
	return tokens.AccessToken, nil
}

// RefreshAfterUnauthorized forces a token refresh (e.g. after Spotify 401).
func (s *Service) RefreshAfterUnauthorized(ctx context.Context, userID string) (domain.SpotifyTokens, error) {
	if s.oauth == nil {
		return domain.SpotifyTokens{}, fmt.Errorf("oauth service unavailable")
	}
	return s.oauth.RefreshAccessToken(ctx, userID)
}

// SessionState is safe to expose to the frontend (no refresh token).
type SessionState struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Scope     string    `json:"scope"`
	Connected bool      `json:"connected"`
}

func (s *Service) SessionState(ctx context.Context, userID string) (SessionState, error) {
	tokens, err := s.GetTokens(ctx, userID)
	if err != nil {
		return SessionState{UserID: userID, Connected: false}, nil
	}
	return SessionState{
		UserID:    userID,
		ExpiresAt: tokens.ExpiresAt,
		Scope:     tokens.Scope,
		Connected: tokens.AccessToken != "",
	}, nil
}
