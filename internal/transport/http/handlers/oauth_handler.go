package handlers

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	authservice "sleepwalker.fm/internal/service/auth"
)

type OAuthHandler struct {
	auth        *authservice.Service
	frontendURL string
}

func NewOAuthHandler(auth *authservice.Service, frontendURL string) *OAuthHandler {
	return &OAuthHandler{auth: auth, frontendURL: frontendURL}
}

func (h *OAuthHandler) Login(c *gin.Context) {
	url, err := h.auth.StartLogin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusFound, url)
}

func (h *OAuthHandler) Callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	profile, err := h.auth.HandleCallback(c.Request.Context(), code, state)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	q := url.Values{}
	q.Set("user_id", profile.ID)
	q.Set("display_name", profile.DisplayName)
	q.Set("email", profile.Email)
	c.Redirect(http.StatusFound, h.frontendURL+"/auth/callback?"+q.Encode())
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
