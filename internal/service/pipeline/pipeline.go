package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
	analyticsservice "sleepwalker.fm/internal/service/analytics"
	lastfmservice "sleepwalker.fm/internal/service/lastfm"
	spotifyapi "sleepwalker.fm/internal/service/spotify"
)

const snapshotCacheTTL = 30 * time.Minute

// SpotifySnapshot is the cached Spotify listening snapshot used by the app.
type SpotifySnapshot struct {
	UserID         string            `json:"user_id"`
	TimeRange      string            `json:"time_range"`
	TopTracks      []Track           `json:"top_tracks"`
	TopArtists     []Artist          `json:"top_artists"`
	RecentlyPlayed []PlayHistoryItem `json:"recently_played"`
	AudioFeatures  []AudioFeature    `json:"audio_features,omitempty"`
	CreatedAt      time.Time         `json:"created_at"`

	// Async enrichment fields (optional)
	TopGenres    []lastfmservice.GenreStat `json:"top_genres,omitempty"`
	LastFmSource string                    `json:"lastfm_source,omitempty"`
	SeedGenres   []string                  `json:"seed_genres,omitempty"`
	SeedArtists  []string                  `json:"seed_artist_ids,omitempty"`
	SeedTracks   []string                  `json:"seed_track_ids,omitempty"`

	// Deprecated alias kept for older call sites inside the handler layer.
	SpotifyArtists []Artist `json:"-"`
}

// Extend snapshot with async enrichment fields (non-blocking)
// These fields are populated asynchronously and are optional in the fast snapshot.
type _snapshotEnrichmentFields struct{}

// UserSnapshot is kept for compatibility with existing call sites.
type UserSnapshot = SpotifySnapshot

type Artist struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Popularity   int               `json:"popularity"`
	Genres       []string          `json:"genres"`
	Images       []Image           `json:"images"`
	ExternalURLs map[string]string `json:"external_urls"`
	Score        float64           `json:"score,omitempty"`
	Position     int               `json:"position,omitempty"`
}

type Image struct {
	URL string `json:"url"`
}

type Track struct {
	ID         string         `json:"id"`
	Name       string         `json:"name"`
	URI        string         `json:"uri"`
	DurationMS int            `json:"duration_ms"`
	Artists    []SimpleArtist `json:"artists"`
}

type SimpleArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type AudioFeature struct {
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

type PlayHistoryItem struct {
	Track    Track  `json:"track"`
	PlayedAt string `json:"played_at"`
}

// Pipeline fetches Spotify data and enriches with Last.fm genres.
type Pipeline struct {
	spotify *spotifyapi.APIService
	lastfm  *lastfmservice.Service
	cache   *redis.Client
	group   singleflight.Group
}

func New(spotify *spotifyapi.APIService, lastfm *lastfmservice.Service, cache *redis.Client) *Pipeline {
	return &Pipeline{spotify: spotify, lastfm: lastfm, cache: cache}
}

func (p *Pipeline) FetchSnapshot(ctx context.Context, userID, timeRange string) (*SpotifySnapshot, error) {
	if timeRange == "" {
		timeRange = "medium_term"
	}
	cacheKey := p.snapshotCacheKey(userID, timeRange)

	if snapshot, ok := p.readSnapshotCache(ctx, cacheKey); ok {
		if len(snapshot.TopTracks) > 0 && len(snapshot.TopArtists) > 0 {
			log.Printf("DIAG snapshot cache HIT user=%s time_range=%s tracks=%d artists=%d", userID, timeRange, len(snapshot.TopTracks), len(snapshot.TopArtists))
			return snapshot, nil
		}
		log.Printf("DIAG snapshot cache HIT but incomplete user=%s — deleting and rebuilding", userID)
		if p.cache != nil {
			_ = p.cache.Del(ctx, cacheKey).Err()
		}
	}
	log.Printf("DIAG snapshot cache MISS user=%s time_range=%s", userID, timeRange)

	value, err, shared := p.group.Do(cacheKey, func() (any, error) {
		if snapshot, ok := p.readSnapshotCache(ctx, cacheKey); ok {
			if len(snapshot.TopTracks) > 0 && len(snapshot.TopArtists) > 0 {
				log.Printf("DIAG snapshot cache REUSED user=%s time_range=%s", userID, timeRange)
				return snapshot, nil
			}
		}
		// Retry once: emptySpotifyPayload() silently fills empty arrays when Spotify is degraded.
		// Treat empty required data as an error and retry after a brief wait.
		var lastErr error
		for attempt := 0; attempt < 2; attempt++ {
			if attempt > 0 {
				log.Printf("DIAG snapshot build retry attempt=%d user=%s last_err=%v", attempt+1, userID, lastErr)
				time.Sleep(2 * time.Second)
			}
			snapshot, err := p.buildSnapshot(ctx, userID, timeRange)
			if err == nil {
				p.writeSnapshotCache(ctx, cacheKey, snapshot)
				log.Printf("DIAG snapshot cache REBUILT user=%s attempt=%d tracks=%d artists=%d recent=%d", userID, attempt+1, len(snapshot.TopTracks), len(snapshot.TopArtists), len(snapshot.RecentlyPlayed))
				return snapshot, nil
			}
			lastErr = err
		}
		return nil, fmt.Errorf("snapshot build failed after 2 attempts: %w", lastErr)
	})
	if shared {
		log.Printf("DIAG snapshot singleflight shared user=%s time_range=%s", userID, timeRange)
	}
	if err != nil {
		return nil, err
	}
	return value.(*SpotifySnapshot), nil
}

func (p *Pipeline) buildSnapshot(ctx context.Context, userID, timeRange string) (*SpotifySnapshot, error) {
	topTracks, err := p.fetchTopTracks(ctx, userID, timeRange, 50)
	if err != nil {
		return nil, fmt.Errorf("top_tracks: %w", err)
	}
	if len(topTracks) == 0 {
		return nil, fmt.Errorf("top_tracks: empty response (spotify may be degraded)")
	}

	artists, err := p.fetchTopArtistsRaw(ctx, userID, timeRange, 50)
	if err != nil {
		return nil, fmt.Errorf("top_artists: %w", err)
	}
	if len(artists) == 0 {
		return nil, fmt.Errorf("top_artists: empty response (spotify may be degraded)")
	}

	recent, recentErr := p.fetchRecentlyPlayed(ctx, userID, 50)
	if recentErr != nil {
		log.Printf("DIAG buildSnapshot: recently_played failed user=%s err=%v (non-fatal)", userID, recentErr)
	}

	snapshot := &SpotifySnapshot{
		UserID:         userID,
		TimeRange:      timeRange,
		TopTracks:      topTracks,
		TopArtists:     artists,
		RecentlyPlayed: recent,
		CreatedAt:      time.Now().UTC(),
	}
	snapshot.SpotifyArtists = snapshot.TopArtists
	return snapshot, nil
}

func (p *Pipeline) RankedArtists(snapshot *UserSnapshot, limit int) []analyticsservice.RankedArtist {
	recent := make([]analyticsservice.ArtistPlayInput, 0)
	for _, item := range snapshot.RecentlyPlayed {
		for _, a := range item.Track.Artists {
			recent = append(recent, analyticsservice.ArtistPlayInput{ArtistID: a.ID, ArtistName: a.Name})
		}
	}
	topTrackArtists := make([]analyticsservice.ArtistPlayInput, 0)
	for _, track := range snapshot.TopTracks {
		for _, a := range track.Artists {
			topTrackArtists = append(topTrackArtists, analyticsservice.ArtistPlayInput{ArtistID: a.ID, ArtistName: a.Name})
		}
	}
	return analyticsservice.ComputeTopArtists(recent, topTrackArtists, limit)
}

func (p *Pipeline) GenreStats(ctx context.Context, snapshot *UserSnapshot, topN int) ([]lastfmservice.GenreStat, string) {
	names := make([]string, 0)
	seen := map[string]struct{}{}
	for _, a := range snapshot.TopArtists {
		if a.Name == "" {
			continue
		}
		key := a.ID
		if key == "" {
			key = a.Name
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		names = append(names, a.Name)
	}
	for _, item := range snapshot.RecentlyPlayed {
		for _, a := range item.Track.Artists {
			if a.Name == "" {
				continue
			}
			key := a.ID
			if key == "" {
				key = a.Name
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			names = append(names, a.Name)
		}
	}
	return p.lastfm.AggregateArtistGenres(ctx, names, 5)
}

func (p *Pipeline) fetchTopTracks(ctx context.Context, userID, timeRange string, limit int) ([]Track, error) {
	rawURL := p.spotify.BaseURL() + "/me/top/tracks?" + url.Values{
		"time_range": {timeRange},
		"limit":      {strconv.Itoa(limit)},
	}.Encode()
	body, err := p.spotify.DoGET(ctx, userID, rawURL)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Items []Track `json:"items"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload.Items, nil
}

func (p *Pipeline) fetchAudioFeatures(ctx context.Context, userID string, trackIDs []string) ([]AudioFeature, error) {
	if len(trackIDs) == 0 {
		return []AudioFeature{}, nil
	}
	rawURL := p.spotify.BaseURL() + "/audio-features?ids=" + url.QueryEscape(strings.Join(trackIDs, ","))
	body, err := p.spotify.DoGET(ctx, userID, rawURL)
	if err != nil {
		return nil, err
	}
	var payload struct {
		AudioFeatures []AudioFeature `json:"audio_features"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	filtered := make([]AudioFeature, 0, len(payload.AudioFeatures))
	for _, feature := range payload.AudioFeatures {
		if feature.ID == "" {
			continue
		}
		filtered = append(filtered, feature)
	}
	return filtered, nil
}

func (p *Pipeline) fetchRecentlyPlayed(ctx context.Context, userID string, limit int) ([]PlayHistoryItem, error) {
	rawURL := p.spotify.BaseURL() + "/me/player/recently-played?" + url.Values{
		"limit": {strconv.Itoa(limit)},
	}.Encode()
	body, err := p.spotify.DoGET(ctx, userID, rawURL)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Items []PlayHistoryItem `json:"items"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload.Items, nil
}

func (p *Pipeline) fetchTopArtistsRaw(ctx context.Context, userID, timeRange string, limit int) ([]Artist, error) {
	rawURL := p.spotify.BaseURL() + "/me/top/artists?" + url.Values{
		"time_range": {timeRange},
		"limit":      {strconv.Itoa(limit)},
	}.Encode()
	body, err := p.spotify.DoGET(ctx, userID, rawURL)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Items []Artist `json:"items"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	return payload.Items, nil
}

// SeedArtistIDs returns top ranked artist IDs for recommendation seeds.
func SeedArtistIDs(ranked []analyticsservice.RankedArtist, n int) []string {
	ids := make([]string, 0, n)
	for _, a := range ranked {
		if a.ArtistID == "" {
			continue
		}
		ids = append(ids, a.ArtistID)
		if len(ids) >= n {
			break
		}
	}
	return ids
}

// SeedGenreNames returns top genre names from Last.fm stats.
func SeedGenreNames(stats []lastfmservice.GenreStat, n int) []string {
	names := make([]string, 0, n)
	for _, s := range stats {
		if s.Genre == "" || s.Genre == "unknown" {
			continue
		}
		names = append(names, s.Genre)
		if len(names) >= n {
			break
		}
	}
	return names
}

// MergeRankedIntoArtists overlays computed rank/score onto Spotify artist items by ID.
func MergeRankedIntoArtists(spotifyArtists []Artist, ranked []analyticsservice.RankedArtist) []RankedArtistOutput {
	byID := map[string]Artist{}
	for _, a := range spotifyArtists {
		byID[a.ID] = a
	}
	out := make([]RankedArtistOutput, 0, len(ranked))
	for _, r := range ranked {
		base, ok := byID[r.ArtistID]
		if !ok {
			base = Artist{ID: r.ArtistID, Name: r.ArtistName}
		}
		out = append(out, RankedArtistOutput{
			Artist:   base,
			Score:    r.Score,
			Position: r.Position,
		})
	}
	return out
}

type RankedArtistOutput struct {
	Artist   Artist
	Score    float64
	Position int
}

func FormatPipelineError(step string, err error) error {
	return fmt.Errorf("%s: %w", step, err)
}

func (p *Pipeline) snapshotCacheKey(userID, timeRange string) string {
	return "swfm:snapshot:" + userID + ":" + timeRange
}

func (p *Pipeline) readSnapshotCache(ctx context.Context, key string) (*SpotifySnapshot, bool) {
	if p.cache == nil {
		return nil, false
	}
	body, err := p.cache.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}
	var snapshot SpotifySnapshot
	if err := json.Unmarshal(body, &snapshot); err != nil {
		return nil, false
	}
	return &snapshot, true
}

func (p *Pipeline) writeSnapshotCache(ctx context.Context, key string, snapshot *SpotifySnapshot) {
	if p.cache == nil || snapshot == nil {
		return
	}
	if len(snapshot.TopTracks) == 0 || len(snapshot.TopArtists) == 0 {
		log.Printf("DIAG writeSnapshotCache: refusing to cache incomplete snapshot (tracks=%d artists=%d)", len(snapshot.TopTracks), len(snapshot.TopArtists))
		return
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return
	}
	_ = p.cache.Set(ctx, key, body, snapshotCacheTTL).Err()
}

func collectTrackIDs(tracks []Track, limit int) []string {
	if limit <= 0 {
		limit = len(tracks)
	}
	ids := make([]string, 0, limit)
	for _, track := range tracks {
		if track.ID == "" {
			continue
		}
		ids = append(ids, track.ID)
		if len(ids) >= limit {
			break
		}
	}
	return ids
}

// FetchAudioFeaturesAsync loads audio features without blocking.
// Called async from handler after snapshot is cached.
func (p *Pipeline) FetchAudioFeaturesAsync(ctx context.Context, userID string, snapshot *SpotifySnapshot) {
	if snapshot == nil || len(snapshot.TopTracks) == 0 {
		return
	}
	go func() {
		trackIDs := collectTrackIDs(snapshot.TopTracks, 20)
		if len(trackIDs) == 0 {
			return
		}
		features, err := p.fetchAudioFeatures(ctx, userID, trackIDs)
		if err != nil {
			log.Printf("DIAG async audio features failed user=%s err=%v", userID, err)
			return
		}
		snapshot.AudioFeatures = features
		log.Printf("DIAG async audio features loaded user=%s count=%d", userID, len(features))
	}()
}

// PopulateGenreStatsAsync loads top genres from Last.fm async.
func (p *Pipeline) PopulateGenreStatsAsync(ctx context.Context, snapshot *SpotifySnapshot) {
	if snapshot == nil {
		return
	}
	go func() {
		stats, warning := p.GenreStats(ctx, snapshot, 10)
		snapshot.TopGenres = stats
		if warning != "" {
			snapshot.LastFmSource = "fallback"
		} else {
			snapshot.LastFmSource = "lastfm"
		}
		log.Printf("DIAG async genre stats loaded genres=%d source=%s", len(stats), snapshot.LastFmSource)
	}()
}

// PopulateRecommendationSeedsAsync prepares seed data for recommendations async.
func (p *Pipeline) PopulateRecommendationSeedsAsync(ctx context.Context, snapshot *SpotifySnapshot) {
	if snapshot == nil {
		return
	}
	go func() {
		// Genre seeds from Last.fm (with fallback)
		genreStats, _ := p.GenreStats(ctx, snapshot, 5)
		seedGenres := SeedGenreNames(genreStats, 5)

		// Fallback: use Spotify genres if Last.fm returns nothing
		if len(seedGenres) == 0 {
			seen := map[string]struct{}{}
			for _, artist := range snapshot.TopArtists {
				for _, g := range artist.Genres {
					if g != "" && g != "unknown" {
						if _, ok := seen[g]; !ok {
							seedGenres = append(seedGenres, g)
							seen[g] = struct{}{}
							if len(seedGenres) >= 5 {
								break
							}
						}
					}
				}
			}
		}

		snapshot.SeedGenres = seedGenres
		snapshot.SeedArtists = SeedArtistIDs(p.RankedArtists(snapshot, 5), 5)
		snapshot.SeedTracks = collectTrackIDs(snapshot.TopTracks, 5)
		log.Printf("DIAG async recommendation seeds loaded genres=%d artists=%d tracks=%d", len(seedGenres), len(snapshot.SeedArtists), len(snapshot.SeedTracks))
	}()
}
