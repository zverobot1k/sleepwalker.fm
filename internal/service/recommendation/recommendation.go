package recommendation

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"

	lastfmservice "sleepwalker.fm/internal/service/lastfm"
	"sleepwalker.fm/internal/service/pipeline"
)

// Track is a minimal recommendation result.
type Track struct {
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

// Service generates recommendations using Last.fm for genres and Spotify as fallback.
type Service struct {
	lastfm *lastfmservice.Client
}

func NewService(lastfm *lastfmservice.Client) *Service {
	return &Service{lastfm: lastfm}
}

func (s *Service) RecommendFromSnapshot(ctx context.Context, snapshot *pipeline.SpotifySnapshot, mode string, limit int) (*Result, error) {
	if snapshot == nil {
		return nil, fmt.Errorf("missing snapshot")
	}
	if limit <= 0 {
		limit = 20
	}
	if limit < 10 {
		limit = 10
	}
	if limit > 50 {
		limit = 50
	}
	if mode != "explore" {
		mode = "comfort"
	}

	seedArtistIDs := pickArtistIDs(snapshot.TopArtists, 5)
	seedTrackIDs := pickTrackIDs(snapshot.TopTracks, 5)
	var seedGenres []string
	source := "spotify_fallback"
	warning := ""

	// Try Last.fm for genre seeds
	if s != nil && s.lastfm != nil && s.lastfm.Enabled() {
		artistNames := uniqueArtistNames(snapshot.TopArtists)
		genreStats, genreWarning := s.aggregateGenres(ctx, artistNames, 5)
		seedGenres = pickGenreNames(genreStats, 5)
		if len(seedGenres) > 0 {
			source = "lastfm_genre_engine"
			log.Printf("DIAG recommendations using lastfm genres=%v", seedGenres)
		} else {
			warning = genreWarning
			log.Printf("DIAG recommendations lastfm failed: %s, using fallback", warning)
		}
	}

	// Fallback to Spotify artist genres
	if len(seedGenres) == 0 {
		seedGenres = extractSpotifyGenres(snapshot.TopArtists, 5)
		if len(seedGenres) > 0 {
			source = "spotify_artist_genres"
		} else {
			seedGenres = []string{"pop", "rock"}
			source = "default_genres"
		}
		log.Printf("DIAG recommendations using fallback genres=%v source=%s", seedGenres, source)
	}

	// Build items: prefer Last.fm when available
	var items []Item
	if s != nil && s.lastfm != nil && s.lastfm.Enabled() && source == "lastfm_genre_engine" {
		artistNames := uniqueArtistNames(snapshot.TopArtists)
		genreStats, _ := s.aggregateGenres(ctx, artistNames, 5)
		items = s.buildItems(ctx, snapshot, genreStats, seedGenres, limit*2)
	} else {
		items = buildBasicRecommendations(snapshot, seedGenres, limit*2)
	}

	// Ensure enough items
	if len(items) < limit {
		extra := buildSpotifyFallbackRecommendations(snapshot, limit-len(items))
		items = append(items, extra...)
	}
	if len(items) > limit {
		items = items[:limit]
	}

	return &Result{
		Mode:          mode,
		Items:         items,
		Source:        source,
		Warning:       warning,
		SeedTrackIDs:  seedTrackIDs,
		SeedArtistIDs: seedArtistIDs,
		SeedGenres:    seedGenres,
	}, nil
}

func (s *Service) buildItems(ctx context.Context, snapshot *pipeline.SpotifySnapshot, genreStats []lastfmservice.GenreStat, seedGenres []string, limit int) []Item {
	type candidate struct {
		artist string
		title  string
		score  float64
		reason string
		genre  string
	}

	candidates := map[string]candidate{}
	addCandidate := func(artist, title string, score float64, reason, genre string) {
		artist = strings.TrimSpace(artist)
		title = strings.TrimSpace(title)
		if artist == "" || title == "" {
			return
		}
		key := candidateKey(artist, title)
		next := candidate{artist: artist, title: title, score: score, reason: reason, genre: genre}
		if current, ok := candidates[key]; ok && current.score >= next.score {
			return
		}
		candidates[key] = next
	}

	genreWeight := map[string]float64{}
	for _, stat := range genreStats {
		genreWeight[stat.Genre] = stat.Weight
	}

	// Use Last.fm tag/top tracks for each seed genre
	for gi, genre := range seedGenres {
		weight := genreWeight[genre]
		if weight <= 0 {
			weight = 1
		}
		tagTracks, err := s.lastfm.GetTagTopTracks(ctx, genre, 40)
		if err == nil {
			for idx, tr := range tagTracks {
				score := weight*100 - float64(idx)
				if gi == 0 {
					score += 10
				}
				addCandidate(tr.Artist, tr.Title, score, fmt.Sprintf("based on genre %s", genre), genre)
			}
		}
		// tag artists -> artist top tracks
		tagArtists, err := s.lastfm.GetTagTopArtists(ctx, genre, 20)
		if err == nil {
			for ai, artistName := range tagArtists {
				topTracks, err := s.lastfm.GetArtistTopTracks(ctx, artistName, 8)
				if err != nil {
					continue
				}
				for ti, tr := range topTracks {
					score := weight*90 - float64(ai) - float64(ti)/10
					addCandidate(tr.Artist, tr.Title, score, fmt.Sprintf("genre %s artist %s", genre, artistName), genre)
				}
			}
		}
	}

	// Also use user's top artists for similar tracks
	for ai, artist := range snapshot.TopArtists {
		artistName := strings.TrimSpace(artist.Name)
		if artistName == "" {
			continue
		}
		baseScore := float64(len(snapshot.TopArtists)-ai) * 12
		topTracks, err := s.lastfm.GetArtistTopTracks(ctx, artistName, 10)
		if err == nil {
			for ti, tr := range topTracks {
				addCandidate(tr.Artist, tr.Title, baseScore-float64(ti), fmt.Sprintf("top artist %s", artistName), "artist")
			}
		}
		similarArtists, err := s.lastfm.GetArtistSimilar(ctx, artistName, 8)
		if err == nil {
			for si, sim := range similarArtists {
				simName := strings.TrimSpace(sim.Name)
				if simName == "" {
					continue
				}
				simTracks, err := s.lastfm.GetArtistTopTracks(ctx, simName, 6)
				if err != nil {
					continue
				}
				for ti, tr := range simTracks {
					score := baseScore/2 + float64(len(similarArtists)-si)*2 - float64(ti)
					addCandidate(tr.Artist, tr.Title, score, fmt.Sprintf("similar to %s", artistName), "similar")
				}
			}
		}
	}

	// Convert and sort
	ordered := make([]candidate, 0, len(candidates))
	for _, c := range candidates {
		ordered = append(ordered, c)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].score == ordered[j].score {
			if ordered[i].genre == ordered[j].genre {
				if ordered[i].artist == ordered[j].artist {
					return ordered[i].title < ordered[j].title
				}
				return ordered[i].artist < ordered[j].artist
			}
			return ordered[i].genre < ordered[j].genre
		}
		return ordered[i].score > ordered[j].score
	})

	if len(ordered) > limit {
		ordered = ordered[:limit]
	}

	items := make([]Item, 0, len(ordered))
	for _, c := range ordered {
		items = append(items, Item{
			Track: Track{
				ID:           candidateID(c.artist, c.title),
				Name:         c.title,
				URI:          "",
				Popularity:   int(c.score),
				DurationMS:   0,
				Artists:      []SimpleArtist{{Name: c.artist}},
				Album:        SimpleAlbum{},
				ExternalURLs: map[string]string{},
			},
			Reason: c.reason,
		})
	}
	return items
}

func extractSpotifyGenres(artists []pipeline.Artist, max int) []string {
	if max <= 0 {
		max = 5
	}
	genreCount := map[string]int{}
	for _, artist := range artists {
		for _, g := range artist.Genres {
			g = strings.TrimSpace(strings.ToLower(g))
			if g != "" && g != "unknown" {
				genreCount[g]++
			}
		}
	}
	type kv struct {
		genre string
		count int
	}
	var sorted []kv
	for g, c := range genreCount {
		sorted = append(sorted, kv{g, c})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
	result := make([]string, 0, max)
	for _, kv := range sorted {
		result = append(result, kv.genre)
		if len(result) >= max {
			break
		}
	}
	return result
}

func buildBasicRecommendations(snapshot *pipeline.SpotifySnapshot, seedGenres []string, limit int) []Item {
	items := make([]Item, 0, limit)
	for idx, track := range snapshot.TopTracks {
		if idx >= limit {
			break
		}
		items = append(items, Item{
			Track: Track{
				ID:         track.ID,
				Name:       track.Name,
				URI:        track.URI,
				DurationMS: track.DurationMS,
				Artists: func() []SimpleArtist {
					res := make([]SimpleArtist, len(track.Artists))
					for i, a := range track.Artists {
						res[i] = SimpleArtist{ID: a.ID, Name: a.Name}
					}
					return res
				}(),
				ExternalURLs: map[string]string{},
			}, Reason: "from your top tracks",
		})
	}
	return items
}

func buildSpotifyFallbackRecommendations(snapshot *pipeline.SpotifySnapshot, count int) []Item {
	items := make([]Item, 0, count)
	for idx, item := range snapshot.RecentlyPlayed {
		if idx >= count {
			break
		}
		items = append(items, Item{
			Track: Track{
				ID:         item.Track.ID,
				Name:       item.Track.Name,
				URI:        item.Track.URI,
				DurationMS: item.Track.DurationMS,
				Artists: func() []SimpleArtist {
					res := make([]SimpleArtist, len(item.Track.Artists))
					for i, a := range item.Track.Artists {
						res[i] = SimpleArtist{ID: a.ID, Name: a.Name}
					}
					return res
				}(),
				ExternalURLs: map[string]string{},
			}, Reason: "from your recently played",
		})
	}
	return items
}

func (s *Service) aggregateGenres(ctx context.Context, artistNames []string, tagsPerArtist int) ([]lastfmservice.GenreStat, string) {
	if s == nil || s.lastfm == nil || !s.lastfm.Enabled() {
		return []lastfmservice.GenreStat{{Genre: "unknown", Count: 0, Weight: 0}}, "last.fm unavailable"
	}
	if tagsPerArtist <= 0 {
		tagsPerArtist = 5
	}
	counts := map[string]int{}
	for _, artist := range artistNames {
		artist = strings.TrimSpace(artist)
		if artist == "" {
			continue
		}
		tags, err := s.lastfm.GetArtistTopTags(ctx, artist, tagsPerArtist)
		if err != nil {
			continue
		}
		for _, tag := range tags {
			n := lastfmservice.NormalizeTag(tag)
			if n == "" {
				continue
			}
			counts[n]++
		}
	}
	if len(counts) == 0 {
		return []lastfmservice.GenreStat{{Genre: "unknown", Count: 0, Weight: 0}}, "no last.fm tags found"
	}
	total := 0
	for _, c := range counts {
		total += c
	}
	stats := make([]lastfmservice.GenreStat, 0, len(counts))
	for g, c := range counts {
		stats = append(stats, lastfmservice.GenreStat{Genre: g, Count: c, Weight: float64(c) / float64(total)})
	}
	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count == stats[j].Count {
			return stats[i].Genre < stats[j].Genre
		}
		return stats[i].Count > stats[j].Count
	})
	return stats, ""
}

func uniqueArtistNames(items []pipeline.Artist) []string {
	seen := map[string]struct{}{}
	res := make([]string, 0, len(items))
	for _, it := range items {
		n := strings.TrimSpace(it.Name)
		if n == "" {
			continue
		}
		k := strings.ToLower(n)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		res = append(res, n)
	}
	return res
}

func pickGenreNames(stats []lastfmservice.GenreStat, max int) []string {
	if max <= 0 {
		return []string{}
	}
	res := make([]string, 0, max)
	for _, s := range stats {
		g := strings.TrimSpace(s.Genre)
		if g == "" || g == "unknown" {
			continue
		}
		res = append(res, g)
		if len(res) >= max {
			break
		}
	}
	return res
}

func pickArtistIDs(items []pipeline.Artist, max int) []string {
	if max <= 0 {
		return []string{}
	}
	res := make([]string, 0, max)
	for _, it := range items {
		if it.ID == "" {
			continue
		}
		res = append(res, it.ID)
		if len(res) >= max {
			break
		}
	}
	return res
}

func pickTrackIDs(items []pipeline.Track, max int) []string {
	if max <= 0 {
		return []string{}
	}
	res := make([]string, 0, max)
	for _, it := range items {
		if it.ID == "" {
			continue
		}
		res = append(res, it.ID)
		if len(res) >= max {
			break
		}
	}
	return res
}

func candidateKey(artist, title string) string {
	return strings.ToLower(strings.TrimSpace(artist + "|" + title))
}
func candidateID(artist, title string) string { return "lastfm:" + slug(artist) + ":" + slug(title) }

func slug(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteRune('-')
			lastDash = true
		}
	}
	out := b.String()
	if out == "" {
		return "unknown"
	}
	return out
}
