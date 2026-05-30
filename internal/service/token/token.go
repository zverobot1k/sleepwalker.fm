package token

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	"sleepwalker.fm/internal/domain"
	"sleepwalker.fm/internal/repository/postgres"
)

const RefreshBuffer = 5 * time.Minute
const (
	spotifyGlobalRequestsPerSecond = 6
	spotifyUserRequestsPerSecond   = 1
	spotifyRateWindow              = time.Second
)

// OAuthRefresher exchanges a stored refresh token for a new access token.
type OAuthRefresher interface {
	RefreshAccessToken(ctx context.Context, userID string) (domain.SpotifyTokens, error)
}

// Service manages Spotify access tokens server-side (refresh tokens never leave the backend).
type Service struct {
	repo  *postgres.TokenRepo
	oauth OAuthRefresher
	redis *redis.Client
	group singleflight.Group
}

func NewService(repo *postgres.TokenRepo, oauth OAuthRefresher, redisClient *redis.Client) *Service {
	return &Service{repo: repo, oauth: oauth, redis: redisClient}
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
		log.Printf("DIAG token EnsureFresh: user=%s tokens not found: %v", userID, err)
		return domain.SpotifyTokens{}, fmt.Errorf("user tokens not found")
	}
	if tokens.AccessToken == "" {
		log.Printf("DIAG token EnsureFresh: user=%s no access token present", userID)
		return domain.SpotifyTokens{}, fmt.Errorf("no access token")
	}
	needs := s.NeedsRefresh(tokens)
	log.Printf("DIAG token EnsureFresh: user=%s access_exists=true expires_at=%s needs_refresh=%v", userID, tokens.ExpiresAt, needs)
	if !needs {
		return tokens, nil
	}
	if s.oauth == nil {
		log.Printf("DIAG token EnsureFresh: user=%s oauth service nil — returning existing tokens", userID)
		return tokens, nil
	}
	log.Printf("DIAG token EnsureFresh: user=%s attempting proactive refresh", userID)
	result, err, _ := s.group.Do(userID, func() (any, error) {
		refreshed, refreshErr := s.refreshWithLock(ctx, userID, false)
		if refreshErr != nil {
			return tokens, refreshErr
		}
		return refreshed, nil
	})
	if err != nil {
		log.Printf("DIAG token EnsureFresh: user=%s refresh failed; using existing tokens: %v", userID, err)
		return tokens, nil
	}
	refreshed, _ := result.(domain.SpotifyTokens)
	log.Printf("DIAG token EnsureFresh: user=%s refresh succeeded expires_at=%s", userID, refreshed.ExpiresAt)
	return refreshed, nil
}

// GetAccessToken returns a valid access token for API calls.
func (s *Service) GetAccessToken(ctx context.Context, userID string) (string, error) {
	tokens, err := s.EnsureFresh(ctx, userID)
	if err != nil {
		log.Printf("DIAG token GetAccessToken: user=%s ensure fresh error: %v", userID, err)
		return "", err
	}
	return tokens.AccessToken, nil
}

// RefreshAfterUnauthorized forces a token refresh (e.g. after Spotify 401).
func (s *Service) RefreshAfterUnauthorized(ctx context.Context, userID string) (domain.SpotifyTokens, error) {
	if s.oauth == nil {
		log.Printf("DIAG token RefreshAfterUnauthorized: user=%s oauth service unavailable", userID)
		return domain.SpotifyTokens{}, fmt.Errorf("oauth service unavailable")
	}
	log.Printf("DIAG token RefreshAfterUnauthorized: user=%s forcing refresh", userID)
	t, err := s.refreshWithLock(ctx, userID, true)
	if err != nil {
		log.Printf("DIAG token RefreshAfterUnauthorized: user=%s refresh failed: %v", userID, err)
		return domain.SpotifyTokens{}, err
	}
	log.Printf("DIAG token RefreshAfterUnauthorized: user=%s refresh succeeded expires_at=%s", userID, t.ExpiresAt)
	return t, nil
}

func (s *Service) refreshWithLock(ctx context.Context, userID string, force bool) (domain.SpotifyTokens, error) {
	current, err := s.repo.GetByUserID(ctx, userID)
	if err != nil {
		return domain.SpotifyTokens{}, err
	}
	if !force && !s.NeedsRefresh(current) {
		return current, nil
	}

	if s.redis == nil {
		return s.oauth.RefreshAccessToken(ctx, userID)
	}

	lockKey := refreshLockKey(userID)
	token, acquired, err := s.tryAcquireLock(ctx, lockKey, 15*time.Second)
	if err != nil {
		return domain.SpotifyTokens{}, err
	}
	if !acquired {
		return s.waitForRefresh(ctx, userID, current)
	}
	defer s.releaseLock(context.Background(), lockKey, token)

	latest, err := s.repo.GetByUserID(ctx, userID)
	if err == nil && !force && !s.NeedsRefresh(latest) {
		return latest, nil
	}
	if !s.acquireSpotifyRequestBudget(ctx, userID) {
		return current, fmt.Errorf("spotify refresh rate limited")
	}

	updated, err := s.oauth.RefreshAccessToken(ctx, userID)
	if err != nil {
		return domain.SpotifyTokens{}, err
	}
	return updated, nil
}

func (s *Service) waitForRefresh(ctx context.Context, userID string, baseline domain.SpotifyTokens) (domain.SpotifyTokens, error) {
	delay := 100 * time.Millisecond
	deadline := time.NewTimer(14 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-ctx.Done():
			return baseline, ctx.Err()
		case <-deadline.C:
			current, err := s.repo.GetByUserID(ctx, userID)
			if err == nil && !s.NeedsRefresh(current) {
				return current, nil
			}
			return baseline, fmt.Errorf("refresh still pending")
		case <-time.After(delay):
			current, err := s.repo.GetByUserID(ctx, userID)
			if err == nil && !s.NeedsRefresh(current) {
				return current, nil
			}
			if delay < 2*time.Second {
				delay *= 2
			}
		}
	}
}

func (s *Service) tryAcquireLock(ctx context.Context, key string, ttl time.Duration) (string, bool, error) {
	if s.redis == nil {
		return "", false, nil
	}
	token := fmt.Sprintf("%d", time.Now().UnixNano())
	ok, err := s.redis.SetNX(ctx, key, token, ttl).Result()
	if err != nil {
		return "", false, err
	}
	return token, ok, nil
}

func (s *Service) releaseLock(ctx context.Context, key, token string) {
	if s.redis == nil || token == "" {
		return
	}
	const script = `if redis.call("GET", KEYS[1]) == ARGV[1] then return redis.call("DEL", KEYS[1]) else return 0 end`
	_ = s.redis.Eval(ctx, script, []string{key}, token).Err()
}

func refreshLockKey(userID string) string {
	return "spotify:refresh:lock:" + userID
}

func (s *Service) acquireSpotifyRequestBudget(ctx context.Context, userID string) bool {
	if s.redis == nil {
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
	res, err := s.redis.Eval(ctx, script, []string{globalKey, userKey}, spotifyGlobalRequestsPerSecond, spotifyUserRequestsPerSecond, int(spotifyRateWindow.Seconds())+1).Int64()
	if err != nil {
		log.Printf("warning: spotify refresh rate limiter unavailable: %v", err)
		return true
	}
	return res == 1
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
