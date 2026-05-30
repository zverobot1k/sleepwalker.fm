package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"sleepwalker.fm/internal/config"
	"sleepwalker.fm/internal/repository/postgres"
	authservice "sleepwalker.fm/internal/service/auth"
	lastfmservice "sleepwalker.fm/internal/service/lastfm"
	"sleepwalker.fm/internal/service/pipeline"
	recommendationservice "sleepwalker.fm/internal/service/recommendation"
	"sleepwalker.fm/internal/service/spotify"
	tokenservice "sleepwalker.fm/internal/service/token"
	transport "sleepwalker.fm/internal/transport/http"
	"sleepwalker.fm/internal/transport/http/handlers"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}
	if cfg.SpotifyClientID == "" {
		log.Fatal("SPOTIFY_CLIENT_ID is required")
	}
	if cfg.DatabaseURL == "" {
		log.Fatal("DATABASE_URL is required")
	}
	log.Printf("config: BASE_URL=%s (api) FRONTEND_URL=%s (ui) SPOTIFY_REDIRECT_URI=%s (spotify→backend)", cfg.BaseURL, cfg.FrontendURL, cfg.SpotifyRedirectURI)
	if cfg.BaseURL == cfg.FrontendURL {
		log.Print("warning: BASE_URL and FRONTEND_URL are identical; post-login redirect may hit the API host instead of the Next.js app")
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

	if err := db.Gorm.AutoMigrate(&postgres.OAuthState{}, &postgres.SpotifyToken{}, &postgres.ArtistMetadata{}); err != nil {
		log.Fatalf("db migration failed: %v", err)
	}

	stateRepo := postgres.NewOAuthStateRepo(db)
	tokenRepo := postgres.NewTokenRepo(db)
	artistMetadataRepo := postgres.NewArtistMetadataRepo(db)

	var redisClient *redis.Client
	if cfg.RedisURL != "" {
		redisClient = redis.NewClient(&redis.Options{Addr: cfg.RedisURL})
		redisCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
		if err := redisClient.Ping(redisCtx).Err(); err != nil {
			log.Printf("warning: redis unavailable: %v", err)
			_ = redisClient.Close()
			redisClient = nil
		}
		cancel()
	}

	httpClient := &http.Client{Timeout: 15 * time.Second}
	oauthService := spotify.NewOAuthService(cfg, httpClient, stateRepo, tokenRepo)
	if cfg.LastFmAPIKey == "" {
		log.Print("warning: LASTFM_API_KEY is not set; Last.fm enrichment will be disabled")
	}
	lastfmClient := lastfmservice.NewClient(cfg.LastFmAPIKey, &http.Client{Timeout: 10 * time.Second})

	tokenSvc := tokenservice.NewService(tokenRepo, oauthService, redisClient)
	spotifyAPI := spotify.NewAPIService(cfg, httpClient, tokenSvc, redisClient)
	lastfmSvc := lastfmservice.NewService(lastfmClient)
	pipe := pipeline.New(spotifyAPI, lastfmSvc)
	recSvc := recommendationservice.NewService(spotifyAPI, lastfmSvc)
	authSvc := authservice.New(oauthService, tokenSvc)

	oauthHandler := handlers.NewOAuthHandler(authSvc, cfg.FrontendURL)
	healthHandler := handlers.NewHealthHandler()
	spotifyAPIHandler := handlers.NewSpotifyAPIHandlerFull(
		tokenRepo,
		artistMetadataRepo,
		lastfmClient,
		oauthService,
		tokenSvc,
		spotifyAPI,
		pipe,
		recSvc,
		lastfmSvc,
		redisClient,
	)
	if redisClient != nil {
		defer redisClient.Close()
	}

	corsOrigins := []string{
		cfg.FrontendURL,
		"http://localhost:3000",
		"http://127.0.0.1:3000",
	}
	if cfg.BaseURL != cfg.FrontendURL {
		corsOrigins = append(corsOrigins, cfg.BaseURL)
	}
	log.Printf("config: CORS allowed origins: %v", corsOrigins)

	router := transport.NewRouter(transport.RouterDeps{
		OAuth:      oauthHandler,
		Health:     healthHandler,
		SpotifyAPI: spotifyAPIHandler,
	}, corsOrigins)

	log.Printf("listening on %s", cfg.ServerAddr)
	if err := router.Run(cfg.ServerAddr); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
