package handlers

import (
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	"sleepwalker.fm/internal/service/spotify"
)

type OAuthHandler struct {
	oauth       *spotify.OAuthService
	frontendURL string
}

func NewOAuthHandler(oauth *spotify.OAuthService, frontendURL string) *OAuthHandler {
	return &OAuthHandler{oauth: oauth, frontendURL: frontendURL}
}

func (h *OAuthHandler) Login(c *gin.Context) {
	url, err := h.oauth.StartLogin(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.Redirect(http.StatusFound, url)
}

func (h *OAuthHandler) Callback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	profile, err := h.oauth.HandleCallback(c.Request.Context(), code, state)
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

	updated, err := h.oauth.RefreshAccessToken(c.Request.Context(), userID)
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
