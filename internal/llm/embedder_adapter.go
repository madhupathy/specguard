package llm

// EmbedderAdapter wraps an LLM Provider to satisfy the vectordb.Embedder interface.
type EmbedderAdapter struct {
	provider Provider
}

// NewEmbedderAdapter creates an adapter from an LLM Provider to vectordb.Embedder.
func NewEmbedderAdapter(p Provider) *EmbedderAdapter {
	return &EmbedderAdapter{provider: p}
}

func (a *EmbedderAdapter) Embed(text string) ([]float64, error) {
	return a.provider.Embed(text)
}

func (a *EmbedderAdapter) Dimension() int {
	return a.provider.EmbedDimension()
}
