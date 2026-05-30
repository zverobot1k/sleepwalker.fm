package spotify

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"sleepwalker.fm/internal/config"
	"sleepwalker.fm/internal/domain"
	tokenservice "sleepwalker.fm/internal/service/token"
)

// APIService performs authenticated Spotify Web API requests with automatic token refresh.
type APIService struct {
	cfg    config.Config
	client *http.Client
	tokens *tokenservice.Service
	cache  *redis.Client
	group  singleflight.Group
}

const (
	spotifyGlobalRequestsPerSecond = 6
	spotifyUserRequestsPerSecond   = 1
	spotifyRateWindow              = time.Second
	spotifyDegradedTTL             = 60 * time.Second
	spotifyStateTTL                = 5 * time.Minute
	spotifyRefreshTTL              = 15 * time.Second
	hardRetryAfterThreshold        = 1000
)

func NewAPIService(cfg config.Config, client *http.Client, tokens *tokenservice.Service, cache *redis.Client) *APIService {
	if client == nil {
		client = http.DefaultClient
	}
	return &APIService{cfg: cfg, client: client, tokens: tokens, cache: cache}
}

func (s *APIService) BaseURL() string {
	return s.cfg.SpotifyAPIBaseURL
}

// DoGET performs a GET with cache-first, request coalescing, refresh-on-401, and backoff-aware degradation.
func (s *APIService) DoGET(ctx context.Context, userID, rawURL string) ([]byte, error) {
	log.Printf("DIAG spotify DoGET start: user=%s url=%s", userID, rawURL)
	cacheKey, freshTTL, staleTTL := s.readCacheDescriptor(userID, rawURL)
	return s.doSharedGET(ctx, userID, cacheKey, freshTTL, staleTTL, rawURL, "")
}

// DoGETWithToken performs a GET with an explicit access token (no refresh). Used for app-token fallback paths.
func (s *APIService) DoGETWithToken(ctx context.Context, userID, accessToken, rawURL string) ([]byte, error) {
	cacheKey, freshTTL, staleTTL := s.readCacheDescriptor(userID, rawURL)
	return s.doSharedGET(ctx, userID, cacheKey, freshTTL, staleTTL, rawURL, accessToken)
}

func (s *APIService) doSharedGET(ctx context.Context, userID, cacheKey string, freshTTL, staleTTL time.Duration, rawURL, accessToken string) ([]byte, error) {
	if body, ok := s.readFreshCache(ctx, cacheKey); ok {
		return body, nil
	}
	if state := s.readSpotifyState(ctx, userID); state == "BLOCKED" || state == "DEGRADED" {
		if body, ok := s.readStaleCache(ctx, cacheKey); ok {
			return body, nil
		}
		return emptySpotifyPayload(), nil
	}

	result, err, _ := s.group.Do(cacheKey, func() (any, error) {
		if body, ok := s.readFreshCache(ctx, cacheKey); ok {
			return body, nil
		}
		if state := s.readSpotifyState(ctx, userID); state == "BLOCKED" || state == "DEGRADED" {
			if body, ok := s.readStaleCache(ctx, cacheKey); ok {
				return body, nil
			}
			return emptySpotifyPayload(), nil
		}
		if ok := s.acquireSpotifyRequestBudget(ctx, userID); !ok {
			s.markSpotifyState(ctx, userID, "DEGRADED", spotifyDegradedTTL)
			if body, ok := s.readStaleCache(ctx, cacheKey); ok {
				return body, nil
			}
			return emptySpotifyPayload(), nil
		}

		body, err := s.doAuthorizedGET(ctx, userID, rawURL, accessToken)
		if err == nil {
			s.markSpotifyState(ctx, userID, "OK", spotifyStateTTL)
			s.writeCache(ctx, cacheKey, body, freshTTL, staleTTL)
			return body, nil
		}

		apiErr, ok := err.(domain.APIError)
		if !ok {
			s.markSpotifyState(ctx, userID, "DEGRADED", spotifyDegradedTTL)
			if body, ok := s.readStaleCache(ctx, cacheKey); ok {
				return body, nil
			}
			return emptySpotifyPayload(), nil
		}

		if apiErr.StatusCode == http.StatusUnauthorized && accessToken == "" {
			log.Printf("DIAG spotify unauthorized: user=%s url=%s — attempting refresh", userID, rawURL)
			refreshed, refreshErr := s.tokens.RefreshAfterUnauthorized(ctx, userID)
			if refreshErr == nil {
				log.Printf("DIAG spotify refresh succeeded: user=%s expires_at=%s", userID, refreshed.ExpiresAt)
				body, err = s.doAuthorizedGET(ctx, userID, rawURL, refreshed.AccessToken)
				if err == nil {
					s.markSpotifyState(ctx, userID, "OK", spotifyStateTTL)
					s.writeCache(ctx, cacheKey, body, freshTTL, staleTTL)
					return body, nil
				}
				if retryErr, ok := err.(domain.APIError); ok {
					apiErr = retryErr
				} else {
					s.markSpotifyState(ctx, userID, "DEGRADED", spotifyDegradedTTL)
					if body, ok := s.readStaleCache(ctx, cacheKey); ok {
						return body, nil
					}
					return emptySpotifyPayload(), nil
				}
			} else {
				log.Printf("DIAG spotify refresh failed: user=%s err=%v\n%s", userID, refreshErr, debug.Stack())
			}
		}

		if apiErr.StatusCode == http.StatusTooManyRequests {
			retryAfter := parseRetryAfterSeconds(apiErr.RetryAfter)
			if retryAfter > 0 {
				if retryAfter >= hardRetryAfterThreshold {
					s.blockSpotifyUser(ctx, userID, retryAfter)
				} else {
					s.markSpotifyState(ctx, userID, "DEGRADED", spotifyDegradedTTL)
				}
			}
			if body, ok := s.readStaleCache(ctx, cacheKey); ok {
				return body, nil
			}
			return emptySpotifyPayload(), nil
		}

		if apiErr.StatusCode >= 500 {
			s.markSpotifyState(ctx, userID, "DEGRADED", spotifyDegradedTTL)
			if body, ok := s.readStaleCache(ctx, cacheKey); ok {
				return body, nil
			}
			return emptySpotifyPayload(), nil
		}

		return nil, err
	})
	if err != nil {
		if apiErr, ok := err.(domain.APIError); ok && (apiErr.StatusCode == http.StatusTooManyRequests || apiErr.StatusCode >= 500) {
			if body, ok := s.readStaleCache(ctx, cacheKey); ok {
				return body, nil
			}
			return emptySpotifyPayload(), nil
		}
		return nil, err
	}
	return result.([]byte), nil
}

func (s *APIService) doGETOnce(ctx context.Context, accessToken, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := s.client.Do(req)
	if err != nil {
		log.Printf("DIAG spotify http do error: url=%s err=%v\n%s", rawURL, err, debug.Stack())
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("DIAG spotify read body error: url=%s err=%v\n%s", rawURL, err, debug.Stack())
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		log.Printf("DIAG spotify non-200: url=%s status=%d retry-after=%s body=%s", rawURL, resp.StatusCode, resp.Header.Get("Retry-After"), snippet(body, 500))
		apiErr := domain.NewError(resp.StatusCode, "spotify api error: "+strings.TrimSpace(string(body)))
		apiErr.RetryAfter = resp.Header.Get("Retry-After")
		return nil, apiErr
	}
	return body, nil
}

func (s *APIService) doAuthorizedGET(ctx context.Context, userID, rawURL, accessToken string) ([]byte, error) {
	if accessToken == "" {
		if s.tokens == nil {
			return nil, fmt.Errorf("missing access token")
		}
		resolved, err := s.tokens.GetAccessToken(ctx, userID)
		if err != nil {
			log.Printf("DIAG spotify GetAccessToken error: user=%s err=%v\n%s", userID, err, debug.Stack())
			return nil, err
		}
		accessToken = resolved
	}
	return s.doGETOnce(ctx, accessToken, rawURL)
}

func (s *APIService) readCacheDescriptor(userID, rawURL string) (string, time.Duration, time.Duration) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return s.genericCacheKey(userID, rawURL), 5 * time.Minute, 12 * time.Hour
	}
	query := parsed.Query().Encode()
	switch parsed.Path {
	case "/v1/me/top/tracks":
		return "spotify:top_tracks:" + userID + ":" + query, 10 * time.Minute, 12 * time.Hour
	case "/v1/me/top/artists":
		return "spotify:top_artists:" + userID + ":" + query, 10 * time.Minute, 12 * time.Hour
	case "/v1/me/player/recently-played":
		return "spotify:recently_played:" + userID + ":" + query, 2 * time.Minute, 2 * time.Hour
	case "/v1/audio-features":
		return "spotify:audio_features:" + userID + ":" + fingerprint(query), 20 * time.Minute, 12 * time.Hour
	case "/v1/recommendations":
		return "spotify:recommendations:" + userID + ":" + fingerprint(query), 10 * time.Minute, 6 * time.Hour
	default:
		return s.genericCacheKey(userID, rawURL), 5 * time.Minute, 12 * time.Hour
	}
}

func (s *APIService) genericCacheKey(userID, rawURL string) string {
	return "spotify:read:" + userID + ":" + fingerprint(rawURL)
}

func (s *APIService) acquireSpotifyRequestBudget(ctx context.Context, userID string) bool {
	if s.cache == nil {
		return true
	}
	second := time.Now().Unix()
	globalKey := fmt.Sprintf("spotify:rate:global:%d", second)
	userKey := fmt.Sprintf("spotify:rate:user:%s:%d", userID, second)
	script := `
local globalKey = KEYS[1]
local userKey = KEYS[2]
local globalLimit = tonumber(ARGV[1])
local userLimit = tonumber(ARGV[2])
local ttl = tonumber(ARGV[3])
local g = redis.call("INCR", globalKey)
if g == 1 then redis.call("EXPIRE", globalKey, ttl) end
local u = redis.call("INCR", userKey)
if u == 1 then redis.call("EXPIRE", userKey, ttl) end
if g > globalLimit or u > userLimit then return 0 end
return 1
`
	res, err := s.cache.Eval(ctx, script, []string{globalKey, userKey}, spotifyGlobalRequestsPerSecond, spotifyUserRequestsPerSecond, int(spotifyRateWindow.Seconds())+1).Int64()
	if err != nil {
		log.Printf("warning: spotify rate limiter unavailable: %v", err)
		return true
	}
	return res == 1
}

func (s *APIService) readSpotifyState(ctx context.Context, userID string) string {
	if s.cache == nil || userID == "" {
		return "OK"
	}
	if blockUntil, err := s.cache.Get(ctx, spotifyBlockKey(userID)).Int64(); err == nil && time.Now().Unix() < blockUntil {
		return "BLOCKED"
	}
	if state, err := s.cache.Get(ctx, spotifyStateKey(userID)).Result(); err == nil && state != "" {
		return state
	}
	return "OK"
}

func (s *APIService) markSpotifyState(ctx context.Context, userID, state string, ttl time.Duration) {
	if s.cache == nil || userID == "" {
		return
	}
	if ttl <= 0 {
		ttl = spotifyStateTTL
	}
	_ = s.cache.Set(ctx, spotifyStateKey(userID), state, ttl).Err()
	if state == "OK" {
		_ = s.cache.Del(ctx, spotifyBlockKey(userID)).Err()
	}
}

func (s *APIService) blockSpotifyUser(ctx context.Context, userID string, retryAfterSeconds int) {
	if s.cache == nil || userID == "" || retryAfterSeconds <= 0 {
		return
	}
	blockedUntil := time.Now().Add(time.Duration(retryAfterSeconds) * time.Second).Unix()
	_ = s.cache.Set(ctx, spotifyBlockKey(userID), blockedUntil, time.Duration(retryAfterSeconds)*time.Second).Err()
	_ = s.cache.Set(ctx, spotifyStateKey(userID), "BLOCKED", time.Duration(retryAfterSeconds)*time.Second).Err()
}

func spotifyStateKey(userID string) string {
	return "spotify:state:" + userID
}

func spotifyBlockKey(userID string) string {
	return "spotify:block:" + userID
}

func (s *APIService) readFreshCache(ctx context.Context, key string) ([]byte, bool) {
	if s.cache == nil {
		return nil, false
	}
	body, err := s.cache.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	return body, true
}

func (s *APIService) readStaleCache(ctx context.Context, key string) ([]byte, bool) {
	if s.cache == nil {
		return nil, false
	}
	body, err := s.cache.Get(ctx, staleCacheKey(key)).Bytes()
	if err != nil {
		return nil, false
	}
	return body, true
}

func (s *APIService) writeCache(ctx context.Context, key string, body []byte, freshTTL, staleTTL time.Duration) {
	if s.cache == nil || len(body) == 0 {
		return
	}
	_ = s.cache.Set(ctx, key, body, freshTTL).Err()
	if staleTTL > freshTTL {
		_ = s.cache.Set(ctx, staleCacheKey(key), body, staleTTL).Err()
	}
}

func staleCacheKey(key string) string {
	return key + ":stale"
}

func fingerprint(value string) string {
	hash := sha256.Sum256([]byte(value))
	return hex.EncodeToString(hash[:16])
}

func (s *APIService) userBackoffActive(ctx context.Context, userID string) bool {
	if s.cache == nil || userID == "" {
		return false
	}
	value, err := s.cache.Get(ctx, "spotify:backoff:"+userID).Int64()
	if err != nil {
		return false
	}
	return time.Now().Unix() < value
}

func (s *APIService) enterBackoff(ctx context.Context, userID string, retryAfterSeconds int) {
	if s.cache == nil || userID == "" || retryAfterSeconds <= 0 {
		return
	}
	expiresAt := time.Now().Add(time.Duration(retryAfterSeconds) * time.Second)
	_ = s.cache.Set(ctx, "spotify:backoff:"+userID, expiresAt.Unix(), time.Duration(retryAfterSeconds)*time.Second).Err()
}

func parseRetryAfterSeconds(value string) int {
	if value == "" {
		return 0
	}
	seconds, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0
	}
	return seconds
}

func emptySpotifyPayload() []byte {
	return []byte(`{"items":[],"audio_features":[],"top_artists":[],"top_tracks":[],"top_genres":[],"highlights":[],"by_day":[],"by_hour":[],"genres":[],"tracks":[],"notice":"spotify temporarily unavailable","source":"spotify_backoff"}`)
}

func snippet(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n]
}
