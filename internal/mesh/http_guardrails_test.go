package mesh

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBodyRejectsOversize(t *testing.T) {
	payload := `{"data":"` + strings.Repeat("a", 200) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/chain/tx", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	var body map[string]string

	if decodeJSONBody(w, req, &body, 64) {
		t.Fatalf("expected decodeJSONBody to reject oversized payload")
	}
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", w.Code)
	}
}

func TestDecodeJSONBodyRejectsMalformedJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/chain/tx", strings.NewReader(`{"from":"a",`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	var body map[string]string

	if decodeJSONBody(w, req, &body, maxJSONBodySmall) {
		t.Fatalf("expected decodeJSONBody to reject malformed JSON")
	}
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestAllowMethodRejectsUnexpectedMethod(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/chain/height", nil)
	w := httptest.NewRecorder()

	if allowMethod(w, req, http.MethodGet) {
		t.Fatalf("expected method check to fail")
	}
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
	if got := w.Header().Get("Allow"); got != http.MethodGet {
		t.Fatalf("unexpected Allow header: %q", got)
	}
}

func TestGuardrailMiddlewareSetsRequestIDAndBlocksRemoteDebug(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	handler := buildHTTPGuardrailMiddleware()(next)

	req := httptest.NewRequest(http.MethodGet, "/debug/nodeid", nil)
	req.RemoteAddr = "8.8.8.8:1234"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
	if got := w.Header().Get("X-Request-Id"); got == "" {
		t.Fatalf("expected X-Request-Id to be set")
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "debug_surface_local_only" {
		t.Fatalf("unexpected error code: %v", resp["error"])
	}
}
