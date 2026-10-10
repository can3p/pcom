package logging

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Middleware gives each request a logger derived from base with the request's
// ID, and logs the request once it is served: at error level when it ended
// with a 5xx status, at info level otherwise. Install it first, so recovery
// and every handler log through the request's logger.
func Middleware(base *zap.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		WithLogger(c, base.With(zap.String("request_id", uuid.NewString())))

		c.Next()

		fields := []zap.Field{
			zap.String("method", c.Request.Method),
			zap.String("path", c.Request.URL.Path),
			zap.Int("status", c.Writer.Status()),
			zap.Duration("latency", time.Since(start)),
			zap.String("client_ip", c.ClientIP()),
		}
		if len(c.Errors) > 0 {
			fields = append(fields, zap.Strings("errors", c.Errors.Errors()))
		}

		// FromContext, not the logger set above: middleware after this one
		// may have added fields, such as the user ID.
		logger := FromContext(c)
		if c.Writer.Status() >= 500 {
			logger.Error("request", fields...)
		} else {
			logger.Info("request", fields...)
		}
	}
}

// Recovery recovers a panicking request and logs the panic with its stack
// through the request's logger, then calls handle, which writes the response.
// A broken connection is not logged: there is nobody to answer.
func Recovery(handle gin.RecoveryFunc) gin.HandlerFunc {
	return gin.CustomRecoveryWithWriter(nil, func(c *gin.Context, rec any) {
		FromContext(c).Error("panic recovered", zap.Any("panic", rec), zap.Stack("stack"))
		handle(c, rec)
	})
}
