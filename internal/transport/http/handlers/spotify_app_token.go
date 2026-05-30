package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

type appTokenCache struct {
	mu        sync.Mutex
	token     string
	expiresAt time.Time
}

type appTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
}

func newAppTokenCache() *appTokenCache {
	return &appTokenCache{}
}

func (c *appTokenCache) get() (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token == "" {
		return "", false
	}
	if time.Now().After(c.expiresAt) {
		c.token = ""
		return "", false
	}
	return c.token, true
}

func (c *appTokenCache) set(token string, expiresAt time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
	c.expiresAt = expiresAt
}

func (h *SpotifyAPIHandler) getAppAccessToken() (string, error) {
	if h.appTokenCache == nil {
		return "", fmt.Errorf("app token cache not initialized")
	}
	if token, ok := h.appTokenCache.get(); ok {
		return token, nil
	}

	clientID := os.Getenv("SPOTIFY_CLIENT_ID")
	clientSecret := os.Getenv("SPOTIFY_CLIENT_SECRET")
	if clientID == "" || clientSecret == "" {
		return "", fmt.Errorf("missing spotify client credentials")
	}

	form := url.Values{}
	form.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", "https://accounts.spotify.com/api/token", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}

	basic := base64.StdEncoding.EncodeToString([]byte(clientID + ":" + clientSecret))
	req.Header.Set("Authorization", "Basic "+basic)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("app token request failed: %s", strings.TrimSpace(string(body)))
	}

	var payload appTokenResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	if payload.AccessToken == "" {
		return "", fmt.Errorf("app token missing access_token")
	}

	expiresAt := time.Now().Add(time.Duration(payload.ExpiresIn) * time.Second)
	if payload.ExpiresIn > 60 {
		expiresAt = expiresAt.Add(-30 * time.Second)
	}
	h.appTokenCache.set(payload.AccessToken, expiresAt)

	return payload.AccessToken, nil
}

func (h *SpotifyAPIHandler) doSpotifyGETWithAppFallback(ctx context.Context, userID, accessToken string, url string) ([]byte, error) {
	if h.spotifyAPI != nil {
		body, err := h.spotifyAPI.DoGETWithToken(ctx, userID, accessToken, url)
		if err == nil {
			return body, nil
		}
		log.Printf("DIAG spotify app-fallback via APIService failed url=%s err=%v", url, err)
		if !isSpotifyForbidden(err) && !isSpotifyRateLimited(err) {
			return nil, err
		}
	}
	body, err := h.doSpotifyGET(accessToken, url)
	if err == nil {
		return body, nil
	}
	log.Printf("DIAG spotify app-fallback: initial request failed url=%s err=%v", url, err)
	if !isSpotifyForbidden(err) && !isSpotifyRateLimited(err) {
		return nil, err
	}
	appToken, appErr := h.getAppAccessToken()
	if appErr != nil {
		log.Printf("DIAG spotify app-fallback: getAppAccessToken failed: %v", appErr)
		return nil, err
	}
	log.Printf("DIAG spotify app-fallback: using app token for url=%s", url)
	if h.spotifyAPI != nil {
		body, appErr = h.spotifyAPI.DoGETWithToken(ctx, userID, appToken, url)
	} else {
		body, appErr = h.doSpotifyGET(appToken, url)
	}
	if appErr != nil {
		log.Printf("DIAG spotify app-fallback: app request err=%v", appErr)
	}
	return body, appErr
}
