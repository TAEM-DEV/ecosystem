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

// ConstraintsHandler handles requests for the constraint_index collection.
// Routes:
//
//	GET  /api/constraints/search?q={query} — top-10 results
//	POST /api/constraints/index            — write/update (ARCH only)
func ConstraintsHandler(qClient *qdrant.Client) http.HandlerFunc {
	collection := "taem_constraint_index"

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		path := strings.TrimPrefix(r.URL.Path, "/api/constraints")

		switch {
		case r.Method == http.MethodGet && (path == "/search" || path == "/search/"):
			handleConstraintsSearch(w, r, qClient, collection)
		case r.Method == http.MethodPost && (path == "/index" || path == "/index/"):
			handleConstraintsWrite(w, r, qClient, collection)
		default:
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		}
	}
}

func handleConstraintsSearch(w http.ResponseWriter, r *http.Request, qClient *qdrant.Client, collection string) {
	q := r.URL.Query().Get("q")
	if q == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "missing query parameter 'q'"})
		return
	}

	filter := map[string]any{
		"should": []map[string]any{
			{
				"key": "text",
				"match": map[string]any{
					"text": q,
				},
			},
			{
				"key": "constraint_id",
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

func handleConstraintsWrite(w http.ResponseWriter, r *http.Request, qClient *qdrant.Client, collection string) {
	writer := r.Header.Get(writerHeader)
	if !CheckWriter("constraint_index", writer) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"error":  "unauthorized writer",
			"detail": fmt.Sprintf("writer %q is not authorized for constraint_index; only ARCH may write", writer),
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
		// Use constraint_id if available, otherwise generate
		if cid, ok := payload["constraint_id"].(string); ok && cid != "" {
			id = cid
		} else {
			id = fmt.Sprintf("constraint-%d", time.Now().UnixNano())
		}
	}
	delete(payload, "id")
	payload["updated_at"] = time.Now().UTC().Format(time.RFC3339)

	if err := qClient.Upsert(collection, id, payload); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": "qdrant unavailable", "detail": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "id": id})
}
