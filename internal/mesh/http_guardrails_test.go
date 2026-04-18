package mesh

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDecodeJSONBodyRejectsOversizedRequest(t *testing.T) {
	body := `{"value":"` + strings.Repeat("a", int(maxJSONBodyBytes)) + `"}`
	req := httptest.NewRequest(http.MethodPost, "/chain/tx", strings.NewReader(body))
	req = withRequestID(req)
	rr := httptest.NewRecorder()

	var dst struct {
		Value string `json:"value"`
	}
	err := decodeJSONBody(rr, req, maxJSONBodyBytes, &dst)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected oversized body rejection, got %v", err)
	}
}

func TestDecodeJSONBodyRejectsMalformedAndTrailingJSON(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/chain/tx", strings.NewReader(`{"value":`))
	req = withRequestID(req)
	rr := httptest.NewRecorder()
	var dst struct {
		Value string `json:"value"`
	}
	if err := decodeJSONBody(rr, req, maxJSONBodyBytes, &dst); err == nil || !strings.Contains(err.Error(), "malformed JSON") {
		t.Fatalf("expected malformed JSON rejection, got %v", err)
	}

	req = httptest.NewRequest(http.MethodPost, "/chain/tx", strings.NewReader(`{"value":"ok"}{"extra":true}`))
	req = withRequestID(req)
	rr = httptest.NewRecorder()
	if err := decodeJSONBody(rr, req, maxJSONBodyBytes, &dst); err == nil || !strings.Contains(err.Error(), "exactly one JSON object") {
		t.Fatalf("expected trailing JSON rejection, got %v", err)
	}
}

func TestRequireMethodReturnsStructuredMethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/chain/status", nil)
	req = withRequestID(req)
	rr := httptest.NewRecorder()

	if requireMethod(rr, req, http.MethodGet) {
		t.Fatalf("expected requireMethod to reject wrong method")
	}
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), `"request_id"`) {
		t.Fatalf("expected structured request_id in response body, got %s", rr.Body.String())
	}
}

func TestBuildHTTPMiddlewareSetsRequestIDAndBlocksRemoteDebug(t *testing.T) {
	cfg := &MeshConfig{HttpRateLimitEnabled: true}
	handler := buildHTTPMiddleware(cfg)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "request_id": requestIDFromContext(r.Context())})
	}))

	req := httptest.NewRequest(http.MethodGet, "/chain/status", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Header().Get(requestIDHeader) == "" {
		t.Fatalf("expected request id header on normal response")
	}

	req = httptest.NewRequest(http.MethodGet, "/debug/ops", nil)
	req.RemoteAddr = "10.0.0.5:12345"
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected remote debug request to be blocked, got %d", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "debug_surface_forbidden") {
		t.Fatalf("expected structured debug guard response, got %s", rr.Body.String())
	}
}
