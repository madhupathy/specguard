// Package llm provides a pluggable LLM provider interface with Ollama and vLLM backends.
package llm

import "context"

// Provider is the interface for LLM generation and embedding.
type Provider interface {
	// Generate produces text from a prompt.
	Generate(ctx context.Context, prompt string) (string, error)
	// Embed produces a vector embedding for the given text.
	Embed(text string) ([]float64, error)
	// EmbedDimension returns the embedding vector dimension.
	EmbedDimension() int
	// Name returns the provider name (e.g. "ollama", "vllm").
	Name() string
}

// Config holds LLM provider configuration parsed from .specguard/config.yaml.
type Config struct {
	Provider        string `yaml:"provider"`         // "ollama", "vllm", or "none"
	BaseURL         string `yaml:"base_url"`          // e.g. http://localhost:11434
	GenerationModel string `yaml:"generation_model"`  // e.g. llama3.1
	EmbeddingModel  string `yaml:"embedding_model"`   // e.g. nomic-embed-text
	APIKey          string `yaml:"api_key"`            // for vLLM auth
	TLSSkipVerify   bool   `yaml:"tls_skip_verify"`   // for internal self-signed certs
}

// DefaultConfig returns a config that disables LLM (graceful fallback).
func DefaultConfig() Config {
	return Config{
		Provider:        "none",
		BaseURL:         "http://localhost:11434",
		GenerationModel: "llama3.1",
		EmbeddingModel:  "nomic-embed-text",
	}
}

// NewProvider creates a Provider from the given config.
// Returns nil if provider is "none" or empty.
func NewProvider(cfg Config) (Provider, error) {
	switch cfg.Provider {
	case "ollama":
		return NewOllamaProvider(cfg.BaseURL, cfg.GenerationModel, cfg.EmbeddingModel), nil
	case "vllm":
		return NewVLLMProvider(cfg.BaseURL, cfg.GenerationModel, cfg.EmbeddingModel, cfg.APIKey, cfg.TLSSkipVerify), nil
	case "none", "":
		return nil, nil
	default:
		return nil, nil
	}
}
