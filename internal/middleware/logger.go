package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
	"github.com/ParsaSafavi05/deploycrane/internal/logging"
	
)

// responseWriter captures HTTP status codes.
type responseWriter struct {
	http.ResponseWriter
	status int
	wrote  bool
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wrote {
		return
	}
	rw.status = code
	rw.wrote = true
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	if !rw.wrote {
		rw.WriteHeader(http.StatusOK)
	}
	return rw.ResponseWriter.Write(b)
}

type ctxKeyLogger struct{}

// LoggingMiddleware logs every HTTP request, adds a request ID,
// and recovers from panics so the server stays alive.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := requestID()

		reqLogger := logging.Logger.With(
			"request_id", reqID,
			"method", r.Method,
			"path", r.URL.Path,
			"remote", r.RemoteAddr,
		)

		w.Header().Set("X-Request-ID", reqID)

		rw := &responseWriter{
			ResponseWriter: w,
			status:         http.StatusOK,
		}

		reqLogger.Info("http request started")

		ctx := WithLogger(r.Context(), reqLogger)

		defer func() {
			if rec := recover(); rec != nil {
				reqLogger.Error("panic recovered",
					"error", rec,
					"stack", string(debug.Stack()),
				)

				if !rw.wrote {
					http.Error(rw, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}

			reqLogger.Info("http request completed",
				"status", rw.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		}()

		next.ServeHTTP(rw, r.WithContext(ctx))
	})
}

// WithLogger stores a request-scoped logger in context.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKeyLogger{}, logger)
}

// FromContext returns the request-scoped logger if present.
// Falls back to the package logger otherwise.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKeyLogger{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return logging.Logger
}

// requestID generates a random request ID.
func requestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UTC().Format("20060102T150405.000000000")
	}
	return hex.EncodeToString(b[:])
}