package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"sleepwalker.fm/internal/repository/postgres"
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

	maxRelatedArtistLookups = 4
	artistMetadataTTL       = 30 * 24 * time.Hour
	artistMetadataRefresh   = 30 * time.Minute
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

type artistMetadataEntry struct {
	artistID       string
	genres         []string
	inferredGenres []string
	relatedArtists []string
	source         string
	expiresAt      time.Time
	updatedAt      time.Time
}

type artistMetadataCache struct {
	mu    sync.Mutex
	ttl   time.Duration
	items map[string]artistMetadataEntry
}

func newArtistCache(ttl time.Duration) *artistCache {
	return &artistCache{ttl: ttl, items: make(map[string]artistCacheEntry)}
}

func (c *artistCache) get(id string) (ArtistItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[id]
	if !ok {
		log.Printf("DIAG cache artistCache MISS id=%s", id)
		return ArtistItem{}, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.items, id)
		log.Printf("DIAG cache artistCache EXPIRED id=%s", id)
		return ArtistItem{}, false
	}
	log.Printf("DIAG cache artistCache HIT id=%s", id)
	return entry.artist, true
}

func (c *artistCache) set(id string, artist ArtistItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[id] = artistCacheEntry{artist: artist, expiresAt: time.Now().Add(c.ttl)}
	log.Printf("DIAG cache artistCache SET id=%s expires_in=%s", id, c.ttl)
}

func newRelatedArtistsCache(ttl time.Duration) *relatedArtistsCache {
	return &relatedArtistsCache{ttl: ttl, items: make(map[string]relatedArtistsCacheEntry)}
}

func (c *relatedArtistsCache) get(id string) ([]ArtistItem, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[id]
	if !ok {
		log.Printf("DIAG cache relatedArtistsCache MISS id=%s", id)
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.items, id)
		log.Printf("DIAG cache relatedArtistsCache EXPIRED id=%s", id)
		return nil, false
	}
	log.Printf("DIAG cache relatedArtistsCache HIT id=%s count=%d", id, len(entry.artists))
	return entry.artists, true
}

func (c *relatedArtistsCache) set(id string, artists []ArtistItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[id] = relatedArtistsCacheEntry{artists: artists, expiresAt: time.Now().Add(c.ttl)}
	log.Printf("DIAG cache relatedArtistsCache SET id=%s count=%d expires_in=%s", id, len(artists), c.ttl)
}

func newArtistMetadataCache(ttl time.Duration) *artistMetadataCache {
	return &artistMetadataCache{ttl: ttl, items: make(map[string]artistMetadataEntry)}
}

func (c *artistMetadataCache) get(id string) (artistMetadataEntry, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.items[id]
	if !ok {
		log.Printf("DIAG cache artistMetadataCache MISS id=%s", id)
		return artistMetadataEntry{}, false
	}
	if time.Now().After(entry.expiresAt) {
		delete(c.items, id)
		log.Printf("DIAG cache artistMetadataCache EXPIRED id=%s", id)
		return artistMetadataEntry{}, false
	}
	log.Printf("DIAG cache artistMetadataCache HIT id=%s source=%s", id, entry.source)
	return entry, true
}

func (c *artistMetadataCache) set(entry artistMetadataEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry.expiresAt = time.Now().Add(c.ttl)
	c.items[entry.artistID] = entry
	log.Printf("DIAG cache artistMetadataCache SET id=%s source=%s expires_in=%s", entry.artistID, entry.source, c.ttl)
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

func inferGenresFromTagCounts(tags []string, topN int) []string {
	if len(tags) == 0 || topN <= 0 {
		return []string{}
	}
	counts := map[string]int{}
	for _, tag := range tags {
		normalized := normalizeGenreValue(tag)
		if normalized == "" || isClearlyInvalidGenre(normalized) {
			continue
		}
		counts[normalized]++
	}
	type genreCount struct {
		name  string
		count int
	}
	items := make([]genreCount, 0, len(counts))
	for name, count := range counts {
		items = append(items, genreCount{name: name, count: count})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].count == items[j].count {
			return items[i].name < items[j].name
		}
		return items[i].count > items[j].count
	})
	if len(items) > topN {
		items = items[:topN]
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		result = append(result, item.name)
	}
	return result
}

func safeStrings(values []string) []string {
	return uniqueNonEmptyStrings(values)
}

func uniqueNonEmptyStrings(values []string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func mergeWarnings(parts ...string) string {
	return compactWarnings(parts)
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

func (h *SpotifyAPIHandler) getArtistMetadataCached(ctx context.Context, artistID string, artistName string, accessToken string) (artistMetadataEntry, error) {
	if artistID == "" {
		return artistMetadataEntry{}, fmt.Errorf("missing artist id")
	}
	if h.artistMetadataCache != nil {
		if cached, ok := h.artistMetadataCache.get(artistID); ok {
			return cached, nil
		}
	}
	if h.artistMetadataRepo != nil {
		if record, ok, err := h.artistMetadataRepo.GetByArtistID(ctx, artistID); err == nil && ok {
			entry := artistMetadataEntry{
				artistID:       record.ArtistID,
				genres:         uniqueNonEmptyStrings(record.Genres),
				inferredGenres: uniqueNonEmptyStrings(record.InferredGenres),
				relatedArtists: uniqueNonEmptyStrings(record.RelatedArtists),
				source:         record.Source,
				updatedAt:      record.UpdatedAt,
			}
			if h.artistMetadataCache != nil {
				h.artistMetadataCache.set(entry)
			}
			return entry, nil
		}
	}

	entry, err := h.refreshArtistMetadata(ctx, artistID, artistName, accessToken)
	if err != nil {
		return artistMetadataEntry{}, err
	}
	if h.artistMetadataCache != nil {
		h.artistMetadataCache.set(entry)
	}
	return entry, nil
}

func (h *SpotifyAPIHandler) refreshArtistMetadata(ctx context.Context, artistID string, artistName string, accessToken string) (artistMetadataEntry, error) {
	entry := artistMetadataEntry{artistID: artistID, source: "cache_miss", updatedAt: time.Now()}
	artist, err := h.fetchArtistByID(nil, "", accessToken, artistID)
	if err != nil {
		return entry, err
	}
	entry.genres = normalizeGenres(artist.Genres)
	entry.source = "spotify_single_artist"
	if len(entry.genres) == 0 && h.lastfmClient != nil && h.lastfmClient.Enabled() {
		lastfmGenres, lastfmErr := h.lastfmClient.GetArtistTopTags(ctx, chooseArtistName(artistName, artist.Name), 10)
		if lastfmErr == nil {
			entry.genres = normalizeGenres(lastfmGenres)
			entry.source = "lastfm_top_tags"
		} else {
			entry.source = "spotify_single_artist"
		}
	}
	if len(entry.genres) == 0 && h.lastfmClient != nil && h.lastfmClient.Enabled() {
		similar, similarErr := h.lastfmClient.GetArtistSimilar(ctx, chooseArtistName(artistName, artist.Name), 10)
		if similarErr == nil {
			similarNames := make([]string, 0, len(similar))
			for _, item := range similar {
				similarNames = append(similarNames, item.Name)
			}
			entry.relatedArtists = safeStrings(similarNames)
			tags := make([]string, 0, len(similar)*3)
			for _, item := range similar {
				name := strings.TrimSpace(item.Name)
				if name == "" {
					continue
				}
				artistTags, tagErr := h.lastfmClient.GetArtistTopTags(ctx, name, 3)
				if tagErr != nil {
					continue
				}
				tags = append(tags, artistTags...)
			}
			entry.inferredGenres = inferGenresFromTagCounts(tags, 5)
			if len(entry.inferredGenres) > 0 {
				entry.source = "lastfm_similar_artists"
			}
		}
	}
	if len(entry.inferredGenres) == 0 && len(entry.relatedArtists) > 0 {
		entry.inferredGenres = []string{}
	}
	entry.genres = normalizeGenres(entry.genres)
	entry.inferredGenres = normalizeGenres(entry.inferredGenres)
	if h.artistMetadataRepo != nil {
		_ = h.artistMetadataRepo.Upsert(ctx, postgres.ArtistMetadataRecord{
			ArtistID:       entry.artistID,
			Genres:         entry.genres,
			InferredGenres: entry.inferredGenres,
			RelatedArtists: entry.relatedArtists,
			Source:         entry.source,
			UpdatedAt:      time.Now(),
		})
	}
	return entry, nil
}

func chooseArtistName(preferred string, fallback string) string {
	preferred = strings.TrimSpace(preferred)
	if preferred != "" {
		return preferred
	}
	return strings.TrimSpace(fallback)
}

func (h *SpotifyAPIHandler) enrichTopArtistsAsync(accessToken string, items []ArtistItem) {
	if h == nil || len(items) == 0 {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		for _, item := range items {
			if item.ID == "" {
				continue
			}
			metadata, err := h.getArtistMetadataCached(ctx, item.ID, item.Name, accessToken)
			if err != nil {
				continue
			}
			_ = metadata
		}
	}()
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
	for _, id := range missing {
		artist, err := h.fetchArtistByID(nil, "", accessToken, id)
		if err != nil {
			if fetchErr == nil {
				fetchErr = err
			}
			continue
		}
		result[id] = artist
		if h.artistCache != nil {
			h.artistCache.set(id, artist)
		}
	}

	if len(result) > 0 {
		return result, nil
	}

	return result, fetchErr
}

func (h *SpotifyAPIHandler) getRelatedArtistsCached(userID, accessToken string, artistID string) ([]ArtistItem, error) {
	if artistID == "" {
		return []ArtistItem{}, nil
	}
	if h.relatedArtistsCache != nil {
		if cached, ok := h.relatedArtistsCache.get(artistID); ok {
			return cached, nil
		}
	}

	artists, err := h.fetchRelatedArtistsFromSpotify(context.Background(), userID, accessToken, artistID)
	if err != nil {
		return nil, err
	}
	if h.relatedArtistsCache != nil {
		h.relatedArtistsCache.set(artistID, artists)
	}
	return artists, nil
}

func (h *SpotifyAPIHandler) fetchRelatedArtistsFromSpotify(ctx context.Context, userID, accessToken string, artistID string) ([]ArtistItem, error) {
	body, err := h.doSpotifyGETWithAppFallback(ctx, userID, accessToken, "https://api.spotify.com/v1/artists/"+artistID+"/related-artists")
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

func (h *SpotifyAPIHandler) expandSeedArtistsWithRelated(userID, accessToken string, seedArtists []string, ranked []string, max int) ([]string, []string) {
	warnings := []string{}
	if len(seedArtists) >= max {
		return seedArtists, warnings
	}

	candidates := pickFirstIDs(ranked, maxRelatedArtistLookups)
	for _, candidateID := range candidates {
		if len(seedArtists) >= max {
			break
		}
		related, err := h.getRelatedArtistsCached(userID, accessToken, candidateID)
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
		warnings = append(warnings, "artist metadata lookup degraded")
	}

	artists := make([]ArtistItem, 0, len(sampleIDs))
	for _, id := range sampleIDs {
		artist, ok := artistDetails[id]
		if !ok {
			continue
		}
		metadata, metaErr := h.getArtistMetadataCached(context.Background(), id, artist.Name, accessToken)
		if metaErr == nil {
			if len(metadata.genres) > 0 {
				artist.Genres = metadata.genres
			} else if len(metadata.inferredGenres) > 0 {
				artist.Genres = metadata.inferredGenres
			}
			if len(metadata.relatedArtists) > 0 {
				warnings = append(warnings, "related artists available for genre inference")
			}
		} else if len(artist.Genres) == 0 {
			warnings = append(warnings, "artist metadata lookup failed")
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
	if artistID == "" {
		return []string{}, "artist id missing"
	}
	if h.artistMetadataRepo != nil {
		if record, ok, err := h.artistMetadataRepo.GetByArtistID(context.Background(), artistID); err == nil && ok && len(record.InferredGenres) > 0 {
			return pickFirstIDs(record.InferredGenres, topN), ""
		}
	}
	return []string{}, "related artists unavailable"
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

	features, err := h.fetchAudioFeaturesFromSpotify(context.Background(), "", accessToken, trackIDs)
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
