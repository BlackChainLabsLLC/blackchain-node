package mesh

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
)

const (
	maxJSONBodySmall  int64 = 16 << 10  // 16 KiB
	maxJSONBodyMedium int64 = 256 << 10 // 256 KiB
	maxJSONBodyLarge  int64 = 2 << 20   // 2 MiB
)

type requestIDContextKey struct{}

var requestIDCounter uint64

func buildHTTPGuardrailMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := requestIDFromHeader(r.Header.Get("X-Request-Id"))
			ctx := context.WithValue(r.Context(), requestIDContextKey{}, reqID)
			w.Header().Set("X-Request-Id", reqID)

			if strings.HasPrefix(r.URL.Path, "/debug/") && !isLoopbackIP(clientIP(r)) {
				writeAPIError(w, r.WithContext(ctx), http.StatusForbidden, "debug_surface_local_only", "debug endpoints are restricted to loopback clients")
				return
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func requestIDFromHeader(in string) string {
	clean := strings.TrimSpace(in)
	if clean != "" && len(clean) <= 64 {
		return clean
	}

	var b [8]byte
	if _, err := rand.Read(b[:]); err == nil {
		return "req-" + hex.EncodeToString(b[:])
	}

	n := atomic.AddUint64(&requestIDCounter, 1)
	return fmt.Sprintf("req-fallback-%s-%d", time.Now().UTC().Format("20060102T150405.000000000"), n)
}

func requestIDFromContext(ctx context.Context) string {
	v := ctx.Value(requestIDContextKey{})
	s, _ := v.(string)
	if strings.TrimSpace(s) == "" {
		return "req-unknown"
	}
	return s
}

func allowMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	w.Header().Set("Allow", method)
	writeAPIError(w, r, http.StatusMethodNotAllowed, "method_not_allowed", "only "+method+" is supported for this endpoint")
	return false
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) bool {
	if maxBytes <= 0 {
		maxBytes = maxJSONBodySmall
	}

	if ct := strings.TrimSpace(r.Header.Get("Content-Type")); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
		writeAPIError(w, r, http.StatusUnsupportedMediaType, "unsupported_media_type", "content-type must be application/json")
		return false
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	if err := dec.Decode(dst); err != nil {
		var syntaxErr *json.SyntaxError
		switch {
		case err == io.EOF:
			writeAPIError(w, r, http.StatusBadRequest, "empty_body", "request body is required")
		case strings.Contains(err.Error(), "http: request body too large"):
			writeAPIError(w, r, http.StatusRequestEntityTooLarge, "body_too_large", "request body exceeds size limit")
		case strings.Contains(err.Error(), "unknown field"):
			writeAPIError(w, r, http.StatusBadRequest, "unknown_field", err.Error())
		case errors.As(err, &syntaxErr):
			writeAPIError(w, r, http.StatusBadRequest, "malformed_json", "invalid JSON syntax")
		default:
			writeAPIError(w, r, http.StatusBadRequest, "invalid_json", err.Error())
		}
		return false
	}

	var extra json.RawMessage
	if err := dec.Decode(&extra); err != io.EOF {
		writeAPIError(w, r, http.StatusBadRequest, "invalid_json_trailing_data", "request body must contain exactly one JSON object")
		return false
	}
	return true
}

func writeAPIError(w http.ResponseWriter, r *http.Request, status int, code string, message string) {
	if code == "" {
		code = "error"
	}
	if message == "" {
		message = "request failed"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":         false,
		"error":      code,
		"message":    message,
		"status":     status,
		"request_id": requestIDFromContext(r.Context()),
		"path":       r.URL.Path,
	})
}
