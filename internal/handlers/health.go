package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/taem-dev/ecosystem/internal/collections"
	"github.com/taem-dev/ecosystem/internal/qdrant"
)

// HealthHandler returns an http.HandlerFunc for GET /health.
// Reports 200 with collection count when Qdrant is reachable, 503 otherwise.
func HealthHandler(qClient *qdrant.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		if err := qClient.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(map[string]any{
				"status": "degraded",
				"qdrant": "unreachable",
				"error":  err.Error(),
			})
			return
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"status":      "ok",
			"qdrant":      "reachable",
			"collections": len(collections.Names),
		})
	}
}
