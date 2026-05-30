package lastfm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

type TopTag struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

type SimilarArtist struct {
	Name  string `json:"name"`
	Match string `json:"match"`
}

func NewClient(apiKey string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 10 * time.Second}
	}
	return &Client{apiKey: apiKey, httpClient: httpClient, baseURL: "https://ws.audioscrobbler.com/2.0/"}
}

func (c *Client) Enabled() bool {
	return c != nil && strings.TrimSpace(c.apiKey) != ""
}

func (c *Client) GetArtistTopTags(ctx context.Context, artist string, limit int) ([]string, error) {
	if !c.Enabled() {
		return []string{}, nil
	}
	if limit <= 0 {
		limit = 5
	}
	values := url.Values{}
	values.Set("method", "artist.gettoptags")
	values.Set("artist", artist)
	values.Set("api_key", c.apiKey)
	values.Set("format", "json")
	values.Set("autocorrect", "1")

	body, err := c.do(ctx, values)
	if err != nil {
		return nil, err
	}

	var payload struct {
		TopTags struct {
			Tag json.RawMessage `json:"tag"`
		} `json:"toptags"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("lastfm artist.gettoptags parse error for %q: %v body=%s", artist, err, truncate(body, 200))
		return nil, fmt.Errorf("lastfm parse error: %w", err)
	}
	if payload.Error != 0 {
		return nil, fmt.Errorf("lastfm api error %d: %s", payload.Error, payload.Message)
	}

	tags, err := parseTagList(payload.TopTags.Tag)
	if err != nil {
		log.Printf("lastfm artist.gettoptags tag list error for %q: %v", artist, err)
		return nil, err
	}

	genres := make([]string, 0, len(tags))
	for _, tag := range tags {
		name := strings.TrimSpace(tag.Name)
		if name == "" {
			continue
		}
		genres = append(genres, name)
		if len(genres) >= limit {
			break
		}
	}
	return genres, nil
}

func (c *Client) GetArtistSimilar(ctx context.Context, artist string, limit int) ([]SimilarArtist, error) {
	if !c.Enabled() {
		return []SimilarArtist{}, nil
	}
	if limit <= 0 {
		limit = 5
	}
	values := url.Values{}
	values.Set("method", "artist.getsimilar")
	values.Set("artist", artist)
	values.Set("api_key", c.apiKey)
	values.Set("format", "json")
	values.Set("autocorrect", "1")
	values.Set("limit", fmt.Sprintf("%d", limit))

	body, err := c.do(ctx, values)
	if err != nil {
		return nil, err
	}

	var payload struct {
		SimilarArtists struct {
			Artist json.RawMessage `json:"artist"`
		} `json:"similarartists"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("lastfm artist.getsimilar parse error for %q: %v", artist, err)
		return nil, fmt.Errorf("lastfm parse error: %w", err)
	}
	if payload.Error != 0 {
		return nil, fmt.Errorf("lastfm api error %d: %s", payload.Error, payload.Message)
	}

	artists, err := parseSimilarArtists(payload.SimilarArtists.Artist)
	if err != nil {
		return nil, err
	}

	result := make([]SimilarArtist, 0, len(artists))
	for _, artistItem := range artists {
		if strings.TrimSpace(artistItem.Name) == "" {
			continue
		}
		result = append(result, artistItem)
	}
	return result, nil
}

// GetTagTopArtists returns top artist names for a Last.fm tag.
func (c *Client) GetTagTopArtists(ctx context.Context, tag string, limit int) ([]string, error) {
	if !c.Enabled() {
		return []string{}, nil
	}
	if limit <= 0 {
		limit = 30
	}
	values := url.Values{}
	values.Set("method", "tag.gettopartists")
	values.Set("tag", tag)
	values.Set("api_key", c.apiKey)
	values.Set("format", "json")
	values.Set("limit", fmt.Sprintf("%d", limit))

	body, err := c.do(ctx, values)
	if err != nil {
		return nil, err
	}

	var payload struct {
		TopArtists struct {
			Artist json.RawMessage `json:"artist"`
		} `json:"topartists"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Error != 0 {
		return nil, fmt.Errorf("lastfm api error %d: %s", payload.Error, payload.Message)
	}

	// parse artist list (can be single or array)
	var artists []struct {
		Name string `json:"name"`
	}
	if len(payload.TopArtists.Artist) == 0 || string(payload.TopArtists.Artist) == "null" {
		return []string{}, nil
	}
	if payload.TopArtists.Artist[0] == '[' {
		if err := json.Unmarshal(payload.TopArtists.Artist, &artists); err != nil {
			return nil, err
		}
	} else {
		var single struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(payload.TopArtists.Artist, &single); err != nil {
			return nil, err
		}
		artists = []struct {
			Name string `json:"name"`
		}{single}
	}

	out := make([]string, 0, len(artists))
	for _, a := range artists {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		out = append(out, name)
	}
	return out, nil
}

// GetTagTopTracks returns top tracks (artist, name) for a Last.fm tag.
func (c *Client) GetTagTopTracks(ctx context.Context, tag string, limit int) ([]struct{ Artist, Title string }, error) {
	if !c.Enabled() {
		return []struct{ Artist, Title string }{}, nil
	}
	if limit <= 0 {
		limit = 40
	}
	values := url.Values{}
	values.Set("method", "tag.gettoptracks")
	values.Set("tag", tag)
	values.Set("api_key", c.apiKey)
	values.Set("format", "json")
	values.Set("limit", fmt.Sprintf("%d", limit))

	body, err := c.do(ctx, values)
	if err != nil {
		return nil, err
	}

	var payload struct {
		Tracks struct {
			Track json.RawMessage `json:"track"`
		} `json:"tracks"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Error != 0 {
		return nil, fmt.Errorf("lastfm api error %d: %s", payload.Error, payload.Message)
	}

	type tstruct struct {
		Name   string `json:"name"`
		Artist struct {
			Name string `json:"name"`
		} `json:"artist"`
	}

	var tracks []tstruct
	if len(payload.Tracks.Track) == 0 || string(payload.Tracks.Track) == "null" {
		return []struct{ Artist, Title string }{}, nil
	}
	if payload.Tracks.Track[0] == '[' {
		if err := json.Unmarshal(payload.Tracks.Track, &tracks); err != nil {
			return nil, err
		}
	} else {
		var single tstruct
		if err := json.Unmarshal(payload.Tracks.Track, &single); err != nil {
			return nil, err
		}
		tracks = []tstruct{single}
	}

	out := make([]struct{ Artist, Title string }, 0, len(tracks))
	for _, tr := range tracks {
		artist := strings.TrimSpace(tr.Artist.Name)
		title := strings.TrimSpace(tr.Name)
		if artist == "" || title == "" {
			continue
		}
		out = append(out, struct{ Artist, Title string }{Artist: artist, Title: title})
	}
	return out, nil
}

// GetArtistTopTracks returns top tracks for an artist.
func (c *Client) GetArtistTopTracks(ctx context.Context, artist string, limit int) ([]struct{ Artist, Title string }, error) {
	if !c.Enabled() {
		return []struct{ Artist, Title string }{}, nil
	}
	if limit <= 0 {
		limit = 10
	}
	values := url.Values{}
	values.Set("method", "artist.gettoptracks")
	values.Set("artist", artist)
	values.Set("api_key", c.apiKey)
	values.Set("format", "json")
	values.Set("limit", fmt.Sprintf("%d", limit))

	body, err := c.do(ctx, values)
	if err != nil {
		return nil, err
	}

	var payload struct {
		TopTracks struct {
			Track json.RawMessage `json:"track"`
		} `json:"toptracks"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Error != 0 {
		return nil, fmt.Errorf("lastfm api error %d: %s", payload.Error, payload.Message)
	}

	type tstruct struct {
		Name   string `json:"name"`
		Artist struct {
			Name string `json:"name"`
		} `json:"artist"`
	}
	var tracks []tstruct
	if len(payload.TopTracks.Track) == 0 || string(payload.TopTracks.Track) == "null" {
		return []struct{ Artist, Title string }{}, nil
	}
	if payload.TopTracks.Track[0] == '[' {
		if err := json.Unmarshal(payload.TopTracks.Track, &tracks); err != nil {
			return nil, err
		}
	} else {
		var single tstruct
		if err := json.Unmarshal(payload.TopTracks.Track, &single); err != nil {
			return nil, err
		}
		tracks = []tstruct{single}
	}

	out := make([]struct{ Artist, Title string }, 0, len(tracks))
	for _, tr := range tracks {
		artist := strings.TrimSpace(tr.Artist.Name)
		title := strings.TrimSpace(tr.Name)
		if artist == "" || title == "" {
			continue
		}
		out = append(out, struct{ Artist, Title string }{Artist: artist, Title: title})
	}
	return out, nil
}

// GetTrackSimilar returns similar tracks (artist,title) for a track.
func (c *Client) GetTrackSimilar(ctx context.Context, artist, track string, limit int) ([]struct{ Artist, Title string }, error) {
	if !c.Enabled() {
		return []struct{ Artist, Title string }{}, nil
	}
	if limit <= 0 {
		limit = 12
	}
	values := url.Values{}
	values.Set("method", "track.getsimilar")
	values.Set("artist", artist)
	values.Set("track", track)
	values.Set("api_key", c.apiKey)
	values.Set("format", "json")
	values.Set("limit", fmt.Sprintf("%d", limit))

	body, err := c.do(ctx, values)
	if err != nil {
		return nil, err
	}

	var payload struct {
		SimilarTracks struct {
			Track json.RawMessage `json:"track"`
		} `json:"similartracks"`
		Error   int    `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if payload.Error != 0 {
		return nil, fmt.Errorf("lastfm api error %d: %s", payload.Error, payload.Message)
	}

	type tstruct struct {
		Name   string `json:"name"`
		Artist struct {
			Name string `json:"name"`
		} `json:"artist"`
	}
	var tracks []tstruct
	if len(payload.SimilarTracks.Track) == 0 || string(payload.SimilarTracks.Track) == "null" {
		return []struct{ Artist, Title string }{}, nil
	}
	if payload.SimilarTracks.Track[0] == '[' {
		if err := json.Unmarshal(payload.SimilarTracks.Track, &tracks); err != nil {
			return nil, err
		}
	} else {
		var single tstruct
		if err := json.Unmarshal(payload.SimilarTracks.Track, &single); err != nil {
			return nil, err
		}
		tracks = []tstruct{single}
	}

	out := make([]struct{ Artist, Title string }, 0, len(tracks))
	for _, tr := range tracks {
		artist := strings.TrimSpace(tr.Artist.Name)
		title := strings.TrimSpace(tr.Name)
		if artist == "" || title == "" {
			continue
		}
		out = append(out, struct{ Artist, Title string }{Artist: artist, Title: title})
	}
	return out, nil
}

func parseTagList(raw json.RawMessage) ([]TopTag, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []TopTag{}, nil
	}
	if raw[0] == '[' {
		var tags []TopTag
		if err := json.Unmarshal(raw, &tags); err != nil {
			return nil, err
		}
		return tags, nil
	}
	var single TopTag
	if err := json.Unmarshal(raw, &single); err != nil {
		return nil, err
	}
	return []TopTag{single}, nil
}

func parseSimilarArtists(raw json.RawMessage) ([]SimilarArtist, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []SimilarArtist{}, nil
	}
	if raw[0] == '[' {
		var artists []SimilarArtist
		if err := json.Unmarshal(raw, &artists); err != nil {
			return nil, err
		}
		return artists, nil
	}
	var single SimilarArtist
	if err := json.Unmarshal(raw, &single); err != nil {
		return nil, err
	}
	return []SimilarArtist{single}, nil
}

func (c *Client) do(ctx context.Context, values url.Values) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"?"+values.Encode(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		log.Printf("DIAG lastfm request error: url=%s err=%v", req.URL.String(), err)
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("DIAG lastfm read body error: url=%s err=%v", req.URL.String(), err)
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		log.Printf("DIAG lastfm non-2xx: url=%s status=%d body=%s", req.URL.String(), resp.StatusCode, strings.TrimSpace(string(body)))
		return nil, fmt.Errorf("lastfm http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func truncate(b []byte, n int) string {
	if len(b) <= n {
		return string(b)
	}
	return string(b[:n]) + "..."
}
