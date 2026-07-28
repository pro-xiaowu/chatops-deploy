package middleware

import (
	"net/http"
	"strings"

	"chatops-deploy/internal/auth"
	"chatops-deploy/internal/domain"
	"github.com/gin-gonic/gin"
)

const PrincipalKey = "chatops.principal"

func RequireToken(tokens *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "Bearer token required"}})
			return
		}
		principal, err := tokens.Verify(c.Request.Context(), strings.TrimPrefix(header, "Bearer "))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "invalid token"}})
			return
		}
		c.Set(PrincipalKey, principal)
		c.Next()
	}
}
func RequireAuth(tokens *auth.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		var principal domain.Principal
		var err error
		if strings.HasPrefix(header, "Bearer ") {
			principal, err = tokens.Verify(c.Request.Context(), strings.TrimPrefix(header, "Bearer "))
		} else {
			session, _ := c.Cookie("chatops_session")
			csrf := c.GetHeader("X-CSRF-Token")
			unsafe := c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions
			principal, err = tokens.VerifySession(c.Request.Context(), session, csrf, unsafe)
		}
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"code": "unauthorized", "message": "authentication required"}})
			return
		}
		c.Set(PrincipalKey, principal)
		c.Next()
	}
}
func Principal(c *gin.Context) (domain.Principal, bool) {
	v, ok := c.Get(PrincipalKey)
	if !ok {
		return domain.Principal{}, false
	}
	p, ok := v.(domain.Principal)
	return p, ok
}
