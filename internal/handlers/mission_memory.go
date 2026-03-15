package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/taem-dev/ecosystem/internal/qdrant"
)

// MissionMemoryHandler handles requests for the mission_memory collection.
// Routes:
//
//	POST /api/mission_memory             — write (kernel.mission_close only)
//	GET  /api/mission_memory/search?q={task} — top-5 similar
func MissionMemoryHandler(qClient *qdrant.Client) http.HandlerFunc {
	collection := "taem_mission_memory"

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		path := strings.TrimPrefix(r.URL.Path, "/api/mission_memory")

		switch {
		case r.Method == http.MethodGet && (path == "/search" || path == "/search/"):
			handleMissionMemorySearch(w, r, qClient, collection)
		case r.Method == http.MethodPost && (path == "" || path == "/"):
			handleMissionMemoryWrite(w, r, qClient, collection)
		default:
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		}
	}
}

func handleMissionMemorySearch(w http.ResponseWriter, r *http.Request, qClient *qdrant.Client, collection string) {
	q := r.URL.Query().Get("q")
	if q == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "missing query parameter 'q'"})
		return
	}

	filter := map[string]any{
		"should": []map[string]any{
			{
				"key": "task",
				"match": map[string]any{
					"text": q,
				},
			},
			{
				"key": "summary",
				"match": map[string]any{
					"text": q,
				},
			},
		},
	}

	results, err := qClient.Search(collection, filter, 5)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": "qdrant unavailable", "detail": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"results": results, "count": len(results)})
}

func handleMissionMemoryWrite(w http.ResponseWriter, r *http.Request, qClient *qdrant.Client, collection string) {
	writer := r.Header.Get(writerHeader)
	if !CheckWriter("mission_memory", writer) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"error":  "unauthorized writer",
			"detail": fmt.Sprintf("writer %q is not authorized for mission_memory; only kernel.mission_close may write", writer),
		})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	if isMissionData(body) {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "use mc-state for mission records"})
		return
	}

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "invalid JSON", "detail": err.Error()})
		return
	}

	id, ok := payload["id"].(string)
	if !ok || id == "" {
		id = fmt.Sprintf("mission-%d", time.Now().UnixNano())
	}
	delete(payload, "id")
	payload["created_at"] = time.Now().UTC().Format(time.RFC3339)

	if err := qClient.Upsert(collection, id, payload); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": "qdrant unavailable", "detail": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "id": id})
}
