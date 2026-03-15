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

// LessonsHandler handles requests for the lessons_learned collection.
// Routes:
//
//	GET  /api/lessons/search?q={task} — semantic search top-10
//	POST /api/lessons                — write lesson (kernel.mission_close only)
func LessonsHandler(qClient *qdrant.Client) http.HandlerFunc {
	collection := "taem_lessons_learned"

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		path := strings.TrimPrefix(r.URL.Path, "/api/lessons")

		switch {
		case r.Method == http.MethodGet && (path == "/search" || path == "/search/"):
			handleLessonsSearch(w, r, qClient, collection)
		case r.Method == http.MethodPost && (path == "" || path == "/"):
			handleLessonsWrite(w, r, qClient, collection)
		default:
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		}
	}
}

func handleLessonsSearch(w http.ResponseWriter, r *http.Request, qClient *qdrant.Client, collection string) {
	q := r.URL.Query().Get("q")
	if q == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "missing query parameter 'q'"})
		return
	}

	// Use keyword filter as fallback when no embedding service is available
	filter := map[string]any{
		"should": []map[string]any{
			{
				"key": "text",
				"match": map[string]any{
					"text": q,
				},
			},
			{
				"key": "task",
				"match": map[string]any{
					"text": q,
				},
			},
		},
	}

	results, err := qClient.Search(collection, filter, 10)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": "qdrant unavailable", "detail": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"results": results, "count": len(results)})
}

func handleLessonsWrite(w http.ResponseWriter, r *http.Request, qClient *qdrant.Client, collection string) {
	writer := r.Header.Get(writerHeader)
	if !CheckWriter("lessons_learned", writer) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"error":  "unauthorized writer",
			"detail": fmt.Sprintf("writer %q is not authorized for lessons_learned; only kernel.mission_close may write", writer),
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

	// Generate an ID from timestamp if not provided
	id, ok := payload["id"].(string)
	if !ok || id == "" {
		id = fmt.Sprintf("lesson-%d", time.Now().UnixNano())
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
