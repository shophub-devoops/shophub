package observability

import (
	"log"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestLogger emits one line per request: method, path, status and duration.
func RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("%s %s -> %d (%s)", c.Request.Method, c.Request.URL.Path, c.Writer.Status(), time.Since(start))
	}
}
