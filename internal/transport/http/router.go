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

// SimpleCORSMiddleware handles CORS preflight requests
func SimpleCORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE, PATCH")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func NewRouter(deps RouterDeps) *gin.Engine {
	r := gin.New()
	r.Use(gin.Logger(), gin.Recovery())
	r.Use(SimpleCORSMiddleware())

	r.GET("/health", deps.Health.Ping)

	auth := r.Group("/auth/spotify")
	{
		auth.GET("/login", deps.OAuth.Login)
		auth.GET("/callback", deps.OAuth.Callback)
		auth.POST("/refresh/:userId", deps.OAuth.Refresh)
	}

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
