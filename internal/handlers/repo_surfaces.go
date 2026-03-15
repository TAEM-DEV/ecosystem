package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/taem-dev/ecosystem/internal/qdrant"
)

// RepoSurfacesConfig holds configuration for repo surface staleness checks.
type RepoSurfacesConfig struct {
	FreshnessHours  int
	ForceStaleHours int
}

// RepoSurfacesHandler handles requests for the repo_surfaces collection.
// Routes:
//
//	GET  /api/repo_surfaces/{org}/{repo}       — retrieve a repo surface
//	PUT  /api/repo_surfaces/{org}/{repo}       — write a repo surface (NAV only)
//	GET  /api/repo_surfaces/{org}/{repo}/stale — check staleness
func RepoSurfacesHandler(qClient *qdrant.Client, cfg RepoSurfacesConfig) http.HandlerFunc {
	collection := "taem_repo_surfaces"

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		// Parse path: /api/repo_surfaces/{org}/{repo}[/stale]
		path := strings.TrimPrefix(r.URL.Path, "/api/repo_surfaces/")
		parts := strings.Split(path, "/")

		if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
			http.Error(w, `{"error":"path must be /api/repo_surfaces/{org}/{repo}"}`, http.StatusBadRequest)
			return
		}

		org := parts[0]
		repo := parts[1]
		isStaleCheck := len(parts) == 3 && parts[2] == "stale"

		pointID := makePointID(org, repo)

		switch {
		case r.Method == http.MethodGet && isStaleCheck:
			handleStaleCheck(w, qClient, collection, pointID, cfg)
		case r.Method == http.MethodGet:
			handleGetSurface(w, qClient, collection, pointID)
		case r.Method == http.MethodPut:
			handlePutSurface(w, r, qClient, collection, pointID)
		default:
			http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		}
	}
}

func makePointID(org, repo string) string {
	raw := fmt.Sprintf("%s/%s", org, repo)
	hash := sha256.Sum256([]byte(raw))
	// Use first 16 bytes as hex string for a compact, Qdrant-safe ID
	return fmt.Sprintf("%x", hash[:16])
}

func handleGetSurface(w http.ResponseWriter, qClient *qdrant.Client, collection, pointID string) {
	payload, err := qClient.Get(collection, pointID)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": "qdrant unavailable", "detail": err.Error()})
		return
	}
	if payload == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "repo surface not found"})
		return
	}
	json.NewEncoder(w).Encode(payload)
}

func handlePutSurface(w http.ResponseWriter, r *http.Request, qClient *qdrant.Client, collection, pointID string) {
	writer := r.Header.Get(writerHeader)
	if !CheckWriter("repo_surfaces", writer) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"error":  "unauthorized writer",
			"detail": fmt.Sprintf("writer %q is not authorized for repo_surfaces; only NAV may write", writer),
		})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Reject raw mission JSONL
	if isMissionData(body) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error": "use mc-state for mission records",
		})
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "invalid JSON", "detail": err.Error()})
		return
	}

	// Set last_updated timestamp
	payload["last_updated"] = time.Now().UTC().Format(time.RFC3339)

	if err := qClient.Upsert(collection, pointID, payload); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": "qdrant unavailable", "detail": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "id": pointID})
}

func handleStaleCheck(w http.ResponseWriter, qClient *qdrant.Client, collection, pointID string, cfg RepoSurfacesConfig) {
	payload, err := qClient.Get(collection, pointID)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": "qdrant unavailable", "detail": err.Error()})
		return
	}
	if payload == nil {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]any{"error": "repo surface not found"})
		return
	}

	stale := false
	if lastUpdated, ok := payload["last_updated"].(string); ok {
		t, err := time.Parse(time.RFC3339, lastUpdated)
		if err == nil {
			age := time.Since(t)
			// C-006-005: stale if older than FRESHNESS_HOURS, force stale if older than FORCE_STALE_HOURS
			if age > time.Duration(cfg.FreshnessHours)*time.Hour {
				stale = true
			}
			if age > time.Duration(cfg.ForceStaleHours)*time.Hour {
				stale = true
			}
		}
	} else {
		// No last_updated field — consider stale
		stale = true
	}

	json.NewEncoder(w).Encode(map[string]any{"stale": stale})
}

// isMissionData checks if the body looks like raw mission JSONL (signals, manifests).
func isMissionData(body []byte) bool {
	s := strings.TrimSpace(string(body))
	// Check for JSONL indicators: multiple JSON objects on separate lines
	// or known mission-specific fields
	if strings.Contains(s, "\"signal_type\"") || strings.Contains(s, "\"step_plan\"") ||
		strings.Contains(s, "\"manifest\"") || strings.Contains(s, "signals.jsonl") {
		return true
	}
	return false
}
