package httpserver

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

// Envelope 统一 API 响应信封。所有 HTTP 响应均通过该结构包装。
type Envelope struct {
	OK        bool      `json:"ok"`
	Data      any       `json:"data,omitempty"`
	Error     *APIError `json:"error,omitempty"`
	Timestamp string    `json:"timestamp"`
}

// APIError 机器可读 API 错误。
type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// WriteOK 写入 200 OK 响应信封。
func WriteOK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{
		OK:        true,
		Data:      data,
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// WriteError 写入错误响应信封并中止请求处理链。
func WriteError(c *gin.Context, status int, code string, message string) {
	c.AbortWithStatusJSON(status, Envelope{
		OK:        false,
		Error:     &APIError{Code: code, Message: message},
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// WriteNotFound 便捷方法：写入 404 响应。
func WriteNotFound(c *gin.Context) {
	WriteError(c, http.StatusNotFound, "NOT_FOUND", "resource not found")
}
