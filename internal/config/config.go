package config

import (
	"os"
	"strings"
)

type Config struct {
	BaseURL             string
	FrontendURL         string
	ServerAddr          string
	DatabaseURL         string
	SpotifyClientID     string
	SpotifyRedirectURI  string
	SpotifyScopes       []string
	SpotifyAuthorizeURL string
	SpotifyTokenURL     string
	SpotifyAPIBaseURL   string
}

func Load() Config {
	baseURL := getenv("BASE_URL", "http://localhost:8080")
	frontendURL := getenv("FRONTEND_URL", "http://localhost:3000")
	serverAddr := getenv("SERVER_ADDR", ":8080")
	scopes := getenv("SPOTIFY_SCOPES", "user-read-email user-read-private user-top-read user-read-recently-played user-library-read user-follow-read playlist-read-private")
	redirectURI := getenv("SPOTIFY_REDIRECT_URI", baseURL+"/auth/spotify/callback")

	return Config{
		BaseURL:             baseURL,
		FrontendURL:         frontendURL,
		ServerAddr:          serverAddr,
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		SpotifyClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		SpotifyRedirectURI:  redirectURI,
		SpotifyScopes:       strings.Fields(scopes),
		SpotifyAuthorizeURL: "https://accounts.spotify.com/authorize",
		SpotifyTokenURL:     "https://accounts.spotify.com/api/token",
		SpotifyAPIBaseURL:   "https://api.spotify.com/v1",
	}
}

func getenv(key, fallback string) string {
	val := os.Getenv(key)
	if val == "" {
		return fallback
	}
	return val
}
