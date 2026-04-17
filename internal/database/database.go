package database

import (
	"context"
	"fmt"
	"log"

	"github.com/specguard/specguard/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
	_ "github.com/lib/pq"
)

type Database struct {
	pool *pgxpool.Pool
}

func New(cfg config.DatabaseConfig) (*Database, error) {
	connString := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.Host, cfg.Port, cfg.User, cfg.Password, cfg.DBName, cfg.SSLMode,
	)

	pool, err := pgxpool.New(context.Background(), connString)
	if err != nil {
		return nil, fmt.Errorf("failed to create connection pool: %w", err)
	}

	// Test connection
	if err := pool.Ping(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	db := &Database{pool: pool}

	// Run migrations
	if err := db.migrate(); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}

	return db, nil
}

func (d *Database) Close() {
	d.pool.Close()
}

func (d *Database) migrate() error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS repositories (
			id SERIAL PRIMARY KEY,
			name VARCHAR(255) NOT NULL UNIQUE,
			url VARCHAR(500) NOT NULL,
			config JSONB,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS specs (
			id SERIAL PRIMARY KEY,
			repo_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
			version VARCHAR(100) NOT NULL,
			branch VARCHAR(100),
			commit_hash VARCHAR(40),
			content_hash VARCHAR(64) NOT NULL,
			spec_type VARCHAR(20) NOT NULL, -- 'openapi', 'proto', 'asyncapi'
			content JSONB NOT NULL,
			metadata JSONB,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
			UNIQUE(repo_id, version, branch)
		)`,

		`CREATE TABLE IF NOT EXISTS changes (
			id SERIAL PRIMARY KEY,
			from_spec_id INTEGER REFERENCES specs(id) ON DELETE SET NULL,
			to_spec_id INTEGER REFERENCES specs(id) ON DELETE CASCADE,
			change_type VARCHAR(20) NOT NULL, -- 'breaking', 'non_breaking', 'deprecation', 'addition'
			classification VARCHAR(30) NOT NULL, -- 'endpoint_added', 'endpoint_removed', 'schema_changed', etc.
			path VARCHAR(500) NOT NULL,
			description TEXT,
			ai_summary TEXT,
			impact_score INTEGER DEFAULT 0,
			metadata JSONB,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS artifacts (
			id SERIAL PRIMARY KEY,
			spec_id INTEGER REFERENCES specs(id) ON DELETE CASCADE,
			artifact_type VARCHAR(20) NOT NULL, -- 'sdk', 'docs', 'changelog'
			language VARCHAR(50), -- for SDKs: 'go', 'python', 'typescript', etc.
			format VARCHAR(20), -- 'tar.gz', 'html', 'pdf', 'markdown'
			storage_path VARCHAR(500) NOT NULL,
			size_bytes BIGINT,
			version VARCHAR(100),
			metadata JSONB,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,

		`CREATE TABLE IF NOT EXISTS webhook_events (
			id SERIAL PRIMARY KEY,
			repo_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
			event_type VARCHAR(50) NOT NULL, -- 'pull_request', 'push', etc.
			payload JSONB NOT NULL,
			processed BOOLEAN DEFAULT FALSE,
			error_message TEXT,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
		)`,

		// Indexes for performance
		`CREATE INDEX IF NOT EXISTS idx_specs_repo_id ON specs(repo_id)`,
		`CREATE INDEX IF NOT EXISTS idx_specs_content_hash ON specs(content_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_changes_to_spec_id ON changes(to_spec_id)`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_spec_id ON artifacts(spec_id)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_events_repo_id ON webhook_events(repo_id)`,
		`CREATE INDEX IF NOT EXISTS idx_webhook_events_processed ON webhook_events(processed)`,
	}

	for _, migration := range migrations {
		if _, err := d.pool.Exec(context.Background(), migration); err != nil {
			return fmt.Errorf("failed to run migration: %w", err)
		}
	}

	log.Println("Database migrations completed successfully")
	return nil
}

func (d *Database) GetPool() *pgxpool.Pool {
	return d.pool
}
