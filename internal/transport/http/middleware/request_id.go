package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"regexp"
)

var requestIDPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]{8,128}$`)

func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if !requestIDPattern.MatchString(id) {
			id = uuid.NewString()
		}
		c.Header("X-Request-ID", id)
		c.Set("request_id", id)
		c.Next()
	}
}
