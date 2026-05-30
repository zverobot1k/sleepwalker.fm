package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"sleepwalker.fm/internal/domain"
)

func (h *SpotifyAPIHandler) cacheGet(ctx context.Context, key string) ([]byte, bool) {
	if h.responseCache == nil {
		return nil, false
	}
	body, err := h.responseCache.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return body, true
}

func (h *SpotifyAPIHandler) cacheSet(ctx context.Context, key string, body []byte, ttl time.Duration) {
	if h.responseCache == nil || ttl <= 0 {
		return
	}
	_ = h.responseCache.Set(ctx, key, body, ttl).Err()
}

func (h *SpotifyAPIHandler) responseCacheKey(parts ...string) string {
	data, _ := json.Marshal(parts)
	return "swfm:response:" + string(data)
}

func (h *SpotifyAPIHandler) writeCachedJSON(c *gin.Context, key string, ttl time.Duration, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if key != "" {
		h.cacheSet(c.Request.Context(), key, body, ttl)
	}
	c.Data(http.StatusOK, "application/json", body)
}

func (h *SpotifyAPIHandler) writeSpotifyError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	apiErr, ok := err.(domain.APIError)
	if !ok {
		return false
	}

	status := apiErr.StatusCode
	if status == 0 {
		status = http.StatusInternalServerError
	}
	if apiErr.RetryAfter != "" {
		c.Header("Retry-After", apiErr.RetryAfter)
	}
	if status == http.StatusTooManyRequests || status >= http.StatusInternalServerError {
		c.JSON(http.StatusOK, gin.H{
			"items":          []any{},
			"audio_features": []any{},
			"top_artists":    []any{},
			"top_tracks":     []any{},
			"top_genres":     []any{},
			"highlights":     []any{},
			"by_day":         []any{},
			"by_hour":        []any{},
			"genres":         []any{},
			"tracks":         []any{},
			"notice":         "spotify temporarily unavailable",
			"source":         "spotify_backoff",
		})
		return true
	}
	resp := gin.H{"error": apiErr.Message}
	if apiErr.RetryAfter != "" {
		resp["retry_after"] = apiErr.RetryAfter
	}
	c.JSON(status, resp)
	return true
}

func newRedisClient(addr string) *redis.Client {
	if addr == "" {
		return nil
	}
	return redis.NewClient(&redis.Options{Addr: addr})
}
