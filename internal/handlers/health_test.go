package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/taem-dev/ecosystem/internal/qdrant"
)

func TestHealthHandler_OK(t *testing.T) {
	// Mock Qdrant returning 200
	qdrantSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"title":"qdrant"}`))
	}))
	defer qdrantSrv.Close()

	qClient := qdrant.NewClient(qdrantSrv.URL, "")
	handler := HealthHandler(qClient)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
	if resp["qdrant"] != "reachable" {
		t.Errorf("expected qdrant reachable, got %v", resp["qdrant"])
	}
	// collections should be 5 (float64 from JSON)
	if resp["collections"] != float64(5) {
		t.Errorf("expected 5 collections, got %v", resp["collections"])
	}
}

func TestHealthHandler_503(t *testing.T) {
	// Mock Qdrant returning 503
	qdrantSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer qdrantSrv.Close()

	qClient := qdrant.NewClient(qdrantSrv.URL, "")
	handler := HealthHandler(qClient)

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "degraded" {
		t.Errorf("expected status degraded, got %v", resp["status"])
	}
	if resp["qdrant"] != "unreachable" {
		t.Errorf("expected qdrant unreachable, got %v", resp["qdrant"])
	}
}
