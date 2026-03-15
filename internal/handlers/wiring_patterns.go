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

// WiringPatternsHandler handles requests for the wiring_patterns collection.
// Routes:
//
//	GET  /api/wiring_patterns/search?from={}&to={}&via={} — filter search
//	POST /api/wiring_patterns                             — write (kernel.mission_close only)
func WiringPatternsHandler(qClient *qdrant.Client) http.HandlerFunc {
	collection := "taem_wiring_patterns"

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		path := strings.TrimPrefix(r.URL.Path, "/api/wiring_patterns")

		switch {
		case r.Method == http.MethodGet && (path == "/search" || path == "/search/"):
			handleWiringSearch(w, r, qClient, collection)
		case r.Method == http.MethodPost && (path == "" || path == "/"):
			handleWiringWrite(w, r, qClient, collection)
		default:
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		}
	}
}

func handleWiringSearch(w http.ResponseWriter, r *http.Request, qClient *qdrant.Client, collection string) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")
	via := r.URL.Query().Get("via")

	if from == "" && to == "" && via == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "at least one of 'from', 'to', or 'via' query parameters required"})
		return
	}

	// Build filter conditions
	var must []map[string]any
	if from != "" {
		must = append(must, map[string]any{
			"key": "from",
			"match": map[string]any{
				"value": from,
			},
		})
	}
	if to != "" {
		must = append(must, map[string]any{
			"key": "to",
			"match": map[string]any{
				"value": to,
			},
		})
	}
	if via != "" {
		must = append(must, map[string]any{
			"key": "via",
			"match": map[string]any{
				"value": via,
			},
		})
	}

	filter := map[string]any{
		"must": must,
	}

	results, err := qClient.Search(collection, filter, 10)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": "qdrant unavailable", "detail": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"results": results, "count": len(results)})
}

func handleWiringWrite(w http.ResponseWriter, r *http.Request, qClient *qdrant.Client, collection string) {
	writer := r.Header.Get(writerHeader)
	if !CheckWriter("wiring_patterns", writer) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"error":  "unauthorized writer",
			"detail": fmt.Sprintf("writer %q is not authorized for wiring_patterns; only kernel.mission_close may write", writer),
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
		id = fmt.Sprintf("wiring-%d", time.Now().UnixNano())
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
