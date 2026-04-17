package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OllamaProvider implements Provider using the Ollama REST API.
type OllamaProvider struct {
	baseURL        string
	genModel       string
	embedModel     string
	client         *http.Client
	embedDimension int
}

// NewOllamaProvider creates an Ollama-backed LLM provider.
func NewOllamaProvider(baseURL, genModel, embedModel string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if genModel == "" {
		genModel = "llama3.1"
	}
	if embedModel == "" {
		embedModel = "nomic-embed-text"
	}
	return &OllamaProvider{
		baseURL:        baseURL,
		genModel:       genModel,
		embedModel:     embedModel,
		client:         &http.Client{Timeout: 120 * time.Second},
		embedDimension: 768,
	}
}

func (o *OllamaProvider) Name() string { return "ollama" }

func (o *OllamaProvider) EmbedDimension() int { return o.embedDimension }

// Generate calls Ollama /api/generate with stream=false.
func (o *OllamaProvider) Generate(ctx context.Context, prompt string) (string, error) {
	body := map[string]interface{}{
		"model":  o.genModel,
		"prompt": prompt,
		"stream": false,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", o.baseURL+"/api/generate", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama generate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama generate status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return result.Response, nil
}

// Embed calls Ollama /api/embeddings to produce a vector.
func (o *OllamaProvider) Embed(text string) ([]float64, error) {
	body := map[string]interface{}{
		"model":  o.embedModel,
		"prompt": text,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", o.baseURL+"/api/embeddings", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := o.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding: %w", err)
	}

	if len(result.Embedding) > 0 {
		o.embedDimension = len(result.Embedding)
	}
	return result.Embedding, nil
}
