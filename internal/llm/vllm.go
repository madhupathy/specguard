package llm

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VLLMProvider implements Provider using the vLLM OpenAI-compatible API.
type VLLMProvider struct {
	baseURL        string
	genModel       string
	embedModel     string
	apiKey         string
	client         *http.Client
	embedDimension int
}

// NewVLLMProvider creates a vLLM-backed LLM provider (OpenAI-compatible API).
func NewVLLMProvider(baseURL, genModel, embedModel, apiKey string, tlsSkipVerify bool) *VLLMProvider {
	if baseURL == "" {
		baseURL = "http://localhost:8000"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if genModel == "" {
		genModel = "meta-llama/Llama-3.1-8B-Instruct"
	}
	if embedModel == "" {
		embedModel = "nomic-ai/nomic-embed-text-v1.5"
	}

	transport := &http.Transport{}
	if tlsSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}

	return &VLLMProvider{
		baseURL:        baseURL,
		genModel:       genModel,
		embedModel:     embedModel,
		apiKey:         apiKey,
		client:         &http.Client{Timeout: 120 * time.Second, Transport: transport},
		embedDimension: 768,
	}
}

func (v *VLLMProvider) Name() string { return "vllm" }

func (v *VLLMProvider) EmbedDimension() int { return v.embedDimension }

// Generate calls the OpenAI-compatible /v1/chat/completions endpoint.
func (v *VLLMProvider) Generate(ctx context.Context, prompt string) (string, error) {
	body := map[string]interface{}{
		"model": v.genModel,
		"messages": []map[string]string{
			{"role": "system", "content": "You are a concise API governance analyst. Provide clear, actionable summaries."},
			{"role": "user", "content": prompt},
		},
		"temperature": 0.1,
		"max_tokens":  1024,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", v.baseURL+"/v1/chat/completions", bytes.NewReader(jsonBody))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if v.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+v.apiKey)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("vllm generate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("vllm generate status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	if len(result.Choices) == 0 {
		return "", fmt.Errorf("vllm returned no choices")
	}
	return result.Choices[0].Message.Content, nil
}

// Embed calls the OpenAI-compatible /v1/embeddings endpoint.
func (v *VLLMProvider) Embed(text string) ([]float64, error) {
	body := map[string]interface{}{
		"model": v.embedModel,
		"input": text,
	}
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequest("POST", v.baseURL+"/v1/embeddings", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if v.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+v.apiKey)
	}

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vllm embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vllm embed status %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		Data []struct {
			Embedding []float64 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode embedding: %w", err)
	}
	if len(result.Data) == 0 {
		return nil, fmt.Errorf("vllm returned no embeddings")
	}

	emb := result.Data[0].Embedding
	if len(emb) > 0 {
		v.embedDimension = len(emb)
	}
	return emb, nil
}
