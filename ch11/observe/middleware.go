package observe

import (
	"github.com/gin-gonic/gin"
	"time"
)

func Middleware(metrics *Metrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		started := time.Now()
		metrics.InFlight.Inc()
		defer metrics.InFlight.Dec()
		c.Next()
		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := statusCode(c.Writer.Status())
		metrics.Requests.WithLabelValues(c.Request.Method, route, status).Inc()
		metrics.Duration.WithLabelValues(c.Request.Method, route, status).Observe(time.Since(started).Seconds())
	}
}
