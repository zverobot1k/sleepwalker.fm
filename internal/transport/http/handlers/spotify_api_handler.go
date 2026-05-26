package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"sleepwalker.fm/internal/domain"
	"sleepwalker.fm/internal/repository/postgres"
)

// TopArtistsResponse — структура ответа Spotify для топ артистов
type TopArtistsResponse struct {
	Items []ArtistItem `json:"items"`
	Total int          `json:"total"`
	Limit int          `json:"limit"`
}

type ArtistItem struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Popularity   int               `json:"popularity"`
	Genres       []string          `json:"genres"` // может быть null в JSON → пустой слайс
	Images       []Image           `json:"images"`
	ExternalURLs map[string]string `json:"external_urls"` // более гибкая структура
}

type Image struct {
	Height int    `json:"height"`
	Width  int    `json:"width"`
	URL    string `json:"url"`
}

type TopTracksResponse struct {
	Items []TrackItem `json:"items"`
	Total int         `json:"total"`
	Limit int         `json:"limit"`
}

type TrackItem struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	URI          string            `json:"uri"`
	Popularity   int               `json:"popularity"`
	DurationMS   int               `json:"duration_ms"`
	Artists      []SimpleArtist    `json:"artists"`
	Album        SimpleAlbum       `json:"album"`
	ExternalURLs map[string]string `json:"external_urls"`
}

type SimpleArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SimpleAlbum struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Images      []Image `json:"images"`
	ReleaseDate string  `json:"release_date"`
}

type RecentlyPlayedResponse struct {
	Items   []PlayHistoryItem `json:"items"`
	Cursors struct {
		After  string `json:"after"`
		Before string `json:"before"`
	} `json:"cursors"`
	Next  string `json:"next"`
	Limit int    `json:"limit"`
}

type PlayHistoryItem struct {
	Track    TrackItem `json:"track"`
	PlayedAt string    `json:"played_at"`
	Context  struct {
		Type string `json:"type"`
		URI  string `json:"uri"`
	} `json:"context"`
}

type AudioFeaturesResponse struct {
	AudioFeatures []AudioFeatureItem `json:"audio_features"`
}

type AudioFeatureItem struct {
	ID               string  `json:"id"`
	Danceability     float64 `json:"danceability"`
	Energy           float64 `json:"energy"`
	Valence          float64 `json:"valence"`
	Tempo            float64 `json:"tempo"`
	Acousticness     float64 `json:"acousticness"`
	Instrumentalness float64 `json:"instrumentalness"`
	Liveness         float64 `json:"liveness"`
	Speechiness      float64 `json:"speechiness"`
	Loudness         float64 `json:"loudness"`
	DurationMS       int     `json:"duration_ms"`
}

type WrappedSummaryResponse struct {
	TimeRange            string       `json:"time_range"`
	TopArtists           []ArtistItem `json:"top_artists"`
	TopTracks            []TrackItem  `json:"top_tracks"`
	TopGenres            []GenreCount `json:"top_genres"`
	RecentPlaysCount     int          `json:"recent_plays_count"`
	RecentMinutesTotal   int          `json:"recent_minutes_total"`
	UniqueTracksRecent   int          `json:"unique_tracks_recent"`
	UniqueArtistsRecent  int          `json:"unique_artists_recent"`
	AudioFeaturesWarning string       `json:"audio_features_warning,omitempty"`
}

type WrappedInsightsResponse struct {
	TimeRange   string   `json:"time_range"`
	Highlights  []string `json:"highlights"`
	TopGenre    string   `json:"top_genre"`
	TopArtist   string   `json:"top_artist"`
	TopTrack    string   `json:"top_track"`
	ListenerTag string   `json:"listener_tag"`
}

type TimelinePoint struct {
	Key   string `json:"key"`
	Plays int    `json:"plays"`
}

type WrappedTimelineResponse struct {
	ByDay  []TimelinePoint `json:"by_day"`
	ByHour []TimelinePoint `json:"by_hour"`
}

type WrappedCompareResponse struct {
	LeftRange        string `json:"left_range"`
	RightRange       string `json:"right_range"`
	TrackOverlap     int    `json:"track_overlap"`
	ArtistOverlap    int    `json:"artist_overlap"`
	NewTracksInLeft  int    `json:"new_tracks_in_left"`
	NewArtistsInLeft int    `json:"new_artists_in_left"`
	LeftTopTrack     string `json:"left_top_track"`
	RightTopTrack    string `json:"right_top_track"`
	LeftTopArtist    string `json:"left_top_artist"`
	RightTopArtist   string `json:"right_top_artist"`
}

type GenreCount struct {
	Genre string `json:"genre"`
	Count int    `json:"count"`
}

type StatsProfileResponse struct {
	TimeRange          string  `json:"time_range"`
	AvgTrackPopularity float64 `json:"avg_track_popularity"`
	AvgTrackDurationMS float64 `json:"avg_track_duration_ms"`
	AvgDanceability    float64 `json:"avg_danceability"`
	AvgEnergy          float64 `json:"avg_energy"`
	AvgValence         float64 `json:"avg_valence"`
	Warning            string  `json:"warning,omitempty"`
}

type RecommendationItem struct {
	Track  TrackItem `json:"track"`
	Reason string    `json:"reason"`
}

type RecommendationsResponse struct {
	Mode          string               `json:"mode"`
	Items         []RecommendationItem `json:"items"`
	Source        string               `json:"source"`
	Warning       string               `json:"warning,omitempty"`
	SeedTrackIDs  []string             `json:"seed_track_ids"`
	SeedArtistIDs []string             `json:"seed_artist_ids"`
}

type CreatePlaylistResponse struct {
	PlaylistID   string   `json:"playlist_id,omitempty"`
	PlaylistURL  string   `json:"playlist_url,omitempty"`
	TracksAdded  int      `json:"tracks_added"`
	Warning      string   `json:"warning,omitempty"`
	FallbackURIs []string `json:"fallback_uris,omitempty"`
}

// SpotifyAPIHandler — обработчик для вызовов Spotify API
type SpotifyAPIHandler struct {
	tokenRepo           *postgres.TokenRepo
	artistCache         *artistCache
	relatedArtistsCache *relatedArtistsCache
	appTokenCache       *appTokenCache
}

const (
	defaultTimeRange = "medium_term"
	defaultLimit     = 20
	minLimit         = 1
	maxLimit         = 50
	enrichWorkers    = 5
)

func NewSpotifyAPIHandler(tokenRepo *postgres.TokenRepo) *SpotifyAPIHandler {
	return &SpotifyAPIHandler{
		tokenRepo:           tokenRepo,
		artistCache:         newArtistCache(30 * time.Minute),
		relatedArtistsCache: newRelatedArtistsCache(30 * time.Minute),
		appTokenCache:       newAppTokenCache(),
	}
}

// GetTopArtists — получить топ артистов пользователя
func (h *SpotifyAPIHandler) GetTopArtists(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}

	timeRange := c.DefaultQuery("time_range", defaultTimeRange)
	if !isValidTimeRange(timeRange) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid time_range: use short_term, medium_term or long_term"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Получи токен из БД
	tokens, err := h.tokenRepo.GetByUserID(context.Background(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user tokens not found"})
		return
	}

	// Проверь, что токен ещё валиден (опционально — Spotify сам скажет, если истёк)
	if tokens.AccessToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "no access token"})
		return
	}

	// Вызови Spotify API
	topArtists, err := h.fetchTopArtistsFromSpotify(tokens.AccessToken, timeRange, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, topArtists)
}

// GetTopTracks — получить топ треков пользователя
func (h *SpotifyAPIHandler) GetTopTracks(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}

	timeRange := c.DefaultQuery("time_range", defaultTimeRange)
	if !isValidTimeRange(timeRange) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid time_range: use short_term, medium_term or long_term"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	accessToken, err := h.getUserAccessToken(userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	result, err := h.fetchTopTracksFromSpotify(accessToken, timeRange, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetRecentlyPlayed — получить последние прослушанные треки пользователя
func (h *SpotifyAPIHandler) GetRecentlyPlayed(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	before := c.Query("before")
	after := c.Query("after")
	if before != "" {
		if _, err := strconv.ParseInt(before, 10, 64); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid before: must be unix timestamp in milliseconds"})
			return
		}
	}
	if after != "" {
		if _, err := strconv.ParseInt(after, 10, 64); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid after: must be unix timestamp in milliseconds"})
			return
		}
	}

	accessToken, err := h.getUserAccessToken(userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	result, err := h.fetchRecentlyPlayedFromSpotify(accessToken, limit, before, after)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetAudioFeatures — получить аудио-фичи по ids или по топ трекам пользователя
func (h *SpotifyAPIHandler) GetAudioFeatures(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}

	accessToken, err := h.getUserAccessToken(userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	idsRaw := strings.TrimSpace(c.Query("ids"))
	var trackIDs []string

	if idsRaw != "" {
		trackIDs = splitAndTrimCSV(idsRaw)
		if len(trackIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ids: provide comma-separated spotify track ids"})
			return
		}
		if len(trackIDs) > 100 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ids: maximum 100 track ids"})
			return
		}
	} else {
		timeRange := c.DefaultQuery("time_range", defaultTimeRange)
		if !isValidTimeRange(timeRange) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid time_range: use short_term, medium_term or long_term"})
			return
		}

		limit, err := parseLimit(c.Query("limit"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		topTracks, err := h.fetchTopTracksFromSpotify(accessToken, timeRange, limit)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		trackIDs = collectTrackIDs(topTracks.Items)
	}

	result, err := h.fetchAudioFeaturesFromSpotify(accessToken, trackIDs)
	if err != nil {
		if isSpotifyForbidden(err) {
			c.JSON(http.StatusOK, gin.H{
				"audio_features": []AudioFeatureItem{},
				"warning":        "spotify returned 403 for audio-features endpoint in current account/app context",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetWrappedSummary — агрегированная сводка для Wrapped
func (h *SpotifyAPIHandler) GetWrappedSummary(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}

	timeRange := c.DefaultQuery("time_range", defaultTimeRange)
	if !isValidTimeRange(timeRange) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid time_range: use short_term, medium_term or long_term"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	accessToken, err := h.getUserAccessToken(userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	topArtists, err := h.fetchTopArtistsFromSpotify(accessToken, timeRange, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	topTracks, err := h.fetchTopTracksFromSpotify(accessToken, timeRange, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	recentlyPlayed, err := h.fetchRecentlyPlayedFromSpotify(accessToken, 50, "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	minutes, uniqueTracks, uniqueArtists := calculateRecentListeningStats(recentlyPlayed.Items)
	resp := WrappedSummaryResponse{
		TimeRange:           timeRange,
		TopArtists:          topArtists.Items,
		TopTracks:           topTracks.Items,
		TopGenres:           topGenresFromArtists(topArtists.Items, 10),
		RecentPlaysCount:    len(recentlyPlayed.Items),
		RecentMinutesTotal:  minutes,
		UniqueTracksRecent:  uniqueTracks,
		UniqueArtistsRecent: uniqueArtists,
	}

	c.JSON(http.StatusOK, resp)
}

// GetWrappedInsights — текстовые инсайты на базе Wrapped summary
func (h *SpotifyAPIHandler) GetWrappedInsights(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}
	timeRange := c.DefaultQuery("time_range", defaultTimeRange)
	if !isValidTimeRange(timeRange) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid time_range: use short_term, medium_term or long_term"})
		return
	}

	accessToken, err := h.getUserAccessToken(userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	topArtists, err := h.fetchTopArtistsFromSpotify(accessToken, timeRange, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	topTracks, err := h.fetchTopTracksFromSpotify(accessToken, timeRange, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	recentlyPlayed, err := h.fetchRecentlyPlayedFromSpotify(accessToken, 50, "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	genres := topGenresFromArtists(topArtists.Items, 1)
	topGenre := "unknown"
	if len(genres) > 0 {
		topGenre = genres[0].Genre
	}
	topArtist := "unknown"
	if len(topArtists.Items) > 0 {
		topArtist = topArtists.Items[0].Name
	}
	topTrack := "unknown"
	if len(topTracks.Items) > 0 {
		topTrack = topTracks.Items[0].Name
	}

	_, _, uniqueArtists := calculateRecentListeningStats(recentlyPlayed.Items)
	listenerTag := "focused listener"
	if uniqueArtists >= 20 {
		listenerTag = "explorer"
	}

	highlights := []string{
		"Top genre: " + topGenre,
		"Top artist: " + topArtist,
		"Top track: " + topTrack,
		"Unique artists in recent history: " + strconv.Itoa(uniqueArtists),
	}

	c.JSON(http.StatusOK, WrappedInsightsResponse{
		TimeRange:   timeRange,
		Highlights:  highlights,
		TopGenre:    topGenre,
		TopArtist:   topArtist,
		TopTrack:    topTrack,
		ListenerTag: listenerTag,
	})
}

// GetWrappedTimeline — таймлайн прослушиваний
func (h *SpotifyAPIHandler) GetWrappedTimeline(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}

	accessToken, err := h.getUserAccessToken(userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	recentlyPlayed, err := h.fetchRecentlyPlayedFromSpotify(accessToken, 50, "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	dayMap := map[string]int{}
	hourMap := map[string]int{}
	for _, item := range recentlyPlayed.Items {
		t, err := time.Parse(time.RFC3339, item.PlayedAt)
		if err != nil {
			continue
		}
		dayKey := t.Format("2006-01-02")
		hourKey := fmt.Sprintf("%02d:00", t.Hour())
		dayMap[dayKey]++
		hourMap[hourKey]++
	}

	c.JSON(http.StatusOK, WrappedTimelineResponse{
		ByDay:  mapToSortedPoints(dayMap),
		ByHour: mapToSortedPoints(hourMap),
	})
}

// GetWrappedCompare — сравнение двух периодов
func (h *SpotifyAPIHandler) GetWrappedCompare(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}

	left := c.DefaultQuery("left", "short_term")
	right := c.DefaultQuery("right", "long_term")
	if !isValidTimeRange(left) || !isValidTimeRange(right) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid left/right range: use short_term, medium_term or long_term"})
		return
	}

	accessToken, err := h.getUserAccessToken(userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	leftTracks, err := h.fetchTopTracksFromSpotify(accessToken, left, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rightTracks, err := h.fetchTopTracksFromSpotify(accessToken, right, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	leftArtists, err := h.fetchTopArtistsFromSpotify(accessToken, left, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rightArtists, err := h.fetchTopArtistsFromSpotify(accessToken, right, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	leftTrackSet := trackIDSet(leftTracks.Items)
	rightTrackSet := trackIDSet(rightTracks.Items)
	leftArtistSet := artistIDSet(leftArtists.Items)
	rightArtistSet := artistIDSet(rightArtists.Items)

	resp := WrappedCompareResponse{
		LeftRange:        left,
		RightRange:       right,
		TrackOverlap:     setOverlap(leftTrackSet, rightTrackSet),
		ArtistOverlap:    setOverlap(leftArtistSet, rightArtistSet),
		NewTracksInLeft:  setDifferenceCount(leftTrackSet, rightTrackSet),
		NewArtistsInLeft: setDifferenceCount(leftArtistSet, rightArtistSet),
	}
	if len(leftTracks.Items) > 0 {
		resp.LeftTopTrack = leftTracks.Items[0].Name
	}
	if len(rightTracks.Items) > 0 {
		resp.RightTopTrack = rightTracks.Items[0].Name
	}
	if len(leftArtists.Items) > 0 {
		resp.LeftTopArtist = leftArtists.Items[0].Name
	}
	if len(rightArtists.Items) > 0 {
		resp.RightTopArtist = rightArtists.Items[0].Name
	}

	c.JSON(http.StatusOK, resp)
}

// GetStatsProfile — профиль пользователя по метрикам треков/фич
func (h *SpotifyAPIHandler) GetStatsProfile(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}
	timeRange := c.DefaultQuery("time_range", defaultTimeRange)
	if !isValidTimeRange(timeRange) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid time_range: use short_term, medium_term or long_term"})
		return
	}

	accessToken, err := h.getUserAccessToken(userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	topTracks, err := h.fetchTopTracksFromSpotify(accessToken, timeRange, 20)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	avgPopularity := 0.0
	avgDuration := 0.0
	if len(topTracks.Items) > 0 {
		var totalPopularity, totalDuration int
		for _, t := range topTracks.Items {
			totalPopularity += t.Popularity
			totalDuration += t.DurationMS
		}
		avgPopularity = float64(totalPopularity) / float64(len(topTracks.Items))
		avgDuration = float64(totalDuration) / float64(len(topTracks.Items))
	}

	resp := StatsProfileResponse{
		TimeRange:          timeRange,
		AvgTrackPopularity: avgPopularity,
		AvgTrackDurationMS: avgDuration,
	}

	features, err := h.fetchAudioFeaturesFromSpotify(accessToken, collectTrackIDs(topTracks.Items))
	if err != nil {
		if isSpotifyForbidden(err) {
			resp.Warning = "audio-features unavailable: spotify returned 403"
			c.JSON(http.StatusOK, resp)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(features.AudioFeatures) > 0 {
		var d, e, v float64
		for _, f := range features.AudioFeatures {
			d += f.Danceability
			e += f.Energy
			v += f.Valence
		}
		resp.AvgDanceability = d / float64(len(features.AudioFeatures))
		resp.AvgEnergy = e / float64(len(features.AudioFeatures))
		resp.AvgValence = v / float64(len(features.AudioFeatures))
	}

	c.JSON(http.StatusOK, resp)
}

// GetStatsGenres — жанровая статистика
func (h *SpotifyAPIHandler) GetStatsGenres(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}
	timeRange := c.DefaultQuery("time_range", defaultTimeRange)
	if !isValidTimeRange(timeRange) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid time_range: use short_term, medium_term or long_term"})
		return
	}

	accessToken, err := h.getUserAccessToken(userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	topArtists, err := h.fetchTopArtistsFromSpotify(accessToken, timeRange, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	genres := topGenresFromArtists(topArtists.Items, 20)
	warning := ""
	if len(genres) == 0 {
		inferred, inferWarnings := h.buildSeedGenres(accessToken, collectArtistIDs(topArtists.Items), 8)
		if len(inferred) > 0 {
			genres = make([]GenreCount, 0, len(inferred))
			for _, genre := range inferred {
				genres = append(genres, GenreCount{Genre: genre, Count: 1})
			}
			warning = "genres inferred from related artists"
			if extra := compactWarnings(inferWarnings); extra != "" {
				warning = warning + ": " + extra
			}
		}
	}

	resp := gin.H{
		"time_range": timeRange,
		"genres":     genres,
	}
	if warning != "" {
		resp["warning"] = warning
	}

	c.JSON(http.StatusOK, resp)
}

// GetStatsListeningTime — статистика времени прослушивания по recently played
func (h *SpotifyAPIHandler) GetStatsListeningTime(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}

	accessToken, err := h.getUserAccessToken(userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	recentlyPlayed, err := h.fetchRecentlyPlayedFromSpotify(accessToken, 50, "", "")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	fromStr := c.Query("from")
	toStr := c.Query("to")
	var from, to time.Time
	var hasFrom, hasTo bool
	if fromStr != "" {
		parsed, err := time.Parse(time.RFC3339, fromStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid from: use RFC3339 format"})
			return
		}
		from = parsed
		hasFrom = true
	}
	if toStr != "" {
		parsed, err := time.Parse(time.RFC3339, toStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid to: use RFC3339 format"})
			return
		}
		to = parsed
		hasTo = true
	}

	totalDuration := 0
	plays := 0
	for _, item := range recentlyPlayed.Items {
		playedAt, err := time.Parse(time.RFC3339, item.PlayedAt)
		if err != nil {
			continue
		}
		if hasFrom && playedAt.Before(from) {
			continue
		}
		if hasTo && playedAt.After(to) {
			continue
		}
		totalDuration += item.Track.DurationMS
		plays++
	}

	c.JSON(http.StatusOK, gin.H{
		"plays":         plays,
		"duration_ms":   totalDuration,
		"minutes":       totalDuration / 60000,
		"hours":         float64(totalDuration) / 3600000.0,
		"source_window": "recently_played_last_50",
	})
}

// GetRecommendations — рекомендации на основе top треков/артистов
func (h *SpotifyAPIHandler) GetRecommendations(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}

	mode := c.DefaultQuery("mode", "comfort")
	if mode != "comfort" && mode != "explore" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid mode: use comfort or explore"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	accessToken, err := h.getUserAccessToken(userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	recs, err := h.fetchRecommendationsFromSpotify(accessToken, mode, limit)
	if err != nil {
		fallback, fbErr := h.fallbackRecommendations(accessToken, limit)
		if fbErr != nil {
			warning := "spotify recommendations unavailable: " + err.Error()
			warning += "; fallback failed: " + fbErr.Error()
			c.JSON(http.StatusOK, RecommendationsResponse{
				Mode:    mode,
				Items:   []RecommendationItem{},
				Source:  "recommendations_unavailable",
				Warning: warning,
			})
			return
		}
		fallback.Warning = "spotify recommendations endpoint unavailable, fallback used: " + err.Error()
		c.JSON(http.StatusOK, fallback)
		return
	}

	c.JSON(http.StatusOK, recs)
}

// CreateRecommendationsPlaylist — создать плейлист из рекомендаций
func (h *SpotifyAPIHandler) CreateRecommendationsPlaylist(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}

	limit, err := parseLimit(c.DefaultQuery("limit", "20"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	accessToken, err := h.getUserAccessToken(userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	recs, err := h.fetchRecommendationsFromSpotify(accessToken, "comfort", limit)
	if err != nil {
		fallback, fbErr := h.fallbackRecommendations(accessToken, limit)
		if fbErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		uris := recommendationURIs(fallback.Items)
		c.JSON(http.StatusOK, CreatePlaylistResponse{
			TracksAdded:  0,
			Warning:      "playlist not created, spotify recommendations unavailable",
			FallbackURIs: uris,
		})
		return
	}

	playlistID, playlistURL, err := h.createPlaylistWithTracks(accessToken, userID, recommendationURIs(recs.Items))
	if err != nil {
		if isSpotifyForbidden(err) {
			c.JSON(http.StatusOK, CreatePlaylistResponse{
				TracksAdded:  0,
				Warning:      "playlist create/add forbidden for current scopes",
				FallbackURIs: recommendationURIs(recs.Items),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, CreatePlaylistResponse{
		PlaylistID:  playlistID,
		PlaylistURL: playlistURL,
		TracksAdded: len(recommendationURIs(recs.Items)),
	})
}

// fetchTopArtistsFromSpotify — вызов Spotify API
func (h *SpotifyAPIHandler) fetchTopArtistsFromSpotify(accessToken string, timeRange string, limit int) (*TopArtistsResponse, error) {
	// Параметры запроса
	url := "https://api.spotify.com/v1/me/top/artists" +
		"?time_range=" + timeRange +
		"&limit=" + strconv.Itoa(limit)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Добавь авторизацию
	req.Header.Add("Authorization", "Bearer "+accessToken)

	// Выполни запрос
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Проверь статус
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, domain.NewError(resp.StatusCode, "spotify api error: "+string(body))
	}

	// Прочитай всё тело ответа для логирования и парсинга
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Парси ответ
	var result TopArtistsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		// Логирование ошибки парсинга с примером тела ответа
		prefix := string(body)
		if len(prefix) > 500 {
			prefix = prefix[:500]
		}
		log.Printf("❌ JSON parse error: %v\nFirst chars of response: %s\n", err, prefix)
		return nil, err
	}

	// Обогащение данных по артистам (genres/popularity и др.) через /artists batch с кэшем.
	artistIDs := collectArtistIDs(result.Items)
	if len(artistIDs) > 0 {
		detailsByID, err := h.getArtistDetailsCached(accessToken, artistIDs)
		if err != nil {
			if isSpotifyRateLimited(err) {
				log.Printf("⚠️ artist enrichment skipped due to rate limit: %v", err)
			} else if isSpotifyForbidden(err) {
				log.Printf("⚠️ artist enrichment skipped: spotify returned 403")
			} else {
				// Частичный fallback: используем всё, что успели получить, и не ломаем ручку.
				log.Printf("⚠️ artist enrichment partially skipped: %v", err)
			}
		} else {
			log.Printf("✅ artist enrichment completed for %d artists", len(detailsByID))
		}
		mergeArtistDetails(result.Items, detailsByID)
	}

	// Нормализуй данные: замени null-значения на пустые значения
	for i := range result.Items {
		// Если genres nil, замени на пустой слайс для консистентности
		if result.Items[i].Genres == nil {
			result.Items[i].Genres = []string{}
		}
		// Если external_urls nil или пусто, замени на пустой map
		if result.Items[i].ExternalURLs == nil {
			result.Items[i].ExternalURLs = make(map[string]string)
		}
	}

	// Логирование для отладки
	log.Printf("✅ Получено топ артистов: %d штук\n", len(result.Items))
	if len(result.Items) > 0 {
		log.Printf("  Первый артист: %s (popularity: %d, genres: %v)\n",
			result.Items[0].Name, result.Items[0].Popularity, result.Items[0].Genres)
	}

	// Логирование raw JSON первого артиста для отладки
	if len(result.Items) > 0 {
		rawJSON, _ := json.MarshalIndent(result.Items[0], "", "  ")
		log.Printf("📋 Raw JSON первого артиста:\n%s\n", string(rawJSON))
	}

	return &result, nil
}

// fetchTopArtistsLite fetches top artists without extra enrichment calls.
// This is used for recommendations to reduce API load and rate-limit risk.
func (h *SpotifyAPIHandler) fetchTopArtistsLite(accessToken string, timeRange string, limit int) (*TopArtistsResponse, error) {
	url := "https://api.spotify.com/v1/me/top/artists" +
		"?time_range=" + timeRange +
		"&limit=" + strconv.Itoa(limit)

	body, err := h.doSpotifyGET(accessToken, url)
	if err != nil {
		return nil, err
	}

	var result TopArtistsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	for i := range result.Items {
		if result.Items[i].Genres == nil {
			result.Items[i].Genres = []string{}
		}
		if result.Items[i].ExternalURLs == nil {
			result.Items[i].ExternalURLs = make(map[string]string)
		}
	}

	return &result, nil
}

func (h *SpotifyAPIHandler) fetchTopTracksFromSpotify(accessToken string, timeRange string, limit int) (*TopTracksResponse, error) {
	url := "https://api.spotify.com/v1/me/top/tracks" +
		"?time_range=" + timeRange +
		"&limit=" + strconv.Itoa(limit)

	body, err := h.doSpotifyGET(accessToken, url)
	if err != nil {
		return nil, err
	}

	var result TopTracksResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	for i := range result.Items {
		if result.Items[i].ExternalURLs == nil {
			result.Items[i].ExternalURLs = make(map[string]string)
		}
	}

	return &result, nil
}

func (h *SpotifyAPIHandler) fetchRecentlyPlayedFromSpotify(accessToken string, limit int, before string, after string) (*RecentlyPlayedResponse, error) {
	url := "https://api.spotify.com/v1/me/player/recently-played?limit=" + strconv.Itoa(limit)
	if before != "" {
		url += "&before=" + before
	}
	if after != "" {
		url += "&after=" + after
	}

	body, err := h.doSpotifyGET(accessToken, url)
	if err != nil {
		return nil, err
	}

	var result RecentlyPlayedResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	for i := range result.Items {
		if result.Items[i].Track.ExternalURLs == nil {
			result.Items[i].Track.ExternalURLs = make(map[string]string)
		}
	}

	return &result, nil
}

func (h *SpotifyAPIHandler) fetchAudioFeaturesFromSpotify(accessToken string, trackIDs []string) (*AudioFeaturesResponse, error) {
	if len(trackIDs) == 0 {
		return &AudioFeaturesResponse{AudioFeatures: []AudioFeatureItem{}}, nil
	}

	const maxIDsPerRequest = 100
	aggregated := make([]AudioFeatureItem, 0, len(trackIDs))

	for start := 0; start < len(trackIDs); start += maxIDsPerRequest {
		end := start + maxIDsPerRequest
		if end > len(trackIDs) {
			end = len(trackIDs)
		}

		chunk := strings.Join(trackIDs[start:end], ",")
		url := "https://api.spotify.com/v1/audio-features?ids=" + chunk
		body, err := h.doSpotifyGETWithAppFallback(accessToken, url)
		if err != nil {
			return nil, err
		}

		var partial AudioFeaturesResponse
		if err := json.Unmarshal(body, &partial); err != nil {
			return nil, err
		}
		aggregated = append(aggregated, partial.AudioFeatures...)
	}

	return &AudioFeaturesResponse{AudioFeatures: aggregated}, nil
}

func (h *SpotifyAPIHandler) doSpotifyGET(accessToken string, url string) ([]byte, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Add("Authorization", "Bearer "+accessToken)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, domain.NewError(resp.StatusCode, "spotify api error: "+string(body))
	}

	return body, nil
}

func (h *SpotifyAPIHandler) getUserAccessToken(userID string) (string, error) {
	tokens, err := h.tokenRepo.GetByUserID(context.Background(), userID)
	if err != nil {
		return "", fmt.Errorf("user tokens not found")
	}
	if tokens.AccessToken == "" {
		return "", fmt.Errorf("no access token")
	}
	return tokens.AccessToken, nil
}

func (h *SpotifyAPIHandler) writeTokenError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	if err.Error() == "user tokens not found" {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	if err.Error() == "no access token" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func isSpotifyForbidden(err error) bool {
	apiErr, ok := err.(domain.APIError)
	if !ok {
		return false
	}
	return apiErr.StatusCode == http.StatusForbidden
}

func isSpotifyRateLimited(err error) bool {
	apiErr, ok := err.(domain.APIError)
	if !ok {
		return false
	}
	return apiErr.StatusCode == http.StatusTooManyRequests
}

func isValidTimeRange(value string) bool {
	return value == "short_term" || value == "medium_term" || value == "long_term"
}

func parseLimit(raw string) (int, error) {
	if raw == "" {
		return defaultLimit, nil
	}

	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid limit: must be an integer from %d to %d", minLimit, maxLimit)
	}
	if v < minLimit || v > maxLimit {
		return 0, fmt.Errorf("invalid limit: must be from %d to %d", minLimit, maxLimit)
	}

	return v, nil
}

func (h *SpotifyAPIHandler) fetchArtistsDetailsIndividually(client *http.Client, accessToken string, artistIDs []string) (map[string]ArtistItem, error) {
	result := make(map[string]ArtistItem, len(artistIDs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, enrichWorkers)
	errCh := make(chan error, len(artistIDs))

	for _, artistID := range artistIDs {
		artistID := artistID
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			artist, err := h.fetchArtistByID(client, accessToken, artistID)
			if err != nil {
				errCh <- err
				return
			}

			mu.Lock()
			result[artistID] = artist
			mu.Unlock()
		}()
	}

	wg.Wait()
	close(errCh)

	var firstErr error
	for err := range errCh {
		if firstErr == nil {
			firstErr = err
		}
	}

	return result, firstErr
}

func (h *SpotifyAPIHandler) fetchArtistByID(client *http.Client, accessToken string, artistID string) (ArtistItem, error) {
	url := "https://api.spotify.com/v1/artists/" + artistID
	if appToken, err := h.getAppAccessToken(); err == nil {
		if body, appErr := h.doSpotifyGET(appToken, url); appErr == nil {
			var artist ArtistItem
			if err := json.Unmarshal(body, &artist); err == nil {
				if artist.Genres == nil {
					artist.Genres = []string{}
				}
				if artist.ExternalURLs == nil {
					artist.ExternalURLs = make(map[string]string)
				}
				return artist, nil
			}
		}
	}

	body, err := h.doSpotifyGET(accessToken, url)
	if err != nil {
		return ArtistItem{}, err
	}

	var artist ArtistItem
	if err := json.Unmarshal(body, &artist); err != nil {
		return ArtistItem{}, err
	}
	if artist.Genres == nil {
		artist.Genres = []string{}
	}
	if artist.ExternalURLs == nil {
		artist.ExternalURLs = make(map[string]string)
	}

	return artist, nil
}

func collectArtistIDs(items []ArtistItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		ids = append(ids, item.ID)
	}
	return ids
}

func collectTrackIDs(items []TrackItem) []string {
	ids := make([]string, 0, len(items))
	for _, item := range items {
		if item.ID == "" {
			continue
		}
		ids = append(ids, item.ID)
	}
	return ids
}

func splitAndTrimCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v == "" {
			continue
		}
		result = append(result, v)
	}
	return result
}

func mergeArtistDetails(items []ArtistItem, detailsByID map[string]ArtistItem) {
	for i := range items {
		details, ok := detailsByID[items[i].ID]
		if !ok {
			continue
		}

		if details.Name != "" {
			items[i].Name = details.Name
		}
		items[i].Popularity = details.Popularity

		if details.Genres == nil {
			items[i].Genres = []string{}
		} else {
			items[i].Genres = details.Genres
		}

		if len(details.Images) > 0 {
			items[i].Images = details.Images
		}

		if details.ExternalURLs == nil {
			items[i].ExternalURLs = make(map[string]string)
		} else {
			items[i].ExternalURLs = details.ExternalURLs
		}
	}
}

func calculateRecentListeningStats(items []PlayHistoryItem) (minutes int, uniqueTracks int, uniqueArtists int) {
	totalDuration := 0
	trackSet := map[string]struct{}{}
	artistSet := map[string]struct{}{}

	for _, item := range items {
		totalDuration += item.Track.DurationMS
		if item.Track.ID != "" {
			trackSet[item.Track.ID] = struct{}{}
		}
		for _, a := range item.Track.Artists {
			if a.ID != "" {
				artistSet[a.ID] = struct{}{}
			}
		}
	}

	return totalDuration / 60000, len(trackSet), len(artistSet)
}

func topGenresFromArtists(items []ArtistItem, topN int) []GenreCount {
	genreMap := map[string]int{}
	for _, a := range items {
		for _, g := range a.Genres {
			if g == "" {
				continue
			}
			genreMap[g]++
		}
	}

	genres := make([]GenreCount, 0, len(genreMap))
	for k, v := range genreMap {
		genres = append(genres, GenreCount{Genre: k, Count: v})
	}

	sort.Slice(genres, func(i, j int) bool {
		if genres[i].Count == genres[j].Count {
			return genres[i].Genre < genres[j].Genre
		}
		return genres[i].Count > genres[j].Count
	})

	if topN > 0 && len(genres) > topN {
		genres = genres[:topN]
	}

	return genres
}

func mapToSortedPoints(values map[string]int) []TimelinePoint {
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	points := make([]TimelinePoint, 0, len(keys))
	for _, k := range keys {
		points = append(points, TimelinePoint{Key: k, Plays: values[k]})
	}
	return points
}

func trackIDSet(items []TrackItem) map[string]struct{} {
	set := map[string]struct{}{}
	for _, it := range items {
		if it.ID != "" {
			set[it.ID] = struct{}{}
		}
	}
	return set
}

func artistIDSet(items []ArtistItem) map[string]struct{} {
	set := map[string]struct{}{}
	for _, it := range items {
		if it.ID != "" {
			set[it.ID] = struct{}{}
		}
	}
	return set
}

func setOverlap(a, b map[string]struct{}) int {
	count := 0
	for k := range a {
		if _, ok := b[k]; ok {
			count++
		}
	}
	return count
}

func setDifferenceCount(a, b map[string]struct{}) int {
	count := 0
	for k := range a {
		if _, ok := b[k]; !ok {
			count++
		}
	}
	return count
}

func (h *SpotifyAPIHandler) fetchRecommendationsFromSpotify(accessToken string, mode string, limit int) (*RecommendationsResponse, error) {
	warnings := []string{}
	inputs := affinityInputs{}

	if topTracks, err := h.fetchTopTracksFromSpotify(accessToken, "short_term", 20); err == nil {
		inputs.topTracksShort = topTracks.Items
	} else {
		warnings = append(warnings, "top tracks short_term unavailable")
	}
	if topTracks, err := h.fetchTopTracksFromSpotify(accessToken, "medium_term", 20); err == nil {
		inputs.topTracksMedium = topTracks.Items
	} else {
		warnings = append(warnings, "top tracks medium_term unavailable")
	}
	if topTracks, err := h.fetchTopTracksFromSpotify(accessToken, "long_term", 20); err == nil {
		inputs.topTracksLong = topTracks.Items
	} else {
		warnings = append(warnings, "top tracks long_term unavailable")
	}

	if topArtists, err := h.fetchTopArtistsLite(accessToken, "short_term", 20); err == nil {
		inputs.topArtistsShort = topArtists.Items
	} else {
		warnings = append(warnings, "top artists short_term unavailable")
	}
	if topArtists, err := h.fetchTopArtistsLite(accessToken, "medium_term", 20); err == nil {
		inputs.topArtistsMedium = topArtists.Items
	} else {
		warnings = append(warnings, "top artists medium_term unavailable")
	}
	if topArtists, err := h.fetchTopArtistsLite(accessToken, "long_term", 20); err == nil {
		inputs.topArtistsLong = topArtists.Items
	} else {
		warnings = append(warnings, "top artists long_term unavailable")
	}

	if recent, err := h.fetchRecentlyPlayedFromSpotify(accessToken, 50, "", ""); err == nil {
		inputs.recentlyPlayed = recent.Items
	} else {
		warnings = append(warnings, "recently played unavailable")
	}

	if len(inputs.topTracksShort)+len(inputs.topTracksMedium)+len(inputs.topTracksLong)+len(inputs.topArtistsShort)+len(inputs.topArtistsMedium)+len(inputs.topArtistsLong)+len(inputs.recentlyPlayed) == 0 {
		return nil, fmt.Errorf("insufficient spotify signals for recommendations")
	}

	scores := computeAffinityScores(inputs)
	rankedArtists := rankArtistsByScore(scores, 12)
	seedTracks := seedTrackIDs(inputs, 2)
	seedArtists := pickFirstIDs(rankedArtists, 3)

	if len(seedArtists) < 3 {
		var relatedWarnings []string
		seedArtists, relatedWarnings = h.expandSeedArtistsWithRelated(accessToken, seedArtists, rankedArtists, 4)
		warnings = append(warnings, relatedWarnings...)
	}

	seedGenres, genreWarnings := h.buildSeedGenres(accessToken, rankedArtists, 2)
	warnings = append(warnings, genreWarnings...)

	audioProfile, audioWarning := h.buildAudioProfileFromInputs(accessToken, inputs)
	if audioWarning != "" {
		warnings = append(warnings, audioWarning)
	}

	if len(seedTracks)+len(seedArtists)+len(seedGenres) == 0 {
		return nil, fmt.Errorf("insufficient recommendation seeds")
	}

	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	if len(seedTracks) > 0 {
		query.Set("seed_tracks", strings.Join(seedTracks, ","))
	}
	if len(seedArtists) > 0 {
		query.Set("seed_artists", strings.Join(seedArtists, ","))
	}
	if len(seedGenres) > 0 && len(seedTracks)+len(seedArtists) < 5 {
		query.Set("seed_genres", strings.Join(seedGenres, ","))
	}
	for key, value := range audioProfile.targets(mode) {
		query.Set(key, value)
	}
	if !audioProfile.hasData() && mode == "explore" {
		query.Set("target_energy", "0.75")
		query.Set("target_valence", "0.65")
	}

	body, err := h.doSpotifyGET(accessToken, "https://api.spotify.com/v1/recommendations?"+query.Encode())
	if err != nil {
		return nil, err
	}

	var payload struct {
		Tracks []TrackItem `json:"tracks"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	reason := "matched your affinity profile"
	if audioProfile.hasData() {
		reason = "matched your affinity and audio profile"
	}
	if len(seedGenres) > 0 {
		reason += " with genre context"
	}

	items := make([]RecommendationItem, 0, len(payload.Tracks))
	for _, t := range payload.Tracks {
		items = append(items, RecommendationItem{Track: t, Reason: reason})
	}

	warningText := compactWarnings(warnings)

	return &RecommendationsResponse{
		Mode:          mode,
		Items:         items,
		Source:        "spotify_recommendations",
		Warning:       warningText,
		SeedTrackIDs:  seedTracks,
		SeedArtistIDs: seedArtists,
	}, nil
}

func (h *SpotifyAPIHandler) fallbackRecommendations(accessToken string, limit int) (*RecommendationsResponse, error) {
	topTracks, err := h.fetchTopTracksFromSpotify(accessToken, defaultTimeRange, limit)
	if err != nil {
		return nil, err
	}
	items := make([]RecommendationItem, 0, len(topTracks.Items))
	for _, t := range topTracks.Items {
		items = append(items, RecommendationItem{Track: t, Reason: "fallback: based on your top tracks"})
	}
	return &RecommendationsResponse{
		Mode:          "comfort",
		Items:         items,
		Source:        "top_tracks_fallback",
		SeedTrackIDs:  pickFirstIDs(collectTrackIDs(topTracks.Items), 5),
		SeedArtistIDs: []string{},
	}, nil
}

func (h *SpotifyAPIHandler) createPlaylistWithTracks(accessToken string, userID string, uris []string) (string, string, error) {
	if len(uris) == 0 {
		return "", "", nil
	}

	createPayload := map[string]interface{}{
		"name":        "Sleepwalker Wrapped Picks",
		"description": "Auto-generated from wrapped recommendations",
		"public":      false,
	}
	body, _ := json.Marshal(createPayload)
	req, err := http.NewRequest("POST", "https://api.spotify.com/v1/users/"+userID+"/playlists", strings.NewReader(string(body)))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", "", domain.NewError(resp.StatusCode, "spotify create playlist error: "+string(respBody))
	}

	var created struct {
		ID           string            `json:"id"`
		ExternalURLs map[string]string `json:"external_urls"`
	}
	if err := json.Unmarshal(respBody, &created); err != nil {
		return "", "", err
	}

	addPayload := map[string]interface{}{"uris": uris}
	addBody, _ := json.Marshal(addPayload)
	addReq, err := http.NewRequest("POST", "https://api.spotify.com/v1/playlists/"+created.ID+"/tracks", strings.NewReader(string(addBody)))
	if err != nil {
		return "", "", err
	}
	addReq.Header.Set("Authorization", "Bearer "+accessToken)
	addReq.Header.Set("Content-Type", "application/json")

	addResp, err := client.Do(addReq)
	if err != nil {
		return "", "", err
	}
	defer addResp.Body.Close()

	addRespBody, _ := io.ReadAll(addResp.Body)
	if addResp.StatusCode != http.StatusCreated && addResp.StatusCode != http.StatusOK {
		return "", "", domain.NewError(addResp.StatusCode, "spotify add tracks error: "+string(addRespBody))
	}

	return created.ID, created.ExternalURLs["spotify"], nil
}

func pickFirstIDs(ids []string, n int) []string {
	if len(ids) == 0 || n <= 0 {
		return []string{}
	}
	if len(ids) < n {
		n = len(ids)
	}
	result := make([]string, 0, n)
	for i := 0; i < n; i++ {
		if ids[i] == "" {
			continue
		}
		result = append(result, ids[i])
	}
	return result
}

func recommendationURIs(items []RecommendationItem) []string {
	uris := make([]string, 0, len(items))
	for _, item := range items {
		if item.Track.URI != "" {
			uris = append(uris, item.Track.URI)
		}
	}
	return uris
}

func recentTracksFromHistory(items []PlayHistoryItem) []TrackItem {
	tracks := make([]TrackItem, 0, len(items))
	for _, item := range items {
		tracks = append(tracks, item.Track)
	}
	return tracks
}
