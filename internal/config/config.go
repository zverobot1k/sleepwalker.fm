package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	BaseURL             string
	FrontendURL         string
	ServerAddr          string
	DatabaseURL         string
	RedisURL            string
	SpotifyClientID     string
	SpotifyClientSecret string
	SpotifyRedirectURI  string
	SpotifyScopes       []string
	SpotifyAuthorizeURL string
	SpotifyTokenURL     string
	SpotifyAPIBaseURL   string
	LastFmAPIKey        string
	LastFmSharedSecret  string
}

func Load() (Config, error) {
	baseURL, err := requireEnv("BASE_URL")
	if err != nil {
		return Config{}, err
	}
	frontendURL, err := requireEnv("FRONTEND_URL")
	if err != nil {
		return Config{}, err
	}
	redirectURI, err := requireEnv("SPOTIFY_REDIRECT_URI")
	if err != nil {
		return Config{}, err
	}

	serverAddr := strings.TrimSpace(os.Getenv("SERVER_ADDR"))
	if serverAddr == "" {
		serverAddr = ":8080"
	}

	scopes := strings.TrimSpace(os.Getenv("SPOTIFY_SCOPES"))
	if scopes == "" {
		scopes = "user-read-email user-read-private user-top-read user-read-recently-played user-library-read user-follow-read playlist-read-private playlist-modify-private playlist-modify-public"
	}

	return Config{
		BaseURL:             strings.TrimRight(baseURL, "/"),
		FrontendURL:         strings.TrimRight(frontendURL, "/"),
		ServerAddr:          serverAddr,
		DatabaseURL:         os.Getenv("DATABASE_URL"),
		RedisURL:            strings.TrimSpace(os.Getenv("REDIS_URL")),
		SpotifyClientID:     os.Getenv("SPOTIFY_CLIENT_ID"),
		SpotifyClientSecret: os.Getenv("SPOTIFY_CLIENT_SECRET"),
		SpotifyRedirectURI:  redirectURI,
		SpotifyScopes:       strings.Fields(scopes),
		SpotifyAuthorizeURL: "https://accounts.spotify.com/authorize",
		SpotifyTokenURL:     "https://accounts.spotify.com/api/token",
		SpotifyAPIBaseURL:   "https://api.spotify.com/v1",
		LastFmAPIKey:        firstEnv("LASTFM_API_KEY", "LAST_FM_API_KEY"),
		LastFmSharedSecret:  firstEnv("LASTFM_SHARED_SECRET", "LAST_FM_SHARED_SECRET"),
	}, nil
}

func requireEnv(key string) (string, error) {
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return "", fmt.Errorf("%s is required (set it in .env — no localhost default)", key)
	}
	return val, nil
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if val := os.Getenv(key); val != "" {
			return val
		}
	}
	return ""
}
