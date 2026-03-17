package qdrant

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPing(t *testing.T) {
	t.Run("reachable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/healthz" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"title":"qdrant","version":"1.0"}`))
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "")
		if err := c.Ping(); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("unreachable", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "")
		if err := c.Ping(); err == nil {
			t.Fatal("expected error for 503 response")
		}
	})
}

func TestCollectionExists(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"result":{"status":"green"}}`))
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "")
		exists, err := c.CollectionExists("test_collection")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !exists {
			t.Fatal("expected collection to exist")
		}
	})

	t.Run("not_exists", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "")
		exists, err := c.CollectionExists("missing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exists {
			t.Fatal("expected collection to not exist")
		}
	})
}

func TestUpsert(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/collections/test_col/points" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		body, _ := io.ReadAll(r.Body)
		var req map[string]any
		if err := json.Unmarshal(body, &req); err != nil {
			t.Fatalf("failed to unmarshal body: %v", err)
		}

		points, ok := req["points"].([]any)
		if !ok || len(points) != 1 {
			t.Fatal("expected 1 point in body")
		}
		point := points[0].(map[string]any)
		if point["id"] != "test-id" {
			t.Errorf("expected id test-id, got %v", point["id"])
		}
		payload := point["payload"].(map[string]any)
		if payload["name"] != "hello" {
			t.Errorf("expected payload name=hello, got %v", payload["name"])
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result":{"operation_id":1,"status":"completed"}}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	err := c.Upsert("test_col", "test-id", map[string]any{"name": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGet(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.URL.Path != "/collections/test_col/points" {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			w.WriteHeader(http.StatusOK)
			resp := `{"result":[{"id":"abc","payload":{"org":"taem-dev","repo":"ecosystem"}}]}`
			w.Write([]byte(resp))
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "")
		payload, err := c.Get("test_col", "abc")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if payload == nil {
			t.Fatal("expected payload, got nil")
		}
		if payload["org"] != "taem-dev" {
			t.Errorf("expected org=taem-dev, got %v", payload["org"])
		}
	})

	t.Run("not_found", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"result":[]}`))
		}))
		defer srv.Close()

		c := NewClient(srv.URL, "")
		payload, err := c.Get("test_col", "missing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if payload != nil {
			t.Fatalf("expected nil, got %v", payload)
		}
	})
}

func TestSearch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/collections/test_col/points/scroll" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		w.WriteHeader(http.StatusOK)
		resp := `{"result":{"points":[{"id":"p1","payload":{"text":"lesson one"}},{"id":"p2","payload":{"text":"lesson two"}}]}}`
		w.Write([]byte(resp))
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "")
	results, err := c.Search("test_col", nil, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0]["text"] != "lesson one" {
		t.Errorf("expected text=lesson one, got %v", results[0]["text"])
	}
}

func TestAPIKeyHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "test-key" {
			t.Errorf("expected api-key header test-key, got %q", r.Header.Get("api-key"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := NewClient(srv.URL, "test-key")
	_ = c.Ping()
}
