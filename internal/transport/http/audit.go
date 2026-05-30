package http

import (
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

type endpointCounter struct {
	count atomic.Int64
}

var auditCounters sync.Map

func SpotifyAuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		fullPath := c.FullPath()
		if fullPath == "" {
			fullPath = c.Request.URL.Path
		}
		if !isAuditedPath(fullPath) {
			return
		}

		userID := c.Param("userId")
		if userID == "" {
			userID = "-"
		}
		page := c.GetHeader("X-SWFM-Page")
		if page == "" {
			page = inferPageFromPath(fullPath)
		}

		key := userID + "|" + page + "|" + fullPath
		counterAny, _ := auditCounters.LoadOrStore(key, &endpointCounter{})
		counter := counterAny.(*endpointCounter)
		count := counter.count.Add(1)

		log.Printf("[%s] %s %s count=%d page=%s", userID, fullPath, time.Now().Format(time.RFC3339Nano), count, page)
	}
}

func isAuditedPath(path string) bool {
	return strings.HasPrefix(path, "/api/spotify/") || strings.HasPrefix(path, "/api/wrapped/") || strings.HasPrefix(path, "/api/stats/") || strings.HasPrefix(path, "/api/recommendations/")
}

func inferPageFromPath(path string) string {
	switch {
	case strings.HasPrefix(path, "/api/wrapped/"):
		return "wrapped"
	case strings.HasPrefix(path, "/api/recommendations/"):
		return "recommendations"
	case strings.HasPrefix(path, "/api/spotify/") || strings.HasPrefix(path, "/api/stats/"):
		return "dashboard"
	default:
		return "unknown"
	}
}
