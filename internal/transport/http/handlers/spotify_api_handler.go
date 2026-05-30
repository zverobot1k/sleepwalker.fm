package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"sleepwalker.fm/internal/domain"
	"sleepwalker.fm/internal/repository/postgres"
	lastfmservice "sleepwalker.fm/internal/service/lastfm"
	"sleepwalker.fm/internal/service/pipeline"
	recommendationservice "sleepwalker.fm/internal/service/recommendation"
	spotifyservice "sleepwalker.fm/internal/service/spotify"
	tokenservice "sleepwalker.fm/internal/service/token"
)

// TopArtistsResponse — структура ответа Spotify для топ артистов
type TopArtistsResponse struct {
	Items  []ArtistItem `json:"items"`
	Total  int          `json:"total"`
	Limit  int          `json:"limit"`
	Source string       `json:"source,omitempty"`
}

type ArtistItem struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Popularity   int               `json:"popularity"`
	Genres       []string          `json:"genres"` // enriched from Last.fm in stats endpoints; Spotify genres not used for analytics
	Images       []Image           `json:"images"`
	ExternalURLs map[string]string `json:"external_urls"`
	Position     int               `json:"position,omitempty"`
	Score        float64           `json:"score,omitempty"`
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
	Genre  string  `json:"genre"`
	Count  int     `json:"count"`
	Weight float64 `json:"weight,omitempty"`
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
	Notice        string               `json:"notice,omitempty"`
	Warning       string               `json:"warning,omitempty"` // deprecated: use notice
	SeedTrackIDs  []string             `json:"seed_track_ids"`
	SeedArtistIDs []string             `json:"seed_artist_ids"`
	SeedGenres    []string             `json:"seed_genres,omitempty"`
}

type ExportTrack struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Artists    []string `json:"artists"`
	URI        string   `json:"uri"`
	SpotifyURL string   `json:"spotify_url,omitempty"`
}

type CreatePlaylistResponse struct {
	PlaylistID     string        `json:"playlist_id,omitempty"`
	PlaylistURL    string        `json:"playlist_url,omitempty"`
	TracksAdded    int           `json:"tracks_added"`
	Notice         string        `json:"notice,omitempty"`
	Warning        string        `json:"warning,omitempty"` // deprecated: use notice
	FallbackURIs   []string      `json:"fallback_uris,omitempty"`
	FallbackTracks []ExportTrack `json:"fallback_tracks,omitempty"`
}

// SpotifyAPIHandler — обработчик для вызовов Spotify API
type SpotifyAPIHandler struct {
	tokenRepo           *postgres.TokenRepo
	artistCache         *artistCache
	relatedArtistsCache *relatedArtistsCache
	appTokenCache       *appTokenCache
	artistMetadataCache *artistMetadataCache
	responseCache       *redis.Client
	artistMetadataRepo  *postgres.ArtistMetadataRepo
	lastfmClient        *lastfmservice.Client
	oauthService        *spotifyservice.OAuthService
	tokenSvc            *tokenservice.Service
	spotifyAPI          *spotifyservice.APIService
	pipeline            *pipeline.Pipeline
	recSvc              *recommendationservice.Service
	lastfmSvc           *lastfmservice.Service
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

func NewSpotifyAPIHandlerWithEnrichment(tokenRepo *postgres.TokenRepo, artistMetadataRepo *postgres.ArtistMetadataRepo, lastfmClient *lastfmservice.Client, oauthService *spotifyservice.OAuthService) *SpotifyAPIHandler {
	return NewSpotifyAPIHandlerFull(tokenRepo, artistMetadataRepo, lastfmClient, oauthService, nil, nil, nil, nil, nil, nil)
}

// NewSpotifyAPIHandlerFull wires the production service layer (token, Spotify API, pipeline, recommendations).
func NewSpotifyAPIHandlerFull(
	tokenRepo *postgres.TokenRepo,
	artistMetadataRepo *postgres.ArtistMetadataRepo,
	lastfmClient *lastfmservice.Client,
	oauthService *spotifyservice.OAuthService,
	tokenSvc *tokenservice.Service,
	spotifyAPI *spotifyservice.APIService,
	pipe *pipeline.Pipeline,
	recSvc *recommendationservice.Service,
	lastfmSvc *lastfmservice.Service,
	responseCache *redis.Client,
) *SpotifyAPIHandler {
	return &SpotifyAPIHandler{
		tokenRepo:           tokenRepo,
		artistCache:         newArtistCache(30 * time.Minute),
		relatedArtistsCache: newRelatedArtistsCache(30 * time.Minute),
		appTokenCache:       newAppTokenCache(),
		artistMetadataCache: newArtistMetadataCache(24 * time.Hour),
		responseCache:       responseCache,
		artistMetadataRepo:  artistMetadataRepo,
		lastfmClient:        lastfmClient,
		oauthService:        oauthService,
		tokenSvc:            tokenSvc,
		spotifyAPI:          spotifyAPI,
		pipeline:            pipe,
		recSvc:              recSvc,
		lastfmSvc:           lastfmSvc,
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

	ctx := c.Request.Context()
	if h.pipeline == nil {
		accessToken, err := h.getUserAccessToken(ctx, userID)
		if err != nil {
			h.writeTokenError(c, err)
			return
		}
		topArtists, err := h.fetchTopArtistsFromSpotify(accessToken, timeRange, limit)
		if err != nil {
			if h.writeSpotifyError(c, err) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		h.writeCachedJSON(c, h.responseCacheKey("top_artists", userID, timeRange, strconv.Itoa(limit)), time.Hour, topArtists)
		return
	}

	if body, ok := h.cacheGet(ctx, h.responseCacheKey("top_artists", userID, timeRange, strconv.Itoa(limit))); ok {
		c.Data(http.StatusOK, "application/json", body)
		return
	}

	snapshot, err := h.pipeline.FetchSnapshot(ctx, userID, timeRange, maxInt(limit, 50))
	if err != nil {
		if isSpotifyUnauthorized(err) {
			h.writeTokenError(c, fmt.Errorf("spotify unauthorized"))
			return
		}
		if h.writeSpotifyError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ranked := h.pipeline.RankedArtists(snapshot, limit)
	merged := pipeline.MergeRankedIntoArtists(snapshot.SpotifyArtists, ranked)
	items := make([]ArtistItem, 0, len(merged))
	for _, row := range merged {
		genres := []string{}
		item := ArtistItem{
			ID:           row.Artist.ID,
			Name:         row.Artist.Name,
			Popularity:   row.Artist.Popularity,
			Genres:       genres,
			Images:       convertPipelineImages(row.Artist.Images),
			ExternalURLs: row.Artist.ExternalURLs,
			Position:     row.Position,
			Score:        row.Score,
		}
		if item.ExternalURLs == nil {
			item.ExternalURLs = map[string]string{}
		}
		items = append(items, item)
	}

	h.writeCachedJSON(c, h.responseCacheKey("top_artists", userID, timeRange, strconv.Itoa(limit)), time.Hour, TopArtistsResponse{
		Items:  items,
		Total:  len(items),
		Limit:  limit,
		Source: "computed",
	})
}

func convertPipelineImages(images []pipeline.Image) []Image {
	out := make([]Image, 0, len(images))
	for _, img := range images {
		out = append(out, Image{URL: img.URL})
	}
	return out
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
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

	ctx := c.Request.Context()
	accessToken, err := h.getUserAccessToken(ctx, userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}
	if body, ok := h.cacheGet(ctx, h.responseCacheKey("top_tracks", userID, timeRange, strconv.Itoa(limit))); ok {
		c.Data(http.StatusOK, "application/json", body)
		return
	}

	fetchLimit := maxInt(limit, 50)
	result, err := h.fetchTopTracksFromSpotify(ctx, userID, accessToken, timeRange, fetchLimit)
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": []TrackItem{}, "notice": "spotify temporarily unavailable"})
		return
	}

	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
	}
	h.writeCachedJSON(c, h.responseCacheKey("top_tracks", userID, timeRange, strconv.Itoa(limit)), time.Hour, result)
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

	ctx := c.Request.Context()
	accessToken, err := h.getUserAccessToken(ctx, userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	fetchLimit := maxInt(limit, 50)
	result, err := h.fetchRecentlyPlayedFromSpotify(ctx, userID, accessToken, fetchLimit, before, after)
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
		c.JSON(http.StatusOK, gin.H{"items": []PlayHistoryItem{}, "notice": "spotify temporarily unavailable"})
		return
	}
	if len(result.Items) > limit {
		result.Items = result.Items[:limit]
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

	accessToken, err := h.getUserAccessToken(c.Request.Context(), userID)
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

		fetchLimit := maxInt(limit, 50)
		topTracks, err := h.fetchTopTracksFromSpotify(c.Request.Context(), userID, accessToken, timeRange, fetchLimit)
		if err != nil {
			if h.writeSpotifyError(c, err) {
				return
			}
			c.JSON(http.StatusOK, gin.H{"audio_features": []AudioFeatureItem{}, "notice": "spotify temporarily unavailable", "source": "spotify_backoff"})
			return
		}
		trackIDs = collectTrackIDs(topTracks.Items)
	}

	var topTracksForProxy []TrackItem
	if idsRaw == "" {
		timeRange := c.DefaultQuery("time_range", defaultTimeRange)
		limit, _ := parseLimit(c.Query("limit"))
		if topTracks, topErr := h.fetchTopTracksFromSpotify(c.Request.Context(), userID, accessToken, timeRange, maxInt(limit, 50)); topErr == nil {
			topTracksForProxy = topTracks.Items
		}
	}

	result, err := h.fetchAudioFeaturesFromSpotify(c.Request.Context(), userID, accessToken, trackIDs)
	if err != nil {
		if isSpotifyForbidden(err) || isSpotifyNotFound(err) {
			proxyTracks := topTracksForProxy
			if len(proxyTracks) == 0 && len(trackIDs) > 0 {
				proxyTracks = make([]TrackItem, 0, len(trackIDs))
				for _, id := range trackIDs {
					proxyTracks = append(proxyTracks, TrackItem{ID: id})
				}
			}
			derived := deriveAudioFeaturesFromTracks(proxyTracks)
			log.Printf("audio-features unavailable (%v); returning %d derived feature rows", err, len(derived))
			c.JSON(http.StatusOK, gin.H{
				"audio_features": derived,
				"notice":         NoticeAudioFeaturesEstimated,
				"source":         "derived",
			})
			return
		}
		if h.writeSpotifyError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(result.AudioFeatures) == 0 && len(topTracksForProxy) > 0 {
		c.JSON(http.StatusOK, gin.H{
			"audio_features": deriveAudioFeaturesFromTracks(topTracksForProxy),
			"notice":         NoticeAudioFeaturesEstimated,
			"source":         "derived",
		})
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

	ctx := c.Request.Context()
	accessToken, err := h.getUserAccessToken(ctx, userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	var topArtistItems []ArtistItem
	var topGenres []GenreCount

	if h.pipeline != nil {
		snapshot, snapErr := h.pipeline.FetchSnapshot(ctx, userID, timeRange, maxInt(limit, 50))
		if snapErr != nil {
			if h.writeSpotifyError(c, snapErr) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": snapErr.Error()})
			return
		}
		ranked := h.pipeline.RankedArtists(snapshot, limit)
		merged := pipeline.MergeRankedIntoArtists(snapshot.SpotifyArtists, ranked)
		topArtistItems = make([]ArtistItem, 0, len(merged))
		for _, row := range merged {
			topArtistItems = append(topArtistItems, ArtistItem{
				ID:           row.Artist.ID,
				Name:         row.Artist.Name,
				Popularity:   row.Artist.Popularity,
				Genres:       []string{},
				Images:       convertPipelineImages(row.Artist.Images),
				ExternalURLs: row.Artist.ExternalURLs,
				Position:     row.Position,
				Score:        row.Score,
			})
		}
		stats, genreWarn := h.pipeline.GenreStats(ctx, snapshot, 10)
		topGenres = genreStatsToGenreCounts(stats)
		if genreWarn != "" && len(topGenres) == 1 && topGenres[0].Genre == "unknown" {
			// keep unknown with warning via empty genres list
		}
		_ = genreWarn

		topTracks := make([]TrackItem, 0, len(snapshot.TopTracks))
		for _, t := range snapshot.TopTracks {
			topTracks = append(topTracks, pipelineTrackToHandler(t))
		}
		recentItems := make([]PlayHistoryItem, 0, len(snapshot.RecentlyPlayed))
		for _, rp := range snapshot.RecentlyPlayed {
			recentItems = append(recentItems, PlayHistoryItem{
				Track:    pipelineTrackToHandler(rp.Track),
				PlayedAt: rp.PlayedAt,
			})
		}
		minutes, uniqueTracks, uniqueArtists := calculateRecentListeningStats(recentItems)
		h.writeCachedJSON(c, h.responseCacheKey("wrapped_summary", userID, timeRange, strconv.Itoa(limit)), 6*time.Hour, WrappedSummaryResponse{
			TimeRange:           timeRange,
			TopArtists:          topArtistItems,
			TopTracks:           topTracks,
			TopGenres:           topGenres,
			RecentPlaysCount:    len(recentItems),
			RecentMinutesTotal:  minutes,
			UniqueTracksRecent:  uniqueTracks,
			UniqueArtistsRecent: uniqueArtists,
		})
		return
	}

	topArtists, err := h.fetchTopArtistsFromSpotify(accessToken, timeRange, limit)
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	topTracks, err := h.fetchTopTracksFromSpotify(ctx, userID, accessToken, timeRange, limit)
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	recentlyPlayed, err := h.fetchRecentlyPlayedFromSpotify(ctx, userID, accessToken, 50, "", "")
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
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

	h.writeCachedJSON(c, h.responseCacheKey("wrapped_summary", userID, timeRange, strconv.Itoa(limit)), 6*time.Hour, resp)
}

func genreStatsToGenreCounts(stats []lastfmservice.GenreStat) []GenreCount {
	out := make([]GenreCount, 0, len(stats))
	seen := map[string]struct{}{}
	for _, s := range stats {
		genre := normalizeGenreValue(s.Genre)
		if genre == "" || genre == "unknown" || isClearlyInvalidGenre(genre) {
			continue
		}
		if _, ok := seen[genre]; ok {
			continue
		}
		seen[genre] = struct{}{}
		out = append(out, GenreCount{Genre: genre, Count: s.Count, Weight: s.Weight})
	}
	return out
}

func pipelineTrackToHandler(t pipeline.Track) TrackItem {
	artists := make([]SimpleArtist, 0, len(t.Artists))
	for _, a := range t.Artists {
		artists = append(artists, SimpleArtist{ID: a.ID, Name: a.Name})
	}
	return TrackItem{
		ID:         t.ID,
		Name:       t.Name,
		URI:        t.URI,
		DurationMS: t.DurationMS,
		Artists:    artists,
	}
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

	ctx := c.Request.Context()
	accessToken, err := h.getUserAccessToken(ctx, userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}
	if body, ok := h.cacheGet(ctx, h.responseCacheKey("wrapped_insights", userID, timeRange)); ok {
		c.Data(http.StatusOK, "application/json", body)
		return
	}

	topGenre := "unknown"
	topArtist := "unknown"
	topTrack := "unknown"
	var recentlyPlayed *RecentlyPlayedResponse

	if h.pipeline != nil {
		snapshot, snapErr := h.pipeline.FetchSnapshot(ctx, userID, timeRange, 20)
		if snapErr != nil {
			if h.writeSpotifyError(c, snapErr) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": snapErr.Error()})
			return
		}
		ranked := h.pipeline.RankedArtists(snapshot, 1)
		if len(ranked) > 0 && ranked[0].ArtistName != "" {
			topArtist = ranked[0].ArtistName
		}
		if len(snapshot.TopTracks) > 0 {
			topTrack = snapshot.TopTracks[0].Name
		}
		stats, _ := h.pipeline.GenreStats(ctx, snapshot, 1)
		if len(stats) > 0 && stats[0].Genre != "" {
			topGenre = stats[0].Genre
		}
		recentItems := make([]PlayHistoryItem, 0, len(snapshot.RecentlyPlayed))
		for _, rp := range snapshot.RecentlyPlayed {
			recentItems = append(recentItems, PlayHistoryItem{Track: pipelineTrackToHandler(rp.Track), PlayedAt: rp.PlayedAt})
		}
		recentlyPlayed = &RecentlyPlayedResponse{Items: recentItems}
	} else {
		topArtists, err := h.fetchTopArtistsFromSpotify(accessToken, timeRange, 20)
		if err != nil {
			if h.writeSpotifyError(c, err) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		topTracks, err := h.fetchTopTracksFromSpotify(ctx, userID, accessToken, timeRange, 20)
		if err != nil {
			if h.writeSpotifyError(c, err) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		recentlyPlayed, err = h.fetchRecentlyPlayedFromSpotify(ctx, userID, accessToken, 50, "", "")
		if err != nil {
			if h.writeSpotifyError(c, err) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		genres := topGenresFromArtists(topArtists.Items, 1)
		if len(genres) > 0 {
			topGenre = genres[0].Genre
		}
		if len(topArtists.Items) > 0 {
			topArtist = topArtists.Items[0].Name
		}
		if len(topTracks.Items) > 0 {
			topTrack = topTracks.Items[0].Name
		}
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

	h.writeCachedJSON(c, h.responseCacheKey("wrapped_insights", userID, timeRange), 6*time.Hour, WrappedInsightsResponse{
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

	accessToken, err := h.getUserAccessToken(c.Request.Context(), userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}
	if body, ok := h.cacheGet(c.Request.Context(), h.responseCacheKey("wrapped_timeline", userID)); ok {
		c.Data(http.StatusOK, "application/json", body)
		return
	}

	recentlyPlayed, err := h.fetchRecentlyPlayedFromSpotify(c.Request.Context(), userID, accessToken, 50, "", "")
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
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

	h.writeCachedJSON(c, h.responseCacheKey("wrapped_timeline", userID), 6*time.Hour, WrappedTimelineResponse{
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

	accessToken, err := h.getUserAccessToken(c.Request.Context(), userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	leftTracks, err := h.fetchTopTracksFromSpotify(c.Request.Context(), userID, accessToken, left, 20)
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rightTracks, err := h.fetchTopTracksFromSpotify(c.Request.Context(), userID, accessToken, right, 20)
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	leftArtists, err := h.fetchTopArtistsFromSpotify(accessToken, left, 20)
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	rightArtists, err := h.fetchTopArtistsFromSpotify(accessToken, right, 20)
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
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

	accessToken, err := h.getUserAccessToken(c.Request.Context(), userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	topTracks, err := h.fetchTopTracksFromSpotify(c.Request.Context(), userID, accessToken, timeRange, 20)
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
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

	features, err := h.fetchAudioFeaturesFromSpotify(c.Request.Context(), userID, accessToken, collectTrackIDs(topTracks.Items))
	if err != nil {
		if isSpotifyForbidden(err) || isSpotifyNotFound(err) {
			log.Printf("stats profile: audio-features restricted: %v", err)
			c.JSON(http.StatusOK, resp)
			return
		}
		if h.writeSpotifyError(c, err) {
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

	ctx := c.Request.Context()
	if _, err := h.getUserAccessToken(ctx, userID); err != nil {
		h.writeTokenError(c, err)
		return
	}
	if body, ok := h.cacheGet(ctx, h.responseCacheKey("stats_genres", userID, timeRange)); ok {
		c.Data(http.StatusOK, "application/json", body)
		return
	}

	genres := []GenreCount{}
	warning := ""

	if h.pipeline != nil {
		snapshot, err := h.pipeline.FetchSnapshot(ctx, userID, timeRange, 50)
		if err != nil {
			if h.writeSpotifyError(c, err) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		stats, genreWarn := h.pipeline.GenreStats(ctx, snapshot, 20)
		genres = genreStatsToGenreCounts(stats)
		warning = genreWarn
	} else {
		accessToken, _ := h.getUserAccessToken(ctx, userID)
		topArtists, err := h.fetchTopArtistsFromSpotify(accessToken, timeRange, 50)
		if err != nil {
			if h.writeSpotifyError(c, err) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		genres = topGenresFromArtists(topArtists.Items, 20)
		if len(genres) == 0 {
			warning = "no genres available without last.fm pipeline"
		}
	}

	if len(genres) == 0 {
		genres = []GenreCount{{Genre: "unknown", Count: 0, Weight: 0}}
		if warning == "" {
			warning = "no last.fm tags found"
		}
	}

	resp := gin.H{
		"time_range": timeRange,
		"genres":     genres,
		"source":     "lastfm",
	}
	if notice := mapGenreWarningToNotice(warning); notice != "" {
		resp["notice"] = notice
	}

	h.writeCachedJSON(c, h.responseCacheKey("stats_genres", userID, timeRange), 6*time.Hour, resp)
}

// GetStatsListeningTime — статистика времени прослушивания по recently played
func (h *SpotifyAPIHandler) GetStatsListeningTime(c *gin.Context) {
	userID := c.Param("userId")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing userId in path"})
		return
	}

	accessToken, err := h.getUserAccessToken(c.Request.Context(), userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	recentlyPlayed, err := h.fetchRecentlyPlayedFromSpotify(c.Request.Context(), userID, accessToken, 50, "", "")
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
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

	ctx := c.Request.Context()
	accessToken, err := h.getUserAccessToken(ctx, userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}
	if body, ok := h.cacheGet(ctx, h.responseCacheKey("recommendations", userID, mode, strconv.Itoa(limit))); ok {
		c.Data(http.StatusOK, "application/json", body)
		return
	}

	if h.recSvc != nil && h.pipeline != nil {
		snapshot, snapErr := h.pipeline.FetchSnapshot(ctx, userID, defaultTimeRange, 50)
		if snapErr != nil {
			if h.writeSpotifyError(c, snapErr) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": snapErr.Error()})
			return
		}
		ranked := h.pipeline.RankedArtists(snapshot, 5)
		stats, genreWarn := h.pipeline.GenreStats(ctx, snapshot, 3)
		seedArtists := pipeline.SeedArtistIDs(ranked, 5)
		seedGenres := pipeline.SeedGenreNames(stats, 3)

		result, recErr := h.recSvc.Recommend(ctx, userID, mode, limit, seedArtists, seedGenres, nil)
		if recErr == nil && result != nil {
			h.writeCachedJSON(c, h.responseCacheKey("recommendations", userID, mode, strconv.Itoa(limit)), time.Hour, recommendationResultToHandler(result))
			return
		}
		if recErr != nil {
			if h.writeSpotifyError(c, recErr) {
				return
			}
			log.Printf("recommendations: spotify path failed: %v", recErr)
		}

		if recs, ok := h.recommendViaLastFM(ctx, userID, accessToken, snapshot, stats, seedArtists, mode, limit); ok {
			if recErr != nil {
				recs.Notice = NoticeRecommendationsListening
			}
			_ = genreWarn
			c.JSON(http.StatusOK, recs)
			return
		}

		fallback, fbErr := h.fallbackRecommendations(ctx, userID, accessToken, limit)
		if fbErr != nil {
			log.Printf("recommendations: all strategies failed: spotify=%v fallback=%v", recErr, fbErr)
			h.writeCachedJSON(c, h.responseCacheKey("recommendations_unavailable", userID, mode, strconv.Itoa(limit)), time.Hour, RecommendationsResponse{
				Mode:   mode,
				Items:  []RecommendationItem{},
				Source: "recommendations_unavailable",
				Notice: NoticeRecommendationsUnavailable,
			})
			return
		}
		fallback.Notice = NoticeRecommendationsTopTracks
		h.writeCachedJSON(c, h.responseCacheKey("recommendations", userID, mode, strconv.Itoa(limit)), time.Hour, fallback)
		return
	}

	recs, err := h.fetchRecommendationsFromSpotify(ctx, userID, accessToken, mode, limit)
	if err != nil {
		if h.writeSpotifyError(c, err) {
			return
		}
		fallback, fbErr := h.fallbackRecommendations(ctx, userID, accessToken, limit)
		if fbErr != nil {
			log.Printf("recommendations: fallback failed: spotify=%v fallback=%v", err, fbErr)
			h.writeCachedJSON(c, h.responseCacheKey("recommendations_unavailable", userID, mode, strconv.Itoa(limit)), time.Hour, RecommendationsResponse{
				Mode:   mode,
				Items:  []RecommendationItem{},
				Source: "recommendations_unavailable",
				Notice: NoticeRecommendationsUnavailable,
			})
			return
		}
		fallback.Notice = NoticeRecommendationsTopTracks
		h.writeCachedJSON(c, h.responseCacheKey("recommendations", userID, mode, strconv.Itoa(limit)), time.Hour, fallback)
		return
	}

	h.writeCachedJSON(c, h.responseCacheKey("recommendations", userID, mode, strconv.Itoa(limit)), time.Hour, recs)
}

func recommendationResultToHandler(result *recommendationservice.Result) RecommendationsResponse {
	items := make([]RecommendationItem, 0, len(result.Items))
	for _, item := range result.Items {
		artists := make([]SimpleArtist, 0, len(item.Track.Artists))
		for _, a := range item.Track.Artists {
			artists = append(artists, SimpleArtist{ID: a.ID, Name: a.Name})
		}
		items = append(items, RecommendationItem{
			Track: TrackItem{
				ID:           item.Track.ID,
				Name:         item.Track.Name,
				URI:          item.Track.URI,
				Popularity:   item.Track.Popularity,
				DurationMS:   item.Track.DurationMS,
				Artists:      artists,
				ExternalURLs: item.Track.ExternalURLs,
			},
			Reason: item.Reason,
		})
	}
	return RecommendationsResponse{
		Mode:          result.Mode,
		Items:         items,
		Source:        result.Source,
		Warning:       result.Warning,
		SeedTrackIDs:  result.SeedTrackIDs,
		SeedArtistIDs: result.SeedArtistIDs,
		SeedGenres:    result.SeedGenres,
	}
}

func recommendationItemsToExportTracks(items []RecommendationItem) []ExportTrack {
	out := make([]ExportTrack, 0, len(items))
	for _, item := range items {
		artists := make([]string, 0, len(item.Track.Artists))
		for _, a := range item.Track.Artists {
			if a.Name != "" {
				artists = append(artists, a.Name)
			}
		}
		spotifyURL := ""
		if item.Track.ExternalURLs != nil {
			spotifyURL = item.Track.ExternalURLs["spotify"]
		}
		out = append(out, ExportTrack{
			ID:         item.Track.ID,
			Name:       item.Track.Name,
			Artists:    artists,
			URI:        item.Track.URI,
			SpotifyURL: spotifyURL,
		})
	}
	return out
}

func (h *SpotifyAPIHandler) fallbackRecommendationItems(ctx context.Context, userID, accessToken string, limit int) ([]recommendationservice.Item, error) {
	fallback, err := h.fallbackRecommendations(ctx, userID, accessToken, limit)
	if err != nil {
		return nil, err
	}
	items := make([]recommendationservice.Item, 0, len(fallback.Items))
	for _, item := range fallback.Items {
		artists := make([]recommendationservice.SimpleArtist, 0, len(item.Track.Artists))
		for _, a := range item.Track.Artists {
			artists = append(artists, recommendationservice.SimpleArtist{ID: a.ID, Name: a.Name})
		}
		items = append(items, recommendationservice.Item{
			Track: recommendationservice.Track{
				ID:           item.Track.ID,
				Name:         item.Track.Name,
				URI:          item.Track.URI,
				Popularity:   item.Track.Popularity,
				DurationMS:   item.Track.DurationMS,
				Artists:      artists,
				ExternalURLs: item.Track.ExternalURLs,
			},
			Reason: item.Reason,
		})
	}
	return items, nil
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

	accessToken, err := h.getUserAccessToken(c.Request.Context(), userID)
	if err != nil {
		h.writeTokenError(c, err)
		return
	}

	ctx := c.Request.Context()
	recs, err := h.fetchRecommendationsFromSpotify(ctx, userID, accessToken, "comfort", limit)
	if err != nil {
		fallback, fbErr := h.fallbackRecommendations(ctx, userID, accessToken, limit)
		if fbErr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "could not load tracks for export"})
			return
		}
		c.JSON(http.StatusOK, playlistExportFallbackResponse(fallback.Items, NoticePlaylistExportFallback))
		return
	}

	uris := recommendationURIs(recs.Items)
	playlistID, playlistURL, err := h.createPlaylistWithTracks(accessToken, uris)
	if err != nil {
		if isSpotifyForbidden(err) || isSpotifyNotFound(err) {
			log.Printf("playlist create restricted: %v", err)
			notice := NoticePlaylistScopesRestricted
			if strings.Contains(strings.ToLower(err.Error()), "scope") {
				notice = NoticePlaylistScopesRestricted
			}
			c.JSON(http.StatusOK, playlistExportFallbackResponse(recs.Items, notice))
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, CreatePlaylistResponse{
		PlaylistID:  playlistID,
		PlaylistURL: playlistURL,
		TracksAdded: len(uris),
	})
}

func playlistExportFallbackResponse(items []RecommendationItem, notice string) CreatePlaylistResponse {
	uris := recommendationURIs(items)
	return CreatePlaylistResponse{
		TracksAdded:    0,
		Notice:         notice,
		FallbackURIs:   uris,
		FallbackTracks: recommendationItemsToExportTracks(items),
	}
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
func (h *SpotifyAPIHandler) fetchTopArtistsLite(ctx context.Context, userID, accessToken, timeRange string, limit int) (*TopArtistsResponse, error) {
	url := "https://api.spotify.com/v1/me/top/artists" +
		"?time_range=" + timeRange +
		"&limit=" + strconv.Itoa(limit)

	body, err := h.spotifyGET(ctx, userID, accessToken, url)
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

func (h *SpotifyAPIHandler) fetchTopTracksFromSpotify(ctx context.Context, userID, accessToken, timeRange string, limit int) (*TopTracksResponse, error) {
	url := "https://api.spotify.com/v1/me/top/tracks" +
		"?time_range=" + timeRange +
		"&limit=" + strconv.Itoa(limit)

	body, err := h.spotifyGET(ctx, userID, accessToken, url)
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

func (h *SpotifyAPIHandler) fetchRecentlyPlayedFromSpotify(ctx context.Context, userID, accessToken string, limit int, before string, after string) (*RecentlyPlayedResponse, error) {
	url := "https://api.spotify.com/v1/me/player/recently-played?limit=" + strconv.Itoa(limit)
	if before != "" {
		url += "&before=" + before
	}
	if after != "" {
		url += "&after=" + after
	}

	body, err := h.spotifyGET(ctx, userID, accessToken, url)
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

func (h *SpotifyAPIHandler) fetchAudioFeaturesFromSpotify(ctx context.Context, userID, accessToken string, trackIDs []string) (*AudioFeaturesResponse, error) {
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
		body, err := h.doSpotifyGETWithAppFallback(ctx, userID, accessToken, url)
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
		log.Printf("DIAG handler doSpotifyGET http error: url=%s err=%v\n%s", url, err, debug.Stack())
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("DIAG handler doSpotifyGET read error: url=%s err=%v\n%s", url, err, debug.Stack())
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("DIAG handler doSpotifyGET non-200: url=%s status=%d body=%s", url, resp.StatusCode, snippet(body, 500))
		return nil, domain.NewError(resp.StatusCode, "spotify api error: "+string(body))
	}

	return body, nil
}

func snippet(b []byte, n int) string {
	s := string(b)
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func (h *SpotifyAPIHandler) getUserAccessToken(ctx context.Context, userID string) (string, error) {
	if h.tokenSvc != nil {
		return h.tokenSvc.GetAccessToken(ctx, userID)
	}
	tokens, err := h.tokenRepo.GetByUserID(ctx, userID)
	if err != nil {
		return "", fmt.Errorf("user tokens not found")
	}
	if tokens.AccessToken == "" {
		return "", fmt.Errorf("no access token")
	}
	return tokens.AccessToken, nil
}

func (h *SpotifyAPIHandler) spotifyGET(ctx context.Context, userID, accessToken, rawURL string) ([]byte, error) {
	log.Printf("DIAG handler spotifyGET: user=%s url=%s using_api_service=%v", userID, rawURL, h.spotifyAPI != nil)
	if h.spotifyAPI != nil && userID != "" {
		body, err := h.spotifyAPI.DoGET(ctx, userID, rawURL)
		if err != nil {
			log.Printf("DIAG handler spotifyGET error via APIService: user=%s url=%s err=%v", userID, rawURL, err)
		}
		return body, err
	}
	body, err := h.doSpotifyGET(accessToken, rawURL)
	if err != nil {
		log.Printf("DIAG handler spotifyGET error via direct HTTP: user=%s url=%s err=%v", userID, rawURL, err)
	}
	return body, err
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

func isSpotifyUnauthorized(err error) bool {
	apiErr, ok := err.(domain.APIError)
	if !ok {
		return false
	}
	return apiErr.StatusCode == http.StatusUnauthorized
}

func isSpotifyRateLimited(err error) bool {
	apiErr, ok := err.(domain.APIError)
	if !ok {
		return false
	}
	return apiErr.StatusCode == http.StatusTooManyRequests
}

func isSpotifyNotFound(err error) bool {
	apiErr, ok := err.(domain.APIError)
	if !ok {
		return false
	}
	return apiErr.StatusCode == http.StatusNotFound
}

// deriveAudioFeaturesFromTracks builds proxy audio metrics when Spotify audio-features is unavailable.
func deriveAudioFeaturesFromTracks(tracks []TrackItem) []AudioFeatureItem {
	features := make([]AudioFeatureItem, 0, len(tracks))
	for _, track := range tracks {
		if track.ID == "" {
			continue
		}
		pop := float64(track.Popularity) / 100.0
		if pop <= 0 {
			pop = 0.45
		}
		durationMin := float64(track.DurationMS) / 60000.0
		if durationMin <= 0 {
			durationMin = 3.5
		}
		tempo := 90.0 + pop*60.0
		if durationMin < 2.5 {
			tempo += 15
		} else if durationMin > 4.5 {
			tempo -= 10
		}
		features = append(features, AudioFeatureItem{
			ID:           track.ID,
			Danceability: clamp01(0.35 + pop*0.5),
			Energy:       clamp01(0.3 + pop*0.55),
			Valence:      clamp01(0.25 + pop*0.45),
			Tempo:        tempo,
			Acousticness: clamp01(1.0 - pop*0.7),
			Speechiness:  clamp01(0.08 + pop*0.15),
			Liveness:     clamp01(0.12 + pop*0.2),
			Loudness:     -8.0 - (1.0-pop)*6.0,
			DurationMS:   track.DurationMS,
		})
	}
	return features
}

func (h *SpotifyAPIHandler) recommendViaLastFM(
	ctx context.Context,
	userID string,
	accessToken string,
	snapshot *pipeline.UserSnapshot,
	genreStats []lastfmservice.GenreStat,
	seedArtistIDs []string,
	mode string,
	limit int,
) (*RecommendationsResponse, bool) {
	if h.lastfmClient == nil || !h.lastfmClient.Enabled() {
		return nil, false
	}

	baseArtists := make([]ArtistItem, 0, len(snapshot.SpotifyArtists))
	for _, a := range snapshot.SpotifyArtists {
		baseArtists = append(baseArtists, ArtistItem{ID: a.ID, Name: a.Name})
	}
	if len(baseArtists) == 0 {
		return nil, false
	}

	similarNames := h.collectLastFmSimilarArtists(ctx, baseArtists, 3, 12)
	if len(similarNames) == 0 {
		log.Printf("recommendations: last.fm returned no similar artists for %d base artists", len(baseArtists))
		return nil, false
	}

	items, recWarnings := h.buildRecommendationsFromSimilarArtists(ctx, userID, accessToken, similarNames, limit)
	if len(items) == 0 {
		log.Printf("recommendations: no tracks found for last.fm similar artists")
		return nil, false
	}

	topGenre := ""
	seedGenres := make([]string, 0, 3)
	for _, g := range genreStats {
		if g.Genre == "" || g.Genre == "unknown" {
			continue
		}
		seedGenres = append(seedGenres, g.Genre)
		if topGenre == "" {
			topGenre = g.Genre
		}
		if len(seedGenres) >= 3 {
			break
		}
	}
	for i := range items {
		reason := items[i].Reason
		if topGenre != "" {
			items[i].Reason = fmt.Sprintf("genre %s via %s", topGenre, reason)
		}
	}

	if len(recWarnings) > 0 {
		log.Printf("recommendations: last.fm track lookup: %s", mergeWarnings(recWarnings...))
	}

	return &RecommendationsResponse{
		Mode:          mode,
		Items:         items,
		Source:        "lastfm_similar_artists",
		SeedArtistIDs: seedArtistIDs,
		SeedGenres:    seedGenres,
	}, true
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

func (h *SpotifyAPIHandler) fetchArtistsDetailsIndividually(client *http.Client, userID, accessToken string, artistIDs []string) (map[string]ArtistItem, error) {
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

			artist, err := h.fetchArtistByID(client, userID, accessToken, artistID)
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

func (h *SpotifyAPIHandler) fetchArtistByID(client *http.Client, userID, accessToken string, artistID string) (ArtistItem, error) {
	url := "https://api.spotify.com/v1/artists/" + artistID
	if appToken, err := h.getAppAccessToken(); err == nil {
		if body, appErr := h.doSpotifyGETWithAppFallback(context.Background(), userID, appToken, url); appErr == nil {
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

	body, err := h.spotifyGET(context.Background(), userID, accessToken, url)
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
			normalized := normalizeGenreValue(g)
			if normalized == "" || isClearlyInvalidGenre(normalized) {
				continue
			}
			genreMap[normalized]++
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

func normalizeGenres(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		normalized := normalizeGenreValue(value)
		if normalized == "" || isClearlyInvalidGenre(normalized) {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	return result
}

func normalizeGenreValue(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return ""
	}
	value = strings.Join(strings.Fields(value), " ")
	return value
}

func isClearlyInvalidGenre(value string) bool {
	if value == "" {
		return true
	}
	blocked := map[string]struct{}{
		"govno":     {},
		"asdf":      {},
		"test":      {},
		"null":      {},
		"undefined": {},
		"n/a":       {},
		"none":      {},
		"unknown":   {},
	}
	if _, ok := blocked[value]; ok {
		return true
	}
	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return true
	}
	if strings.Contains(value, "<script") || strings.Contains(value, "function(") {
		return true
	}
	if strings.Contains(value, "based on last.fm similar artist:") {
		return true
	}
	return false
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

func (h *SpotifyAPIHandler) fetchRecommendationsFromSpotify(ctx context.Context, userID, accessToken, mode string, limit int) (*RecommendationsResponse, error) {
	if h.recSvc != nil && h.pipeline != nil && userID != "" {
		snapshot, err := h.pipeline.FetchSnapshot(ctx, userID, defaultTimeRange, 50)
		if err == nil {
			ranked := h.pipeline.RankedArtists(snapshot, 5)
			stats, genreWarn := h.pipeline.GenreStats(ctx, snapshot, 3)
			seedArtists := pipeline.SeedArtistIDs(ranked, 5)
			seedGenres := pipeline.SeedGenreNames(stats, 3)

			result, recErr := h.recSvc.Recommend(ctx, userID, mode, limit, seedArtists, seedGenres, nil)
			if recErr == nil && result != nil {
				resp := recommendationResultToHandler(result)
				return &resp, nil
			}

			if recs, ok := h.recommendViaLastFM(ctx, userID, accessToken, snapshot, stats, seedArtists, mode, limit); ok {
				if recErr != nil {
					recs.Notice = NoticeRecommendationsListening
				}
				_ = genreWarn
				return recs, nil
			}
		}
	}
	return h.fallbackRecommendations(ctx, userID, accessToken, limit)
}

func (h *SpotifyAPIHandler) collectLastFmSimilarArtists(ctx context.Context, baseArtists []ArtistItem, perArtistLimit int, maxArtists int) []string {
	if h == nil || h.lastfmClient == nil || !h.lastfmClient.Enabled() {
		return []string{}
	}
	if perArtistLimit <= 0 {
		perArtistLimit = 3
	}
	if maxArtists <= 0 {
		maxArtists = 12
	}

	seen := map[string]struct{}{}
	result := make([]string, 0, maxArtists)
	for _, artist := range baseArtists {
		if len(result) >= maxArtists {
			break
		}
		sourceName := strings.TrimSpace(artist.Name)
		if sourceName == "" {
			continue
		}
		similar, err := h.lastfmClient.GetArtistSimilar(ctx, sourceName, perArtistLimit)
		if err != nil {
			continue
		}
		for _, item := range similar {
			name := strings.TrimSpace(item.Name)
			if name == "" {
				continue
			}
			key := strings.ToLower(name)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, name)
			if len(result) >= maxArtists {
				break
			}
		}
	}
	return result
}

func (h *SpotifyAPIHandler) buildRecommendationsFromSimilarArtists(ctx context.Context, userID, accessToken string, artistNames []string, limit int) ([]RecommendationItem, []string) {
	warnings := []string{}
	if len(artistNames) == 0 || limit <= 0 {
		return []RecommendationItem{}, warnings
	}

	items := make([]RecommendationItem, 0, limit)
	seenTracks := map[string]struct{}{}
	for _, artistName := range artistNames {
		if len(items) >= limit {
			break
		}
		tracks, err := h.searchSpotifyTracksByArtistName(ctx, userID, accessToken, artistName, 3)
		if err != nil {
			warnings = append(warnings, "track lookup failed for "+artistName)
			continue
		}
		for _, track := range tracks {
			if len(items) >= limit {
				break
			}
			if track.ID == "" {
				continue
			}
			if _, ok := seenTracks[track.ID]; ok {
				continue
			}
			seenTracks[track.ID] = struct{}{}
			items = append(items, RecommendationItem{
				Track:  track,
				Reason: "based on Last.fm similar artist: " + artistName,
			})
		}
	}
	return items, warnings
}

func (h *SpotifyAPIHandler) searchSpotifyTracksByArtistName(ctx context.Context, userID, accessToken string, artistName string, limit int) ([]TrackItem, error) {
	artistName = strings.TrimSpace(artistName)
	if artistName == "" {
		return []TrackItem{}, nil
	}
	if limit <= 0 {
		limit = 3
	}

	query := url.Values{}
	query.Set("type", "track")
	query.Set("limit", strconv.Itoa(limit))
	query.Set("market", "from_token")
	query.Set("q", fmt.Sprintf("artist:%q", artistName))

	body, err := h.doSpotifyGETWithAppFallback(ctx, userID, accessToken, "https://api.spotify.com/v1/search?"+query.Encode())
	if err != nil {
		return nil, err
	}

	var payload struct {
		Tracks struct {
			Items []TrackItem `json:"items"`
		} `json:"tracks"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	for i := range payload.Tracks.Items {
		if payload.Tracks.Items[i].ExternalURLs == nil {
			payload.Tracks.Items[i].ExternalURLs = make(map[string]string)
		}
		if payload.Tracks.Items[i].Artists == nil {
			payload.Tracks.Items[i].Artists = []SimpleArtist{}
		}
	}

	return payload.Tracks.Items, nil
}

func (h *SpotifyAPIHandler) fallbackRecommendations(ctx context.Context, userID, accessToken string, limit int) (*RecommendationsResponse, error) {
	topTracks, err := h.fetchTopTracksFromSpotify(ctx, userID, accessToken, defaultTimeRange, limit)
	if err != nil {
		return nil, err
	}
	items := make([]RecommendationItem, 0, len(topTracks.Items))
	for _, t := range topTracks.Items {
		items = append(items, RecommendationItem{Track: t, Reason: "top_tracks_fallback"})
	}
	return &RecommendationsResponse{
		Mode:          "comfort",
		Items:         items,
		Source:        "top_tracks_fallback",
		Notice:        NoticeRecommendationsTopTracks,
		SeedTrackIDs:  pickFirstIDs(collectTrackIDs(topTracks.Items), 5),
		SeedArtistIDs: []string{},
	}, nil
}

func (h *SpotifyAPIHandler) createPlaylistWithTracks(accessToken string, uris []string) (string, string, error) {
	if len(uris) == 0 {
		return "", "", nil
	}

	createPayload := map[string]interface{}{
		"name":        "Sleepwalker Wrapped Picks",
		"description": "Auto-generated from sleepwalker.fm recommendations",
		"public":      false,
	}
	body, _ := json.Marshal(createPayload)
	req, err := http.NewRequest("POST", "https://api.spotify.com/v1/me/playlists", strings.NewReader(string(body)))
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
	addReq, err := http.NewRequest("POST", "https://api.spotify.com/v1/playlists/"+created.ID+"/items", strings.NewReader(string(addBody)))
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
