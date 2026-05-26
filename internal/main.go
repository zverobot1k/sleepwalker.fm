package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"sleepwalker.fm/internal/config"
	"sleepwalker.fm/internal/repository/postgres"
	"sleepwalker.fm/internal/service/spotify"
	transport "sleepwalker.fm/internal/transport/http"
	"sleepwalker.fm/internal/transport/http/handlers"
)

func main() {
	cfg := config.Load()
	if cfg.SpotifyClientID == "" {
		log.Fatal("SPOTIFY_CLIENT_ID is required")
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx := context.Background()
	db, err := postgres.New(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("db init failed: %v", err)
	}
	sqlDB, err := db.Gorm.DB()
	if err != nil {
		log.Fatalf("db handle failed: %v", err)
	}
	defer sqlDB.Close()

	if err := db.Gorm.AutoMigrate(&postgres.OAuthState{}, &postgres.SpotifyToken{}); err != nil {
		log.Fatalf("db migration failed: %v", err)
	}

	stateRepo := postgres.NewOAuthStateRepo(db)
	tokenRepo := postgres.NewTokenRepo(db)

	httpClient := &http.Client{Timeout: 15 * time.Second}
	oauthService := spotify.NewOAuthService(cfg, httpClient, stateRepo, tokenRepo)

	oauthHandler := handlers.NewOAuthHandler(oauthService, cfg.FrontendURL)
	healthHandler := handlers.NewHealthHandler()
	spotifyAPIHandler := handlers.NewSpotifyAPIHandler(tokenRepo)

	router := transport.NewRouter(transport.RouterDeps{
		OAuth:      oauthHandler,
		Health:     healthHandler,
		SpotifyAPI: spotifyAPIHandler,
	})

	log.Printf("listening on %s", cfg.ServerAddr)
	if err := router.Run(cfg.ServerAddr); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
