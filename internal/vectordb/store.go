package vectordb

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	_ "modernc.org/sqlite"
)

// Store is a SQLite-backed vector database for document embeddings.
type Store struct {
	db *sql.DB
}

// Record represents a stored embedding with its metadata.
type Record struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	ChunkIdx  int       `json:"chunk_index"`
	Content   string    `json:"content"`
	Embedding []float64 `json:"embedding"`
}

// SearchResult is a record with a similarity score.
type SearchResult struct {
	Record
	Score float64 `json:"score"`
}

// Open creates or opens a SQLite vector store at the given path.
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("create db dir: %w", err)
	}

	db, err := sql.Open("sqlite", dbPath+"?_journal_mode=WAL")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// Upsert inserts or replaces an embedding record.
func (s *Store) Upsert(rec Record) error {
	embJSON, err := json.Marshal(rec.Embedding)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}

	_, err = s.db.Exec(`
		INSERT OR REPLACE INTO embeddings (id, source, chunk_index, content, embedding, dim)
		VALUES (?, ?, ?, ?, ?, ?)
	`, rec.ID, rec.Source, rec.ChunkIdx, rec.Content, string(embJSON), len(rec.Embedding))
	return err
}

// UpsertBatch inserts multiple records in a single transaction.
func (s *Store) UpsertBatch(records []Record) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO embeddings (id, source, chunk_index, content, embedding, dim)
		VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, rec := range records {
		embJSON, err := json.Marshal(rec.Embedding)
		if err != nil {
			return fmt.Errorf("marshal embedding for %s: %w", rec.ID, err)
		}
		if _, err := stmt.Exec(rec.ID, rec.Source, rec.ChunkIdx, rec.Content, string(embJSON), len(rec.Embedding)); err != nil {
			return fmt.Errorf("insert %s: %w", rec.ID, err)
		}
	}

	return tx.Commit()
}

// Search finds the top-k most similar records to the query embedding using cosine similarity.
func (s *Store) Search(queryEmbedding []float64, topK int) ([]SearchResult, error) {
	rows, err := s.db.Query(`SELECT id, source, chunk_index, content, embedding FROM embeddings`)
	if err != nil {
		return nil, fmt.Errorf("query embeddings: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var rec Record
		var embJSON string
		if err := rows.Scan(&rec.ID, &rec.Source, &rec.ChunkIdx, &rec.Content, &embJSON); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		if err := json.Unmarshal([]byte(embJSON), &rec.Embedding); err != nil {
			continue
		}

		score := cosineSimilarity(queryEmbedding, rec.Embedding)
		results = append(results, SearchResult{Record: rec, Score: score})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK > 0 && len(results) > topK {
		results = results[:topK]
	}

	return results, nil
}

// Count returns the number of stored embeddings.
func (s *Store) Count() (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM embeddings`).Scan(&count)
	return count, err
}

// DeleteBySource removes all embeddings from a given source file.
func (s *Store) DeleteBySource(source string) error {
	_, err := s.db.Exec(`DELETE FROM embeddings WHERE source = ?`, source)
	return err
}

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS embeddings (
			id TEXT PRIMARY KEY,
			source TEXT NOT NULL,
			chunk_index INTEGER NOT NULL,
			content TEXT NOT NULL,
			embedding TEXT NOT NULL,
			dim INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_embeddings_source ON embeddings(source);
	`)
	return err
}

func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
