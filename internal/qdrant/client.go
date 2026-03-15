package qdrant

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is a thin HTTP wrapper around the Qdrant REST API.
// No Qdrant Go SDK — just net/http.
type Client struct {
	BaseURL string
	APIKey  string
	HTTP    *http.Client
}

// NewClient creates a new Qdrant HTTP client.
func NewClient(baseURL, apiKey string) *Client {
	return &Client{
		BaseURL: baseURL,
		APIKey:  apiKey,
		HTTP: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// doRequest executes an HTTP request with optional API key header.
func (c *Client) doRequest(method, url string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.APIKey != "" {
		req.Header.Set("api-key", c.APIKey)
	}
	return c.HTTP.Do(req)
}

// Ping checks if Qdrant is reachable via GET /health.
func (c *Client) Ping() error {
	resp, err := c.doRequest("GET", c.BaseURL+"/health", nil)
	if err != nil {
		return fmt.Errorf("qdrant ping: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("qdrant ping: status %d", resp.StatusCode)
	}
	return nil
}

// CollectionExists checks if a collection exists via GET /collections/{name}.
func (c *Client) CollectionExists(name string) (bool, error) {
	resp, err := c.doRequest("GET", c.BaseURL+"/collections/"+name, nil)
	if err != nil {
		return false, fmt.Errorf("collection exists check: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, nil
	}
	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	return false, fmt.Errorf("collection exists check: status %d", resp.StatusCode)
}

// CreateCollection creates a new collection with the given vector size.
// PUT /collections/{name} with vectors config.
func (c *Client) CreateCollection(name string, vectorSize int) error {
	payload := map[string]any{
		"vectors": map[string]any{
			"size":     vectorSize,
			"distance": "Cosine",
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal create collection: %w", err)
	}
	resp, err := c.doRequest("PUT", c.BaseURL+"/collections/"+name, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create collection: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("create collection %s: status %d: %s", name, resp.StatusCode, string(respBody))
	}
	return nil
}

// Upsert inserts or updates a point in a collection.
// PUT /collections/{collection}/points with point data.
func (c *Client) Upsert(collection string, id string, payload map[string]any) error {
	// Build a zero vector as placeholder when no embedding is provided
	vector := make([]float64, 0)
	if v, ok := payload["_vector"]; ok {
		if vec, ok := v.([]float64); ok {
			vector = vec
		}
		delete(payload, "_vector")
	}

	points := map[string]any{
		"points": []map[string]any{
			{
				"id":      id,
				"vector":  vector,
				"payload": payload,
			},
		},
	}
	body, err := json.Marshal(points)
	if err != nil {
		return fmt.Errorf("marshal upsert: %w", err)
	}
	resp, err := c.doRequest("PUT", c.BaseURL+"/collections/"+collection+"/points", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("upsert: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("upsert to %s: status %d: %s", collection, resp.StatusCode, string(respBody))
	}
	return nil
}

// Get retrieves a point by ID from a collection.
// POST /collections/{collection}/points with id list.
func (c *Client) Get(collection string, id string) (map[string]any, error) {
	reqBody := map[string]any{
		"ids":          []string{id},
		"with_payload": true,
		"with_vector":  false,
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal get: %w", err)
	}
	resp, err := c.doRequest("POST", c.BaseURL+"/collections/"+collection+"/points", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("get point: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read get response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get from %s: status %d: %s", collection, resp.StatusCode, string(respBody))
	}

	var result struct {
		Result []struct {
			ID      string         `json:"id"`
			Payload map[string]any `json:"payload"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal get response: %w", err)
	}
	if len(result.Result) == 0 {
		return nil, nil
	}
	return result.Result[0].Payload, nil
}

// Search performs a scroll/filter search on a collection.
// POST /collections/{collection}/points/scroll with filter.
func (c *Client) Search(collection string, filter map[string]any, limit int) ([]map[string]any, error) {
	reqBody := map[string]any{
		"limit":        limit,
		"with_payload": true,
		"with_vector":  false,
	}
	if filter != nil {
		reqBody["filter"] = filter
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal search: %w", err)
	}
	resp, err := c.doRequest("POST", c.BaseURL+"/collections/"+collection+"/points/scroll", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("search: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read search response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("search %s: status %d: %s", collection, resp.StatusCode, string(respBody))
	}

	var result struct {
		Result struct {
			Points []struct {
				ID      string         `json:"id"`
				Payload map[string]any `json:"payload"`
			} `json:"points"`
		} `json:"result"`
	}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("unmarshal search response: %w", err)
	}

	var out []map[string]any
	for _, p := range result.Result.Points {
		entry := p.Payload
		entry["_id"] = p.ID
		out = append(out, entry)
	}
	return out, nil
}

// Delete removes a point by ID from a collection.
// POST /collections/{collection}/points/delete.
func (c *Client) Delete(collection string, id string) error {
	reqBody := map[string]any{
		"points": []string{id},
	}
	body, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("marshal delete: %w", err)
	}
	resp, err := c.doRequest("POST", c.BaseURL+"/collections/"+collection+"/points/delete", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("delete: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete from %s: status %d: %s", collection, resp.StatusCode, string(respBody))
	}
	return nil
}
