package recommendation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"sleepwalker.fm/internal/domain"
	lastfmservice "sleepwalker.fm/internal/service/lastfm"
	spotifyapi "sleepwalker.fm/internal/service/spotify"
)

// Track is a minimal recommendation result.
type Track struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	URI          string `json:"uri"`
	Popularity   int    `json:"popularity"`
	DurationMS   int    `json:"duration_ms"`
	Artists      []SimpleArtist `json:"artists"`
	Album        SimpleAlbum    `json:"album"`
	ExternalURLs map[string]string `json:"external_urls"`
}

type SimpleArtist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type SimpleAlbum struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Images []struct {
		URL string `json:"url"`
	} `json:"images"`
}

// Item is one recommended track with a human-readable reason.
type Item struct {
	Track  Track  `json:"track"`
	Reason string `json:"reason"`
}

// Result is the recommendation engine output.
type Result struct {
	Mode          string   `json:"mode"`
	Items         []Item   `json:"items"`
	Source        string   `json:"source"`
	Warning       string   `json:"warning,omitempty"`
	SeedTrackIDs  []string `json:"seed_track_ids"`
	SeedArtistIDs []string `json:"seed_artist_ids"`
	SeedGenres    []string `json:"seed_genres,omitempty"`
}

// Service generates genre-driven Spotify recommendations.
type Service struct {
	spotify *spotifyapi.APIService
	lastfm  *lastfmservice.Service
}

func NewService(spotify *spotifyapi.APIService, lastfm *lastfmservice.Service) *Service {
	return &Service{spotify: spotify, lastfm: lastfm}
}

// Recommend uses top Last.fm genres and computed top artist IDs as Spotify seeds.
func (s *Service) Recommend(
	ctx context.Context,
	userID string,
	mode string,
	limit int,
	seedArtistIDs []string,
	seedGenres []string,
	fallbackTopTracks func(ctx context.Context) ([]Item, error),
) (*Result, error) {
	if limit <= 0 {
		limit = 20
	}
	if mode != "explore" {
		mode = "comfort"
	}

	artistSeeds := pickFirst(seedArtistIDs, 2)
	genreSeeds := pickFirst(seedGenres, 3)
	if len(artistSeeds)+len(genreSeeds) == 0 {
		return s.fallback(mode, limit, fallbackTopTracks, "no seeds available")
	}

	items, err := s.fetchSpotifyRecommendations(ctx, userID, mode, limit, artistSeeds, genreSeeds, nil)
	if err != nil || len(items) == 0 {
		if err != nil {
			if errors.Is(err, ErrSpotifyRecommendationsUnavailable) {
				log.Printf("spotify recommendations restricted for app (seeds artists=%v genres=%v)", artistSeeds, genreSeeds)
				return nil, ErrSpotifyRecommendationsUnavailable
			}
			log.Printf("spotify recommendations failed (seeds artists=%v genres=%v): %v", artistSeeds, genreSeeds, err)
			return nil, err
		}
		log.Printf("spotify recommendations returned 0 tracks (seeds artists=%v genres=%v)", artistSeeds, genreSeeds)
		return nil, ErrSpotifyRecommendationsUnavailable
	}

	return &Result{
		Mode:          mode,
		Items:         items,
		Source:        "spotify_recommendations",
		SeedArtistIDs: artistSeeds,
		SeedGenres:    genreSeeds,
	}, nil
}

func (s *Service) fallback(mode string, limit int, fn func(context.Context) ([]Item, error), warn string) (*Result, error) {
	if fn == nil {
		return &Result{
			Mode:   mode,
			Source: "recommendations_unavailable",
			Warning: compact(warn, "recommendations unavailable"),
		}, nil
	}
	items, err := fn(context.Background())
	if err != nil {
		return &Result{
			Mode:    mode,
			Source:  "recommendations_unavailable",
			Warning: compact(warn, err.Error()),
		}, nil
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return &Result{
		Mode:    mode,
		Items:   items,
		Source:  "top_tracks_fallback",
		Warning: compact(warn, "using top tracks fallback"),
	}, nil
}

func (s *Service) fetchSpotifyRecommendations(
	ctx context.Context,
	userID, mode string,
	limit int,
	seedArtists, seedGenres, seedTracks []string,
) ([]Item, error) {
	query := url.Values{}
	query.Set("limit", strconv.Itoa(limit))
	if len(seedArtists) > 0 {
		query.Set("seed_artists", strings.Join(seedArtists, ","))
	}
	if len(seedGenres) > 0 {
		query.Set("seed_genres", strings.Join(seedGenres, ","))
	}
	if len(seedTracks) > 0 {
		query.Set("seed_tracks", strings.Join(seedTracks, ","))
	}
	if mode == "explore" {
		query.Set("min_popularity", "20")
	} else {
		query.Set("target_popularity", "55")
	}

	rawURL := s.spotify.BaseURL() + "/recommendations?" + query.Encode()
	body, err := s.spotify.DoGET(ctx, userID, rawURL)
	if err != nil {
		if apiErr, ok := err.(domain.APIError); ok {
			if apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusForbidden {
				return nil, ErrSpotifyRecommendationsUnavailable
			}
			return nil, fmt.Errorf("spotify recommendations %d: %s", apiErr.StatusCode, apiErr.Message)
		}
		return nil, err
	}

	var payload struct {
		Tracks []Track `json:"tracks"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	items := make([]Item, 0, len(payload.Tracks))
	for _, track := range payload.Tracks {
		if track.ID == "" {
			continue
		}
		if track.ExternalURLs == nil {
			track.ExternalURLs = map[string]string{}
		}
		reason := "genre-driven pick"
		if len(seedGenres) > 0 {
			reason = fmt.Sprintf("based on genres: %s", strings.Join(seedGenres, ", "))
		}
		items = append(items, Item{Track: track, Reason: reason})
	}
	return items, nil
}

func pickFirst(values []string, n int) []string {
	if n <= 0 || len(values) == 0 {
		return []string{}
	}
	out := make([]string, 0, n)
	for _, v := range values {
		if v == "" {
			continue
		}
		out = append(out, v)
		if len(out) >= n {
			break
		}
	}
	return out
}

func compact(parts ...string) string {
	nonEmpty := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			nonEmpty = append(nonEmpty, p)
		}
	}
	return strings.Join(nonEmpty, "; ")
}
