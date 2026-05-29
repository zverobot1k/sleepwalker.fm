package analytics

import (
	"sort"
)

// ArtistPlayInput is minimal data for ranking artists.
type ArtistPlayInput struct {
	ArtistID   string
	ArtistName string
}

// RankedArtist is a computed top artist with explicit position and score.
type RankedArtist struct {
	ArtistID   string
	ArtistName string
	Score      float64
	Position   int
}

const (
	recentPlayWeight    = 1.5
	topTrackOccurWeight = 1.0
)

// ComputeTopArtists ranks artists by recent plays and top-track occurrences (not Spotify order).
func ComputeTopArtists(
	recentPlays []ArtistPlayInput,
	topTrackArtists []ArtistPlayInput,
	limit int,
) []RankedArtist {
	recentCounts := map[string]int{}
	names := map[string]string{}

	for _, play := range recentPlays {
		if play.ArtistID == "" {
			continue
		}
		recentCounts[play.ArtistID]++
		if play.ArtistName != "" {
			names[play.ArtistID] = play.ArtistName
		}
	}

	trackCounts := map[string]int{}
	for _, play := range topTrackArtists {
		if play.ArtistID == "" {
			continue
		}
		trackCounts[play.ArtistID]++
		if play.ArtistName != "" {
			names[play.ArtistID] = play.ArtistName
		}
	}

	artistIDs := map[string]struct{}{}
	for id := range recentCounts {
		artistIDs[id] = struct{}{}
	}
	for id := range trackCounts {
		artistIDs[id] = struct{}{}
	}

	type scored struct {
		id    string
		score float64
	}
	items := make([]scored, 0, len(artistIDs))
	for id := range artistIDs {
		score := float64(recentCounts[id])*recentPlayWeight + float64(trackCounts[id])*topTrackOccurWeight
		if score <= 0 {
			continue
		}
		items = append(items, scored{id: id, score: score})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].id < items[j].id
		}
		return items[i].score > items[j].score
	})

	if limit > 0 && len(items) > limit {
		items = items[:limit]
	}

	result := make([]RankedArtist, 0, len(items))
	for i, item := range items {
		result = append(result, RankedArtist{
			ArtistID:   item.id,
			ArtistName: names[item.id],
			Score:      item.score,
			Position:   i + 1,
		})
	}
	return result
}

// ExtractArtistPlaysFromRecent builds play inputs from recently played track artist lists.
func ExtractArtistPlaysFromRecent(getArtists func() []ArtistPlayInput) []ArtistPlayInput {
	return getArtists()
}
