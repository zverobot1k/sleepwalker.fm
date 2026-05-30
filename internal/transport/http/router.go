package http

import (
	"github.com/gin-gonic/gin"
	"sleepwalker.fm/internal/transport/http/handlers"
)

type RouterDeps struct {
	OAuth      *handlers.OAuthHandler
	Health     *handlers.HealthHandler
	SpotifyAPI *handlers.SpotifyAPIHandler
}

func NewRouter(deps RouterDeps, allowedOrigins []string) *gin.Engine {
	r := gin.New()
	r.Use(SpotifyAuditMiddleware())
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(CORSMiddleware(allowedOrigins))

	r.GET("/health", deps.Health.Ping)

	auth := r.Group("/auth/spotify")
	{
		auth.GET("/login", deps.OAuth.Login)
		auth.GET("/callback", deps.OAuth.Callback)
		auth.POST("/refresh/:userId", deps.OAuth.Refresh)
	}
	r.POST("/auth/refresh/:userId", deps.OAuth.Refresh)
	r.GET("/auth/session/:userId", deps.OAuth.Session)

	api := r.Group("/api/spotify")
	{
		api.GET("/top/artists/:userId", deps.SpotifyAPI.GetTopArtists)
		api.GET("/top/tracks/:userId", deps.SpotifyAPI.GetTopTracks)
		api.GET("/recently-played/:userId", deps.SpotifyAPI.GetRecentlyPlayed)
		api.GET("/audio-features/:userId", deps.SpotifyAPI.GetAudioFeatures)
	}

	wrapped := r.Group("/api/wrapped")
	{
		wrapped.GET("/summary/:userId", deps.SpotifyAPI.GetWrappedSummary)
		wrapped.GET("/insights/:userId", deps.SpotifyAPI.GetWrappedInsights)
		wrapped.GET("/timeline/:userId", deps.SpotifyAPI.GetWrappedTimeline)
		wrapped.GET("/compare/:userId", deps.SpotifyAPI.GetWrappedCompare)
	}

	stats := r.Group("/api/stats")
	{
		stats.GET("/profile/:userId", deps.SpotifyAPI.GetStatsProfile)
		stats.GET("/genres/:userId", deps.SpotifyAPI.GetStatsGenres)
		stats.GET("/listening-time/:userId", deps.SpotifyAPI.GetStatsListeningTime)
	}

	recommendations := r.Group("/api/recommendations")
	{
		recommendations.GET("/:userId", deps.SpotifyAPI.GetRecommendations)
		recommendations.POST("/playlist/:userId", deps.SpotifyAPI.CreateRecommendationsPlaylist)
	}

	return r
}
