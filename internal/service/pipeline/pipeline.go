package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"

	analyticsservice "sleepwalker.fm/internal/service/analytics"
	lastfmservice "sleepwalker.fm/internal/service/lastfm"
	spotifyapi "sleepwalker.fm/internal/service/spotify"
)

// UserSnapshot is Spotify listening data used for analytics.
type UserSnapshot struct {
	TopTracks       []Track
	RecentlyPlayed  []PlayHistoryItem
	SpotifyArtists  []Artist // raw spotify top artists for metadata merge
}

type Artist struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Popularity int      `json:"popularity"`
	Genres     []string `json:"genres"`
	Images     []Image  `json:"images"`
	ExternalURLs map[string]string `json:"external_urls"`
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

type PlayHistoryItem struct {
	Track    Track  `json:"track"`
	PlayedAt string `json:"played_at"`
}

// Pipeline fetches Spotify data and enriches with Last.fm genres.
type Pipeline struct {
	spotify *spotifyapi.APIService
	lastfm  *lastfmservice.Service
}

func New(spotify *spotifyapi.APIService, lastfm *lastfmservice.Service) *Pipeline {
	return &Pipeline{spotify: spotify, lastfm: lastfm}
}

func (p *Pipeline) FetchSnapshot(ctx context.Context, userID, timeRange string, limit int) (*UserSnapshot, error) {
	if limit <= 0 {
		limit = 20
	}
	topTracks, err := p.fetchTopTracks(ctx, userID, timeRange, limit)
	if err != nil {
		return nil, err
	}
	recent, err := p.fetchRecentlyPlayed(ctx, userID, 50)
	if err != nil {
		return nil, err
	}
	artists, err := p.fetchTopArtistsRaw(ctx, userID, timeRange, limit)
	if err != nil {
		return nil, err
	}
	return &UserSnapshot{
		TopTracks:      topTracks,
		RecentlyPlayed: recent,
		SpotifyArtists: artists,
	}, nil
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
	for _, a := range snapshot.SpotifyArtists {
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
