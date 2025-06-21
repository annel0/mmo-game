package middleware

import (
	"time"

	"github.com/annel0/mmo-game/internal/logging"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/trace"
)

// RequestLogger снабжает каждый HTTP-запрос trace-ID и пишет краткие логи.
// Использует глобальный logging пакет (Info/Debug).

type RequestLogger struct{}

func NewRequestLogger() *RequestLogger { return &RequestLogger{} }

func (rl *RequestLogger) Handler() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Пытаемся извлечь trace-id из OpenTelemetry, если уже создан.
		span := trace.SpanFromContext(c.Request.Context())
		var traceID string
		if span.SpanContext().IsValid() {
			traceID = span.SpanContext().TraceID().String()
		} else {
			traceID = uuid.NewString()
		}
		c.Set("trace_id", traceID)

		start := time.Now()
		method := c.Request.Method
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}
		clientIP := c.ClientIP()

		logging.Info("[HTTP] ▶ %s %s ip=%s trace=%s", method, path, clientIP, traceID)

		c.Next()

		status := c.Writer.Status()
		latency := time.Since(start)
		logging.Info("[HTTP] ◀ %s %s %d %s trace=%s", method, path, status, latency, traceID)
	}
}
