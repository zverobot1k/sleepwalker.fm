package handlers

import (
	"log"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	authservice "sleepwalker.fm/internal/service/auth"
)

type OAuthHandler struct {
	auth        *authservice.Service
	frontendURL string
}

func NewOAuthHandler(auth *authservice.Service, frontendURL string) *OAuthHandler {
	return &OAuthHandler{
		auth:        auth,
		frontendURL: strings.TrimRight(frontendURL, "/"),
	}
}

func (h *OAuthHandler) Login(c *gin.Context) {
	url, err := h.auth.StartLogin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusFound, url)
}

// Callback handles Spotify redirect at GET /auth/spotify/callback (see SPOTIFY_REDIRECT_URI).
func (h *OAuthHandler) Callback(c *gin.Context) {
	log.Printf("oauth callback: %s %s", c.Request.Method, c.Request.URL.RequestURI())

	if errParam := c.Query("error"); errParam != "" {
		desc := c.Query("error_description")
		log.Printf("oauth callback: spotify error=%q description=%q", errParam, desc)
		target := h.frontendURL + "/?oauth_error=" + url.QueryEscape(errParam)
		log.Printf("oauth callback: redirecting to frontend error page %s", target)
		c.Redirect(http.StatusFound, target)
		return
	}

	code := c.Query("code")
	state := c.Query("state")
	if code == "" {
		log.Printf("oauth callback: missing code")
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing authorization code"})
		return
	}

	profile, err := h.auth.HandleCallback(c.Request.Context(), code, state)
	if err != nil {
		log.Printf("oauth callback: token exchange failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("oauth callback: authenticated spotify user_id=%s", profile.ID)

	q := url.Values{}
	q.Set("user_id", profile.ID)
	if profile.DisplayName != "" {
		q.Set("display_name", profile.DisplayName)
	}
	if profile.Email != "" {
		q.Set("email", profile.Email)
	}

	// Frontend app URL (FRONTEND_URL) — never BASE_URL /auth/callback on the API host.
	target := h.frontendURL + "/dashboard?" + q.Encode()
	log.Printf("oauth callback: redirecting to frontend dashboard host=%s path=/dashboard", h.frontendURL)
	c.Redirect(http.StatusFound, target)
}

func (h *OAuthHandler) Refresh(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId"})
		return
	}

	updated, err := h.auth.RefreshTokens(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"user_id":    updated.UserID,
		"expires_at": updated.ExpiresAt,
		"scope":      updated.Scope,
		"token_type": updated.TokenType,
	})
}

// Session returns safe session state for the frontend (no refresh token).
func (h *OAuthHandler) Session(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId"})
		return
	}
	state, err := h.auth.SessionState(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, state)
}
