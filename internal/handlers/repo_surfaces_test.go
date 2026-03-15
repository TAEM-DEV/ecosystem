package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/taem-dev/ecosystem/internal/qdrant"
)

func newMockQdrant(handler http.HandlerFunc) (*httptest.Server, *qdrant.Client) {
	srv := httptest.NewServer(handler)
	client := qdrant.NewClient(srv.URL, "")
	return srv, client
}

func TestRepoSurfaces_PUT_Authorized(t *testing.T) {
	srv, qClient := newMockQdrant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":{"operation_id":1,"status":"completed"}}`))
	}))
	defer srv.Close()

	cfg := RepoSurfacesConfig{FreshnessHours: 24, ForceStaleHours: 168}
	handler := RepoSurfacesHandler(qClient, cfg)

	body := `{"org":"taem-dev","repo":"ecosystem","languages":["Go"]}`
	req := httptest.NewRequest("PUT", "/api/repo_surfaces/taem-dev/ecosystem", strings.NewReader(body))
	req.Header.Set("X-TAEM-Writer", "NAV")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %v", resp["status"])
	}
}

func TestRepoSurfaces_PUT_Unauthorized(t *testing.T) {
	srv, qClient := newMockQdrant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := RepoSurfacesConfig{FreshnessHours: 24, ForceStaleHours: 168}
	handler := RepoSurfacesHandler(qClient, cfg)

	body := `{"org":"taem-dev","repo":"ecosystem"}`
	req := httptest.NewRequest("PUT", "/api/repo_surfaces/taem-dev/ecosystem", strings.NewReader(body))
	req.Header.Set("X-TAEM-Writer", "ARCH") // Wrong writer
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRepoSurfaces_PUT_NoWriter(t *testing.T) {
	srv, qClient := newMockQdrant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := RepoSurfacesConfig{FreshnessHours: 24, ForceStaleHours: 168}
	handler := RepoSurfacesHandler(qClient, cfg)

	body := `{"org":"taem-dev","repo":"ecosystem"}`
	req := httptest.NewRequest("PUT", "/api/repo_surfaces/taem-dev/ecosystem", strings.NewReader(body))
	// No X-TAEM-Writer header
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRepoSurfaces_GET_Found(t *testing.T) {
	srv, qClient := newMockQdrant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := `{"result":[{"id":"abc","payload":{"org":"taem-dev","repo":"ecosystem","languages":["Go"]}}]}`
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	cfg := RepoSurfacesConfig{FreshnessHours: 24, ForceStaleHours: 168}
	handler := RepoSurfacesHandler(qClient, cfg)

	req := httptest.NewRequest("GET", "/api/repo_surfaces/taem-dev/ecosystem", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["org"] != "taem-dev" {
		t.Errorf("expected org=taem-dev, got %v", resp["org"])
	}
}

func TestRepoSurfaces_GET_NotFound(t *testing.T) {
	srv, qClient := newMockQdrant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":[]}`))
	}))
	defer srv.Close()

	cfg := RepoSurfacesConfig{FreshnessHours: 24, ForceStaleHours: 168}
	handler := RepoSurfacesHandler(qClient, cfg)

	req := httptest.NewRequest("GET", "/api/repo_surfaces/taem-dev/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestRepoSurfaces_Stale_Fresh(t *testing.T) {
	freshTime := time.Now().UTC().Format(time.RFC3339)
	srv, qClient := newMockQdrant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := `{"result":[{"id":"abc","payload":{"last_updated":"` + freshTime + `"}}]}`
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	cfg := RepoSurfacesConfig{FreshnessHours: 24, ForceStaleHours: 168}
	handler := RepoSurfacesHandler(qClient, cfg)

	req := httptest.NewRequest("GET", "/api/repo_surfaces/taem-dev/ecosystem/stale", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["stale"] != false {
		t.Errorf("expected stale=false, got %v", resp["stale"])
	}
}

func TestRepoSurfaces_Stale_Old(t *testing.T) {
	oldTime := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	srv, qClient := newMockQdrant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		resp := `{"result":[{"id":"abc","payload":{"last_updated":"` + oldTime + `"}}]}`
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	cfg := RepoSurfacesConfig{FreshnessHours: 24, ForceStaleHours: 168}
	handler := RepoSurfacesHandler(qClient, cfg)

	req := httptest.NewRequest("GET", "/api/repo_surfaces/taem-dev/ecosystem/stale", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["stale"] != true {
		t.Errorf("expected stale=true for 48h old entry, got %v", resp["stale"])
	}
}

func TestRepoSurfaces_PUT_MissionDataRejected(t *testing.T) {
	srv, qClient := newMockQdrant(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	cfg := RepoSurfacesConfig{FreshnessHours: 24, ForceStaleHours: 168}
	handler := RepoSurfacesHandler(qClient, cfg)

	body := `{"signal_type":"error","message":"something broke"}`
	req := httptest.NewRequest("PUT", "/api/repo_surfaces/taem-dev/ecosystem", strings.NewReader(body))
	req.Header.Set("X-TAEM-Writer", "NAV")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "use mc-state for mission records" {
		t.Errorf("expected mc-state error, got %v", resp["error"])
	}
}
