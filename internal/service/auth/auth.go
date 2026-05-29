package auth

import (
	"context"
	"time"

	"sleepwalker.fm/internal/domain"
	"sleepwalker.fm/internal/service/spotify"
	tokenservice "sleepwalker.fm/internal/service/token"
)

// Service wraps OAuth and token refresh for HTTP handlers.
type Service struct {
	oauth  *spotify.OAuthService
	tokens *tokenservice.Service
}

func New(oauth *spotify.OAuthService, tokens *tokenservice.Service) *Service {
	return &Service{oauth: oauth, tokens: tokens}
}

func (s *Service) StartLogin(ctx context.Context) (string, error) {
	return s.oauth.StartLogin(ctx)
}

func (s *Service) HandleCallback(ctx context.Context, code, state string) (domain.SpotifyProfile, error) {
	return s.oauth.HandleCallback(ctx, code, state)
}

// RefreshTokens returns a new access token expiry for the user (refresh token stays server-side).
func (s *Service) RefreshTokens(ctx context.Context, userID string) (RefreshResponse, error) {
	updated, err := s.oauth.RefreshAccessToken(ctx, userID)
	if err != nil {
		return RefreshResponse{}, err
	}
	return RefreshResponse{
		UserID:    updated.UserID,
		ExpiresAt: updated.ExpiresAt,
		Scope:     updated.Scope,
		TokenType: updated.TokenType,
	}, nil
}

type RefreshResponse struct {
	UserID    string    `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	Scope     string    `json:"scope"`
	TokenType string    `json:"token_type"`
}

func (s *Service) SessionState(ctx context.Context, userID string) (tokenservice.SessionState, error) {
	return s.tokens.SessionState(ctx, userID)
}
