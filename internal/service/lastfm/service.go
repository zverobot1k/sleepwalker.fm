package lastfm

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
)

// GenreStat is a normalized Last.fm tag with frequency and weight.
type GenreStat struct {
	Genre  string  `json:"genre"`
	Count  int     `json:"count"`
	Weight float64 `json:"weight"`
}

// Service enriches Spotify entities with Last.fm tags (genres only).
type Service struct {
	client *Client
}

func NewService(client *Client) *Service {
	return &Service{client: client}
}

func (s *Service) Enabled() bool {
	return s != nil && s.client != nil && s.client.Enabled()
}

// NormalizeTag lowercases, trims, and collapses similar tag variants.
func NormalizeTag(tag string) string {
	tag = strings.TrimSpace(strings.ToLower(tag))
	if tag == "" {
		return ""
	}
	replacements := map[string]string{
		"hip-hop": "hip hop",
		"hiphop":  "hip hop",
		"r&b":     "rnb",
		"rhythm-and-blues": "rnb",
		"indie-rock":       "indie rock",
		"post-punk":        "post punk",
	}
	if canon, ok := replacements[tag]; ok {
		return canon
	}
	return tag
}

// mergeSimilarTags deduplicates tags that normalize to the same key.
func mergeSimilarTags(tags []string) []string {
	seen := map[string]string{}
	order := make([]string, 0, len(tags))
	for _, tag := range tags {
		key := NormalizeTag(tag)
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = key
		order = append(order, key)
	}
	return order
}

// ArtistTags fetches and normalizes top tags for an artist.
func (s *Service) ArtistTags(ctx context.Context, artistName string, limit int) ([]string, error) {
	if !s.Enabled() {
		return []string{}, nil
	}
	raw, err := s.client.GetArtistTopTags(ctx, artistName, limit)
	if err != nil {
		return nil, err
	}
	return mergeSimilarTags(raw), nil
}

// AggregateArtistGenres collects tags from multiple artists and returns weighted genre stats.
func (s *Service) AggregateArtistGenres(ctx context.Context, artistNames []string, tagsPerArtist int) ([]GenreStat, string) {
	if !s.Enabled() {
		return []GenreStat{{Genre: "unknown", Count: 0, Weight: 0}}, "last.fm unavailable"
	}
	if tagsPerArtist <= 0 {
		tagsPerArtist = 5
	}

	counts := map[string]int{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	var fetchErrors []string

	for _, name := range artistNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		wg.Add(1)
		go func(artist string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			tags, err := s.ArtistTags(ctx, artist, tagsPerArtist)
			if err != nil {
				mu.Lock()
				fetchErrors = append(fetchErrors, artist+": "+err.Error())
				mu.Unlock()
				return
			}
			if len(tags) == 0 {
				return
			}
			mu.Lock()
			for _, tag := range tags {
				counts[tag]++
			}
			mu.Unlock()
		}(name)
	}
	wg.Wait()

	if len(counts) == 0 {
		warn := "no last.fm tags found"
		if len(fetchErrors) > 0 {
			log.Printf("lastfm genre aggregation failures: %s", strings.Join(fetchErrors, "; "))
			warn = warn + ": " + fetchErrors[0]
		}
		return []GenreStat{{Genre: "unknown", Count: 0, Weight: 0}}, warn
	}

	total := 0
	for _, c := range counts {
		total += c
	}

	stats := make([]GenreStat, 0, len(counts))
	for genre, count := range counts {
		weight := float64(count) / float64(total)
		stats = append(stats, GenreStat{Genre: genre, Count: count, Weight: weight})
	}

	sort.Slice(stats, func(i, j int) bool {
		if stats[i].Count == stats[j].Count {
			return stats[i].Genre < stats[j].Genre
		}
		return stats[i].Count > stats[j].Count
	})

	return stats, ""
}
