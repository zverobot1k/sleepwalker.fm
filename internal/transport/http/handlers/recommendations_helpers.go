package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	affinityTrackWeight  = 5.0
	affinityShortWeight  = 10.0
	affinityMediumWeight = 7.0
	affinityLongWeight   = 5.0

	recencyHighHours   = 24.0
	recencyMediumHours = 72.0
	recencyHighWeight  = 6.0
	recencyMidWeight   = 3.0
	recencyLowWeight   = 1.0

	maxArtistBatchSize      = 50
	maxRelatedArtistLookups = 4
)

type affinityInputs struct {
	topTracksShort   []TrackItem
	topTracksMedium  []TrackItem
	topTracksLong    []TrackItem
	topArtistsShort  []ArtistItem
	topArtistsMedium []ArtistItem
	topArtistsLong   []ArtistItem
	recentlyPlayed   []PlayHistoryItem
}

type artistCacheEntry struct {
	artist    ArtistItem
	expiresAt time.Time
}

type artistCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]artistCacheEntry
}

type relatedArtistsCacheEntry struct {
	artists   []ArtistItem
	expiresAt time.Time
}

type relatedArtistsCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]relatedArtistsCacheEntry
}

func newArtistCache(ttl time.Duration) *artistCache {
	return &artistCache{ttl: ttl, items: make(map[string]artistCacheEntry)}
}

func (c *artistCache) get(id string) (ArtistItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[id]
	if !ok {
		return ArtistItem{}, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.items, id)
		return ArtistItem{}, false
	}
	return entry.artist, true
}

func (c *artistCache) set(id string, artist ArtistItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[id] = artistCacheEntry{artist: artist, expiresAt: time.Now().Add(c.ttl)}
}

func newRelatedArtistsCache(ttl time.Duration) *relatedArtistsCache {
	return &relatedArtistsCache{ttl: ttl, items: make(map[string]relatedArtistsCacheEntry)}
}

func (c *relatedArtistsCache) get(id string) ([]ArtistItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[id]
	if !ok {
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.items, id)
		return nil, false
	}
	return entry.artists, true
}

func (c *relatedArtistsCache) set(id string, artists []ArtistItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[id] = relatedArtistsCacheEntry{artists: artists, expiresAt: time.Now().Add(c.ttl)}
}

func computeAffinityScores(inputs affinityInputs) map[string]float64 {
	scores := map[string]float64{}
	trackCounts := map[string]int{}

	addTrackCounts := func(tracks []TrackItem) {
		for _, track := range tracks {
			for _, artist := range track.Artists {
				if artist.ID == "" {
					continue
				}
				trackCounts[artist.ID]++
			}
		}
	}

	addTrackCounts(inputs.topTracksShort)
	addTrackCounts(inputs.topTracksMedium)
	addTrackCounts(inputs.topTracksLong)

	for id, count := range trackCounts {
		scores[id] += float64(count) * affinityTrackWeight
	}

	addPresence := func(artists []ArtistItem, weight float64) {
		for _, artist := range artists {
			if artist.ID == "" {
				continue
			}
			scores[artist.ID] += weight
		}
	}

	addPresence(inputs.topArtistsShort, affinityShortWeight)
	addPresence(inputs.topArtistsMedium, affinityMediumWeight)
	addPresence(inputs.topArtistsLong, affinityLongWeight)

	now := time.Now()
	for _, item := range inputs.recentlyPlayed {
		playedAt, err := time.Parse(time.RFC3339, item.PlayedAt)
		if err != nil {
			continue
		}
		weight := recencyWeight(now, playedAt)
		for _, artist := range item.Track.Artists {
			if artist.ID == "" {
				continue
			}
			scores[artist.ID] += weight
		}
	}

	return scores
}

func recencyWeight(now time.Time, playedAt time.Time) float64 {
	hours := now.Sub(playedAt).Hours()
	if hours <= recencyHighHours {
		return recencyHighWeight
	}
	if hours <= recencyMediumHours {
		return recencyMidWeight
	}
	return recencyLowWeight
}

func rankArtistsByScore(scores map[string]float64, max int) []string {
	if len(scores) == 0 || max == 0 {
		return []string{}
	}

	type scoredArtist struct {
		id    string
		score float64
	}
	items := make([]scoredArtist, 0, len(scores))
	for id, score := range scores {
		items = append(items, scoredArtist{id: id, score: score})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].score == items[j].score {
			return items[i].id < items[j].id
		}
		return items[i].score > items[j].score
	})

	if max > len(items) {
		max = len(items)
	}

	ranked := make([]string, 0, max)
	for i := 0; i < max; i++ {
		ranked = append(ranked, items[i].id)
	}
	return ranked
}

func seedTrackIDs(inputs affinityInputs, max int) []string {
	if max <= 0 {
		return []string{}
	}

	ids := make([]string, 0, max)
	seen := map[string]struct{}{}

	appendTrackIDs := func(tracks []TrackItem) {
		for _, id := range collectTrackIDs(tracks) {
			if id == "" {
				continue
			}
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
			if len(ids) >= max {
				return
			}
		}
	}

	appendTrackIDs(inputs.topTracksShort)
	appendTrackIDs(inputs.topTracksMedium)
	appendTrackIDs(inputs.topTracksLong)

	if len(ids) < max {
		for _, item := range inputs.recentlyPlayed {
			if item.Track.ID == "" {
				continue
			}
			if _, ok := seen[item.Track.ID]; ok {
				continue
			}
			seen[item.Track.ID] = struct{}{}
			ids = append(ids, item.Track.ID)
			if len(ids) >= max {
				break
			}
		}
	}

	return ids
}

func appendUniqueIDs(dest []string, candidates []string, max int) []string {
	if max <= 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	for _, id := range dest {
		if id == "" {
			continue
		}
		seen[id] = struct{}{}
	}
	for _, id := range candidates {
		if len(dest) >= max {
			break
		}
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		dest = append(dest, id)
	}
	return dest
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, v := range values {
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		result = append(result, v)
	}
	return result
}

type audioProfile struct {
	danceability     float64
	energy           float64
	valence          float64
	acousticness     float64
	instrumentalness float64
	tempo            float64
	count            int
}

func buildAudioProfile(features []AudioFeatureItem) audioProfile {
	profile := audioProfile{}
	for _, feature := range features {
		profile.danceability += feature.Danceability
		profile.energy += feature.Energy
		profile.valence += feature.Valence
		profile.acousticness += feature.Acousticness
		profile.instrumentalness += feature.Instrumentalness
		profile.tempo += feature.Tempo
		profile.count++
	}

	if profile.count == 0 {
		return profile
	}

	denom := float64(profile.count)
	profile.danceability /= denom
	profile.energy /= denom
	profile.valence /= denom
	profile.acousticness /= denom
	profile.instrumentalness /= denom
	profile.tempo /= denom

	return profile
}

func (p audioProfile) hasData() bool {
	return p.count > 0
}

func (p audioProfile) targets(mode string) map[string]string {
	if !p.hasData() {
		return nil
	}

	energy := clamp01(p.energy)
	valence := clamp01(p.valence)
	if mode == "explore" {
		energy = clamp01(energy + 0.08)
		valence = clamp01(valence + 0.05)
	}

	return map[string]string{
		"target_danceability":     formatFloat(p.danceability),
		"target_energy":           formatFloat(energy),
		"target_valence":          formatFloat(valence),
		"target_acousticness":     formatFloat(p.acousticness),
		"target_instrumentalness": formatFloat(p.instrumentalness),
		"target_tempo":            formatTempo(p.tempo),
	}
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', 2, 64)
}

func formatTempo(value float64) string {
	if value <= 0 {
		return "0"
	}
	return strconv.FormatFloat(value, 'f', 0, 64)
}

func inferGenresFromRelatedArtists(related []ArtistItem, topN int) []string {
	if len(related) == 0 {
		return []string{}
	}

	genreMap := map[string]int{}
	for _, artist := range related {
		for _, genre := range artist.Genres {
			if genre == "" {
				continue
			}
			genreMap[genre]++
		}
	}

	type genreCount struct {
		name  string
		count int
	}
	items := make([]genreCount, 0, len(genreMap))
	for name, count := range genreMap {
		items = append(items, genreCount{name: name, count: count})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].name < items[j].name
		}
		return items[i].count > items[j].count
	})

	if topN > 0 && len(items) > topN {
		items = items[:topN]
	}

	genres := make([]string, 0, len(items))
	for _, item := range items {
		genres = append(genres, item.name)
	}

	return genres
}

func compactWarnings(warnings []string) string {
	seen := map[string]struct{}{}
	filtered := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		value := strings.TrimSpace(warning)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		filtered = append(filtered, value)
	}
	return strings.Join(filtered, "; ")
}

func (h *SpotifyAPIHandler) getArtistDetailsCached(accessToken string, artistIDs []string) (map[string]ArtistItem, error) {
	result := make(map[string]ArtistItem, len(artistIDs))
	missing := make([]string, 0, len(artistIDs))

	for _, id := range uniqueStrings(artistIDs) {
		if id == "" {
			continue
		}
		if h.artistCache != nil {
			if artist, ok := h.artistCache.get(id); ok {
				result[id] = artist
				continue
			}
		}
		missing = append(missing, id)
	}

	var fetchErr error
	skipIndividual := false
	for start := 0; start < len(missing); start += maxArtistBatchSize {
		end := start + maxArtistBatchSize
		if end > len(missing) {
			end = len(missing)
		}
		chunk := missing[start:end]
		artists, err := h.fetchArtistsBatch(accessToken, chunk)
		if err != nil {
			fetchErr = err
			if isSpotifyRateLimited(err) {
				skipIndividual = true
				break
			}
			continue
		}
		for _, artist := range artists {
			if artist.ID == "" {
				continue
			}
			result[artist.ID] = artist
			if h.artistCache != nil {
				h.artistCache.set(artist.ID, artist)
			}
		}
	}

	stillMissing := make([]string, 0)
	for _, id := range missing {
		if _, ok := result[id]; !ok {
			stillMissing = append(stillMissing, id)
		}
	}

	if len(stillMissing) > 0 && !skipIndividual {
		client := &http.Client{}
		detailsByID, err := h.fetchArtistsDetailsIndividually(client, accessToken, stillMissing)
		if err != nil && fetchErr == nil {
			fetchErr = err
		}
		for id, artist := range detailsByID {
			result[id] = artist
			if h.artistCache != nil {
				h.artistCache.set(id, artist)
			}
		}
	}

	if len(result) > 0 {
		return result, nil
	}

	return result, fetchErr
}

func (h *SpotifyAPIHandler) fetchArtistsBatch(accessToken string, artistIDs []string) ([]ArtistItem, error) {
	if len(artistIDs) == 0 {
		return []ArtistItem{}, nil
	}

	query := url.Values{}
	query.Set("ids", strings.Join(artistIDs, ","))
	body, err := h.doSpotifyGETWithAppFallback(accessToken, "https://api.spotify.com/v1/artists?"+query.Encode())
	if err != nil {
		return nil, err
	}

	var payload struct {
		Artists []ArtistItem `json:"artists"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	for i := range payload.Artists {
		if payload.Artists[i].Genres == nil {
			payload.Artists[i].Genres = []string{}
		}
		if payload.Artists[i].ExternalURLs == nil {
			payload.Artists[i].ExternalURLs = make(map[string]string)
		}
	}

	return payload.Artists, nil
}

func (h *SpotifyAPIHandler) getRelatedArtistsCached(accessToken string, artistID string) ([]ArtistItem, error) {
	if artistID == "" {
		return []ArtistItem{}, nil
	}
	if h.relatedArtistsCache != nil {
		if cached, ok := h.relatedArtistsCache.get(artistID); ok {
			return cached, nil
		}
	}

	artists, err := h.fetchRelatedArtistsFromSpotify(accessToken, artistID)
	if err != nil {
		return nil, err
	}
	if h.relatedArtistsCache != nil {
		h.relatedArtistsCache.set(artistID, artists)
	}
	return artists, nil
}

func (h *SpotifyAPIHandler) fetchRelatedArtistsFromSpotify(accessToken string, artistID string) ([]ArtistItem, error) {
	body, err := h.doSpotifyGETWithAppFallback(accessToken, "https://api.spotify.com/v1/artists/"+artistID+"/related-artists")
	if err != nil {
		return nil, err
	}

	var payload struct {
		Artists []ArtistItem `json:"artists"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}

	for i := range payload.Artists {
		if payload.Artists[i].Genres == nil {
			payload.Artists[i].Genres = []string{}
		}
		if payload.Artists[i].ExternalURLs == nil {
			payload.Artists[i].ExternalURLs = make(map[string]string)
		}
	}

	return payload.Artists, nil
}

func (h *SpotifyAPIHandler) expandSeedArtistsWithRelated(accessToken string, seedArtists []string, ranked []string, max int) ([]string, []string) {
	warnings := []string{}
	if len(seedArtists) >= max {
		return seedArtists, warnings
	}

	candidates := pickFirstIDs(ranked, maxRelatedArtistLookups)
	for _, candidateID := range candidates {
		if len(seedArtists) >= max {
			break
		}
		related, err := h.getRelatedArtistsCached(accessToken, candidateID)
		if err != nil {
			warnings = append(warnings, "related artists unavailable")
			continue
		}
		relatedIDs := make([]string, 0, len(related))
		for _, artist := range related {
			relatedIDs = append(relatedIDs, artist.ID)
		}
		seedArtists = appendUniqueIDs(seedArtists, relatedIDs, max)
	}

	return seedArtists, warnings
}

func (h *SpotifyAPIHandler) buildSeedGenres(accessToken string, artistIDs []string, maxGenres int) ([]string, []string) {
	warnings := []string{}
	if len(artistIDs) == 0 || maxGenres <= 0 {
		return []string{}, warnings
	}

	sampleIDs := pickFirstIDs(artistIDs, 8)
	artistDetails, err := h.getArtistDetailsCached(accessToken, sampleIDs)
	if err != nil {
		warnings = append(warnings, "artist genre lookup degraded")
	}

	artists := make([]ArtistItem, 0, len(sampleIDs))
	for _, id := range sampleIDs {
		artist, ok := artistDetails[id]
		if !ok {
			continue
		}
		if len(artist.Genres) == 0 {
			inferred, inferWarn := h.inferGenresWithRelated(accessToken, id, 3)
			if inferWarn != "" {
				warnings = append(warnings, inferWarn)
			}
			if len(inferred) > 0 {
				artist.Genres = inferred
				if h.artistCache != nil {
					h.artistCache.set(id, artist)
				}
			}
		}
		artists = append(artists, artist)
	}

	genreCounts := topGenresFromArtists(artists, maxGenres)
	genres := make([]string, 0, len(genreCounts))
	for _, g := range genreCounts {
		genres = append(genres, g.Genre)
	}

	return genres, warnings
}

func (h *SpotifyAPIHandler) inferGenresWithRelated(accessToken string, artistID string, topN int) ([]string, string) {
	related, err := h.getRelatedArtistsCached(accessToken, artistID)
	if err != nil {
		return []string{}, "related artists lookup failed"
	}
	if len(related) == 0 {
		return []string{}, "related artists empty for genre inference"
	}
	inferred := inferGenresFromRelatedArtists(related, topN)
	if len(inferred) == 0 {
		return []string{}, "related artists had no genres"
	}
	return inferred, ""
}

func (h *SpotifyAPIHandler) buildAudioProfileFromInputs(accessToken string, inputs affinityInputs) (audioProfile, string) {
	trackIDs := []string{}
	trackIDs = append(trackIDs, collectTrackIDs(inputs.topTracksShort)...)
	trackIDs = append(trackIDs, collectTrackIDs(inputs.topTracksMedium)...)
	trackIDs = append(trackIDs, collectTrackIDs(inputs.topTracksLong)...)
	trackIDs = uniqueStrings(trackIDs)

	if len(trackIDs) > 50 {
		trackIDs = trackIDs[:50]
	}
	if len(trackIDs) == 0 {
		return audioProfile{}, "audio features unavailable"
	}

	features, err := h.fetchAudioFeaturesFromSpotify(accessToken, trackIDs)
	if err != nil {
		if isSpotifyForbidden(err) {
			return audioProfile{}, "audio features unavailable: spotify returned 403"
		}
		return audioProfile{}, "audio features unavailable"
	}

	profile := buildAudioProfile(features.AudioFeatures)
	if !profile.hasData() {
		return profile, "audio features unavailable"
	}
	return profile, ""
}
