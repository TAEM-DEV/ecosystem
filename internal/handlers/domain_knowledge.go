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

// DomainKnowledgeHandler handles requests for the domain_knowledge collection.
// ADR-009a: field intelligence from refexplorer (Semantic Scholar, OpenAlex, etc.)
//
// Routes:
//
//	GET  /api/domain_knowledge/search?q={query}          — semantic search top-10
//	GET  /api/domain_knowledge/search?q={query}&tag={t}  — filtered by domain tag
//	POST /api/domain_knowledge                           — write entry (refexplorer only)
func DomainKnowledgeHandler(qClient *qdrant.Client) http.HandlerFunc {
	collection := "domain_knowledge"

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		path := strings.TrimPrefix(r.URL.Path, "/api/domain_knowledge")

		switch {
		case r.Method == http.MethodGet && (path == "/search" || path == "/search/"):
			handleDomainKnowledgeSearch(w, r, qClient, collection)
		case r.Method == http.MethodPost && (path == "" || path == "/"):
			handleDomainKnowledgeWrite(w, r, qClient, collection)
		default:
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		}
	}
}

func handleDomainKnowledgeSearch(w http.ResponseWriter, r *http.Request, qClient *qdrant.Client, collection string) {
	q := r.URL.Query().Get("q")
	if q == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "missing query parameter 'q'"})
		return
	}

	// Build filter — keyword search on title and abstract, optional domain tag filter
	shouldClauses := []map[string]any{
		{
			"key": "title",
			"match": map[string]any{
				"text": q,
			},
		},
		{
			"key": "abstract",
			"match": map[string]any{
				"text": q,
			},
		},
	}

	filter := map[string]any{
		"should": shouldClauses,
	}

	// Optional domain tag filter
	tag := r.URL.Query().Get("tag")
	if tag != "" {
		filter = map[string]any{
			"must": []map[string]any{
				{
					"should": shouldClauses,
				},
				{
					"key": "domain_tags",
					"match": map[string]any{
						"value": tag,
					},
				},
			},
		}
	}

	results, err := qClient.Search(collection, filter, 10)
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": "qdrant unavailable", "detail": err.Error()})
		return
	}

	json.NewEncoder(w).Encode(map[string]any{"results": results, "count": len(results)})
}

func handleDomainKnowledgeWrite(w http.ResponseWriter, r *http.Request, qClient *qdrant.Client, collection string) {
	writer := r.Header.Get(writerHeader)
	if !CheckWriter("domain_knowledge", writer) {
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(map[string]any{
			"error":  "unauthorized writer",
			"detail": fmt.Sprintf("writer %q is not authorized for domain_knowledge; only refexplorer may write", writer),
		})
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{"error": "invalid JSON", "detail": err.Error()})
		return
	}

	// Validate required fields per ADR-009a C-009-001
	required := []string{"entry_id", "title", "source", "domain_tags", "score"}
	for _, field := range required {
		if _, ok := payload[field]; !ok {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]any{
				"error":  "missing required field",
				"detail": fmt.Sprintf("field %q is required per ADR-009a C-009-001", field),
			})
			return
		}
	}

	// Validate source field per ADR-009a
	validSources := map[string]bool{
		"semantic_scholar": true, "openalex": true, "crossref": true,
		"operator": true, "eagle-scout": true, "NVD": true,
	}
	source, _ := payload["source"].(string)
	if !validSources[source] {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]any{
			"error":  "invalid source",
			"detail": fmt.Sprintf("source %q not in allowed set per ADR-009a", source),
		})
		return
	}

	// Operator annotations are append-only (C-009-005)
	// Ensure annotations field exists as array
	if _, ok := payload["operator_annotations"]; !ok {
		payload["operator_annotations"] = []any{}
	}

	id, ok := payload["entry_id"].(string)
	if !ok || id == "" {
		id = fmt.Sprintf("dk-%d", time.Now().UnixNano())
	}
	payload["updated_at"] = time.Now().UTC().Format(time.RFC3339)
	if _, ok := payload["created_at"]; !ok {
		payload["created_at"] = payload["updated_at"]
	}

	if err := qClient.Upsert(collection, id, payload); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]any{"error": "qdrant unavailable", "detail": err.Error()})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]any{"status": "ok", "id": id})
}
