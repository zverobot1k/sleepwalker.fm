package recommendation

import "errors"

// ErrSpotifyRecommendationsUnavailable is returned when Spotify's /recommendations
// endpoint is not available for this application (403/404 since Nov 2024 restrictions).
var ErrSpotifyRecommendationsUnavailable = errors.New("spotify_recommendations_unavailable")
