package http

import "github.com/gin-gonic/gin"

type Envelope struct {
	Data  any       `json:"data,omitempty"`
	Error *APIError `json:"error,omitempty"`
}

type APIError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id,omitempty"`
}

func respondData(c *gin.Context, status int, data any) {
	c.JSON(status, Envelope{Data: data})
}

func respondError(c *gin.Context, status int, code, message string) {
	c.JSON(status, Envelope{Error: &APIError{
		Code:      code,
		Message:   message,
		RequestID: c.GetHeader("X-Request-ID"),
	}})
}
