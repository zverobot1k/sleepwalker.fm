package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"sleepwalker.fm/internal/config"
	"sleepwalker.fm/internal/domain"
	"sleepwalker.fm/internal/repository/postgres"
)

type OAuthService struct {
	cfg        config.Config
	httpClient *http.Client
	stateRepo  *postgres.OAuthStateRepo
	tokenRepo  *postgres.TokenRepo
}

func NewOAuthService(cfg config.Config, client *http.Client, stateRepo *postgres.OAuthStateRepo, tokenRepo *postgres.TokenRepo) *OAuthService {
	return &OAuthService{
		cfg:        cfg,
		httpClient: client,
		stateRepo:  stateRepo,
		tokenRepo:  tokenRepo,
	}
}

func (s *OAuthService) StartLogin(ctx context.Context) (string, error) {
	state, err := randomString(32)
	if err != nil {
		return "", err
	}
	verifier, err := randomString(64)
	if err != nil {
		return "", err
	}
	challenge := codeChallenge(verifier)

	expiresAt := time.Now().Add(10 * time.Minute)
	if err := s.stateRepo.Create(ctx, state, verifier, expiresAt); err != nil {
		return "", err
	}

	params := url.Values{}
	params.Set("response_type", "code")
	params.Set("client_id", s.cfg.SpotifyClientID)
	redirectURI := strings.TrimSpace(s.cfg.SpotifyRedirectURI)
	params.Set("redirect_uri", redirectURI)
	params.Set("scope", strings.Join(s.cfg.SpotifyScopes, " "))
	params.Set("state", state)
	params.Set("code_challenge_method", "S256")
	params.Set("code_challenge", challenge)

	authURL := s.cfg.SpotifyAuthorizeURL + "?" + params.Encode()
	log.Printf("spotify oauth: authorization request redirect_uri=%q (encoded in URL)", redirectURI)
	return authURL, nil
}

func (s *OAuthService) HandleCallback(ctx context.Context, code, state string) (domain.SpotifyProfile, error) {
	if code == "" || state == "" {
		return domain.SpotifyProfile{}, errors.New("missing code or state")
	}
	verifier, err := s.stateRepo.GetAndDelete(ctx, state)
	if err != nil {
		return domain.SpotifyProfile{}, fmt.Errorf("invalid or expired state: %w", err)
	}

	tokenResp, err := s.exchangeToken(ctx, code, verifier)
	if err != nil {
		return domain.SpotifyProfile{}, err
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	if tokenResp.RefreshToken == "" {
		return domain.SpotifyProfile{}, errors.New("spotify did not return refresh_token")
	}

	userID := spotifyUserID(tokenResp.RefreshToken)

	if err := s.tokenRepo.UpsertTokens(ctx, domain.SpotifyTokens{
		UserID:       userID,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: tokenResp.RefreshToken,
		Scope:        tokenResp.Scope,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    expiresAt,
	}); err != nil {
		return domain.SpotifyProfile{}, err
	}

	return domain.SpotifyProfile{ID: userID}, nil
}

func (s *OAuthService) RefreshAccessToken(ctx context.Context, userID string) (domain.SpotifyTokens, error) {
	stored, err := s.tokenRepo.GetByUserID(ctx, userID)
	if err != nil {
		return domain.SpotifyTokens{}, err
	}

	form := url.Values{}
	form.Set("grant_type", "refresh_token")
	form.Set("refresh_token", stored.RefreshToken)
	form.Set("client_id", s.cfg.SpotifyClientID)

	resp, err := s.httpClient.PostForm(s.cfg.SpotifyTokenURL, form)
	if err != nil {
		return domain.SpotifyTokens{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return domain.SpotifyTokens{}, fmt.Errorf("token refresh failed: %s", strings.TrimSpace(string(body)))
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return domain.SpotifyTokens{}, err
	}

	refreshToken := stored.RefreshToken
	if tokenResp.RefreshToken != "" {
		refreshToken = tokenResp.RefreshToken
	}

	updated := domain.SpotifyTokens{
		UserID:       stored.UserID,
		AccessToken:  tokenResp.AccessToken,
		RefreshToken: refreshToken,
		Scope:        tokenResp.Scope,
		TokenType:    tokenResp.TokenType,
		ExpiresAt:    time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
	}

	if err := s.tokenRepo.UpsertTokens(ctx, updated); err != nil {
		return domain.SpotifyTokens{}, err
	}

	return updated, nil
}

func (s *OAuthService) exchangeToken(ctx context.Context, code, verifier string) (tokenResponse, error) {
	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", code)
	redirectURI := strings.TrimSpace(s.cfg.SpotifyRedirectURI)
	form.Set("redirect_uri", redirectURI)
	form.Set("client_id", s.cfg.SpotifyClientID)
	form.Set("code_verifier", verifier)

	log.Printf("spotify oauth: token exchange redirect_uri=%q", redirectURI)
	resp, err := s.httpClient.PostForm(s.cfg.SpotifyTokenURL, form)
	if err != nil {
		return tokenResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return tokenResponse{}, fmt.Errorf("token exchange failed: %s", strings.TrimSpace(string(body)))
	}

	var tokenResp tokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return tokenResponse{}, err
	}
	return tokenResp, nil
}

func (s *OAuthService) fetchProfile(ctx context.Context, accessToken string) (domain.SpotifyProfile, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.SpotifyAPIBaseURL+"/me", nil)
	if err != nil {
		return domain.SpotifyProfile{}, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return domain.SpotifyProfile{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		return domain.SpotifyProfile{}, fmt.Errorf("profile fetch failed: %s", strings.TrimSpace(string(body)))
	}

	var profile domain.SpotifyProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return domain.SpotifyProfile{}, err
	}
	return profile, nil
}

func randomString(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func codeChallenge(verifier string) string {
	hash := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(hash[:])
}

func spotifyUserID(refreshToken string) string {
	hash := sha256.Sum256([]byte(refreshToken))
	return "spotify_" + hex.EncodeToString(hash[:16])
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	Scope        string `json:"scope"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
}
