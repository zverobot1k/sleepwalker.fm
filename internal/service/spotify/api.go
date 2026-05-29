package spotify

import (
	"context"
	"io"
	"net/http"
	"strings"

	"sleepwalker.fm/internal/config"
	"sleepwalker.fm/internal/domain"
	tokenservice "sleepwalker.fm/internal/service/token"
)

// APIService performs authenticated Spotify Web API requests with automatic token refresh.
type APIService struct {
	cfg    config.Config
	client *http.Client
	tokens *tokenservice.Service
}

func NewAPIService(cfg config.Config, client *http.Client, tokens *tokenservice.Service) *APIService {
	if client == nil {
		client = http.DefaultClient
	}
	return &APIService{cfg: cfg, client: client, tokens: tokens}
}

func (s *APIService) BaseURL() string {
	return s.cfg.SpotifyAPIBaseURL
}

// DoGET performs a GET with the user's access token. Refreshes proactively and retries once on 401.
func (s *APIService) DoGET(ctx context.Context, userID, rawURL string) ([]byte, error) {
	accessToken, err := s.tokens.GetAccessToken(ctx, userID)
	if err != nil {
		return nil, err
	}
	body, err := s.doGETOnce(ctx, accessToken, rawURL)
	if isUnauthorized(err) {
		refreshed, refreshErr := s.tokens.RefreshAfterUnauthorized(ctx, userID)
		if refreshErr != nil {
			return nil, err
		}
		return s.doGETOnce(ctx, refreshed.AccessToken, rawURL)
	}
	return body, err
}

// DoGETWithToken performs a GET with an explicit access token (no refresh). Used for app-token fallback paths.
func (s *APIService) DoGETWithToken(ctx context.Context, accessToken, rawURL string) ([]byte, error) {
	return s.doGETOnce(ctx, accessToken, rawURL)
}

func (s *APIService) doGETOnce(ctx context.Context, accessToken, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, domain.NewError(resp.StatusCode, "spotify api error: "+strings.TrimSpace(string(body)))
	}
	return body, nil
}

func isUnauthorized(err error) bool {
	apiErr, ok := err.(domain.APIError)
	if !ok {
		return false
	}
	return apiErr.StatusCode == http.StatusUnauthorized
}
