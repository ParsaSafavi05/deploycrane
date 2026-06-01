package middleware

import (
	"net/http"
	"strings"
)

func CORS(allowedOrigins string) func(http.Handler) http.Handler {
	isAllowAll := strings.TrimSpace(allowedOrigins) == "*"

	var originMap map[string]bool
	if !isAllowAll {
		origins := strings.Split(allowedOrigins, ",")
		originMap = make(map[string]bool)
		for _, origin := range origins {
			originMap[strings.TrimSpace(origin)] = true
		}
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {

			origin := r.Header.Get("Origin")

			// ✅ allow all origins
			if isAllowAll && origin != "" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if originMap != nil && originMap[origin] {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Vary", "Origin")
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
