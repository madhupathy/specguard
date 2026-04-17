package vectordb

import (
	"crypto/sha256"
	"math"
)

// Embedder is the interface for generating vector embeddings from text.
type Embedder interface {
	Embed(text string) ([]float64, error)
	Dimension() int
}

// PseudoEmbedder generates deterministic pseudo-embeddings from text hashes.
// Useful for testing and offline mode without an LLM API key.
type PseudoEmbedder struct {
	dim int
}

// NewPseudoEmbedder creates a PseudoEmbedder with the given dimension.
func NewPseudoEmbedder(dim int) *PseudoEmbedder {
	if dim <= 0 {
		dim = 64
	}
	return &PseudoEmbedder{dim: dim}
}

func (p *PseudoEmbedder) Embed(text string) ([]float64, error) {
	hash := sha256.Sum256([]byte(text))
	vec := make([]float64, p.dim)
	for i := 0; i < p.dim; i++ {
		byteIdx := i % len(hash)
		vec[i] = float64(hash[byteIdx]) / 255.0
	}
	// Normalize to unit vector
	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	norm = math.Sqrt(norm)
	if norm > 0 {
		for i := range vec {
			vec[i] /= norm
		}
	}
	return vec, nil
}

func (p *PseudoEmbedder) Dimension() int {
	return p.dim
}
