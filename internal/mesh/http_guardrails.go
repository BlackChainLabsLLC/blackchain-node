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
)

const (
	maxJSONBodyBytes     int64 = 64 << 10
	maxSnapshotBodyBytes int64 = 2 << 20
	requestIDHeader            = "X-Request-Id"
)

type requestIDContextKey struct{}

func requestIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(requestIDContextKey{}).(string)
	return v
}

func withRequestID(r *http.Request) *http.Request {
	id := strings.TrimSpace(r.Header.Get(requestIDHeader))
	if id == "" {
		id = newRequestID()
	}
	ctx := context.WithValue(r.Context(), requestIDContextKey{}, id)
	return r.WithContext(ctx)
}

func newRequestID() string {
	var b [12]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "req-fallback"
	}
	return hex.EncodeToString(b[:])
}

func writeAPIError(r *http.Request, w http.ResponseWriter, status int, code, message string) {
	resp := map[string]any{
		"ok":         false,
		"error":      code,
		"message":    message,
		"status":     status,
		"request_id": requestIDFromContext(r.Context()),
	}
	writeJSON(w, status, resp)
}

func requireMethod(w http.ResponseWriter, r *http.Request, method string) bool {
	if r.Method == method {
		return true
	}
	writeAPIError(r, w, http.StatusMethodNotAllowed, "method_not_allowed", fmt.Sprintf("method %s not allowed; expected %s", r.Method, method))
	return false
}

func decodeJSONBody(w http.ResponseWriter, r *http.Request, limit int64, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		var syntaxErr *json.SyntaxError
		var typeErr *json.UnmarshalTypeError
		switch {
		case errors.As(err, &syntaxErr):
			return fmt.Errorf("malformed JSON at byte %d", syntaxErr.Offset)
		case errors.As(err, &typeErr):
			return fmt.Errorf("invalid JSON type for field %s", typeErr.Field)
		case errors.Is(err, io.EOF):
			return fmt.Errorf("request body must not be empty")
		case errors.Is(err, io.ErrUnexpectedEOF):
			return fmt.Errorf("malformed JSON")
		case strings.Contains(err.Error(), "unexpected EOF"):
			return fmt.Errorf("malformed JSON")
		case strings.Contains(err.Error(), "http: request body too large"):
			return fmt.Errorf("request body exceeds %d bytes", limit)
		default:
			return fmt.Errorf("invalid request body: %w", err)
		}
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		return fmt.Errorf("request body must contain exactly one JSON object")
	}
	return nil
}

func readBodyBytes(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	defer r.Body.Close()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		if strings.Contains(err.Error(), "http: request body too large") {
			return nil, fmt.Errorf("request body exceeds %d bytes", limit)
		}
		return nil, fmt.Errorf("read request body: %w", err)
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil, fmt.Errorf("request body must not be empty")
	}
	return raw, nil
}
