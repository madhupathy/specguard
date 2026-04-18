package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/cobra"
	_ "modernc.org/sqlite"
)

func newServeCmd() *cobra.Command {
	var port string
	var host string

	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Start the SpecGuard API server (SQLite-backed, no Postgres needed)",
		RunE: func(cmd *cobra.Command, args []string) error {
			db, err := initSQLite()
			if err != nil {
				return fmt.Errorf("init db: %w", err)
			}
			defer db.Close()

			srv := &apiServer{db: db}
			return srv.start(host, port)
		},
	}

	cmd.Flags().StringVar(&port, "port", "8080", "Port to listen on")
	cmd.Flags().StringVar(&host, "host", "0.0.0.0", "Host to bind to")
	return cmd
}

// ---------------------------------------------------------------------------
// SQLite setup
// ---------------------------------------------------------------------------

func initSQLite() (*sql.DB, error) {
	db, err := sql.Open("sqlite", "file:specguard.db?cache=shared&mode=rwc&_journal_mode=WAL")
	if err != nil {
		return nil, err
	}

	migrations := []string{
		`CREATE TABLE IF NOT EXISTS repositories (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL UNIQUE,
			url TEXT NOT NULL,
			local_path TEXT,
			config TEXT,
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS specs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			repo_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
			version TEXT NOT NULL,
			branch TEXT,
			commit_hash TEXT,
			content_hash TEXT NOT NULL,
			spec_type TEXT NOT NULL,
			content TEXT,
			metadata TEXT,
			created_at TEXT DEFAULT (datetime('now')),
			UNIQUE(repo_id, version, branch)
		)`,
		`CREATE TABLE IF NOT EXISTS changes (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			from_spec_id INTEGER,
			to_spec_id INTEGER,
			change_type TEXT NOT NULL,
			classification TEXT NOT NULL,
			path TEXT NOT NULL,
			description TEXT,
			ai_summary TEXT,
			impact_score INTEGER DEFAULT 0,
			metadata TEXT,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS artifacts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			spec_id INTEGER,
			artifact_type TEXT NOT NULL,
			language TEXT,
			format TEXT,
			storage_path TEXT NOT NULL,
			size_bytes INTEGER,
			version TEXT,
			metadata TEXT,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS connectors (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT,
			status TEXT NOT NULL DEFAULT 'not_connected',
			auth_type TEXT,
			config TEXT,
			connected_at TEXT,
			created_at TEXT DEFAULT (datetime('now')),
			updated_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS documents (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			repo_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
			filename TEXT NOT NULL,
			doc_type TEXT NOT NULL,
			content TEXT,
			size_bytes INTEGER,
			chunk_count INTEGER DEFAULT 0,
			metadata TEXT,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS doc_chunks (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			doc_id INTEGER REFERENCES documents(id) ON DELETE CASCADE,
			repo_id INTEGER,
			chunk_index INTEGER NOT NULL,
			content TEXT NOT NULL,
			embedding TEXT,
			metadata TEXT,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
		`CREATE TABLE IF NOT EXISTS reports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			repo_id INTEGER REFERENCES repositories(id) ON DELETE CASCADE,
			report_type TEXT NOT NULL,
			title TEXT NOT NULL,
			content TEXT,
			content_md TEXT,
			summary TEXT,
			metadata TEXT,
			created_at TEXT DEFAULT (datetime('now'))
		)`,
	}

	for _, m := range migrations {
		if _, err := db.Exec(m); err != nil {
			return nil, fmt.Errorf("migration: %w", err)
		}
	}

	log.Println("SQLite database initialized")
	return db, nil
}

// ---------------------------------------------------------------------------
// API server
// ---------------------------------------------------------------------------

type apiServer struct {
	db *sql.DB
}

func (s *apiServer) start(host, port string) error {
	router := gin.Default()
	router.Use(gin.Recovery())
	router.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	v1 := router.Group("/api/v1")
	{
		v1.GET("/health", s.health)
		v1.GET("/repositories", s.listRepositories)
		v1.POST("/repositories", s.createRepository)
		v1.GET("/repositories/:id", s.getRepository)
		v1.POST("/repositories/:id/scan", s.scanRepository)
		v1.GET("/specs", s.listSpecs)
		v1.GET("/specs/:id", s.getSpec)
		v1.POST("/specs", s.createSpec)
		v1.POST("/specs/upload", s.uploadSpec)
		v1.GET("/specs/:id/changes", s.getSpecChanges)
		v1.GET("/changes", s.listChanges)
		v1.GET("/changes/:id", s.getChange)
		v1.GET("/artifacts", s.listArtifacts)
		v1.GET("/artifacts/:id", s.getArtifact)
		v1.GET("/artifacts/:id/download", s.downloadArtifact)

		// Connectors (LLM, Git)
		v1.GET("/connections", s.listConnections)
		v1.POST("/connections/:connector", s.saveConnection)
		v1.DELETE("/connections/:connector", s.deleteConnection)
		v1.POST("/connections/:connector/test", s.testConnection)

		// Documents (PDF/doc upload, chunking, RAG)
		v1.GET("/documents", s.listDocuments)
		v1.POST("/documents/upload", s.uploadDocument)
		v1.GET("/documents/:id", s.getDocument)
		v1.DELETE("/documents/:id", s.deleteDocument)
		v1.GET("/documents/:id/chunks", s.getDocumentChunks)

		// Reports
		v1.GET("/reports", s.listReports)
		v1.GET("/reports/:id", s.getReport)
		v1.POST("/repositories/:id/generate-reports", s.generateReports)

		// Swagger / OpenAPI viewer
		v1.GET("/repositories/:id/swagger", s.getSwagger)

		v1.POST("/webhooks/github", s.handleGitHubWebhook)
	}

	addr := host + ":" + port
	log.Printf("SpecGuard API server listening on %s", addr)

	srv := &http.Server{
		Addr:         addr,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}
	return srv.ListenAndServe()
}

// ---------------------------------------------------------------------------
// Handlers
// ---------------------------------------------------------------------------

func (s *apiServer) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "1.0.0",
	})
}

// --- Repositories ---

func (s *apiServer) listRepositories(c *gin.Context) {
	rows, err := s.db.Query(`SELECT id, name, url, local_path, config, created_at, updated_at FROM repositories ORDER BY created_at DESC`)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	repos := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int64
		var name, url string
		var localPath, cfgStr, createdAt, updatedAt sql.NullString
		if err := rows.Scan(&id, &name, &url, &localPath, &cfgStr, &createdAt, &updatedAt); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		var cfg interface{}
		if cfgStr.Valid {
			json.Unmarshal([]byte(cfgStr.String), &cfg)
		}
		repos = append(repos, map[string]interface{}{
			"id": id, "name": name, "url": url, "local_path": nullStr(localPath),
			"config": cfg, "created_at": createdAt.String, "updated_at": updatedAt.String,
		})
	}
	c.JSON(200, gin.H{"repositories": repos})
}

func (s *apiServer) createRepository(c *gin.Context) {
	var req struct {
		Name      string      `json:"name" binding:"required"`
		URL       string      `json:"url"`
		LocalPath string      `json:"local_path"`
		Config    interface{} `json:"config"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	if req.URL == "" && req.LocalPath == "" {
		c.JSON(400, gin.H{"error": "Either url or local_path is required"})
		return
	}
	if req.URL == "" {
		req.URL = "local://" + req.LocalPath
	}
	// Validate local path exists if provided
	if req.LocalPath != "" {
		if _, err := os.Stat(req.LocalPath); err != nil {
			c.JSON(400, gin.H{"error": fmt.Sprintf("Local path does not exist: %s", req.LocalPath)})
			return
		}
	}
	cfgBytes, _ := json.Marshal(req.Config)
	res, err := s.db.Exec(`INSERT INTO repositories (name, url, local_path, config) VALUES (?, ?, ?, ?)`,
		req.Name, req.URL, req.LocalPath, string(cfgBytes))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	now := time.Now().UTC().Format(time.RFC3339)
	c.JSON(201, gin.H{
		"id": id, "name": req.Name, "url": req.URL, "local_path": req.LocalPath,
		"config": req.Config, "created_at": now, "updated_at": now,
	})
}

func (s *apiServer) getRepository(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid repository ID"})
		return
	}
	var name, url string
	var localPath, cfgStr, createdAt, updatedAt sql.NullString
	err = s.db.QueryRow(`SELECT id, name, url, local_path, config, created_at, updated_at FROM repositories WHERE id = ?`, id).
		Scan(&id, &name, &url, &localPath, &cfgStr, &createdAt, &updatedAt)
	if err != nil {
		c.JSON(404, gin.H{"error": "Repository not found"})
		return
	}
	var cfg interface{}
	if cfgStr.Valid {
		json.Unmarshal([]byte(cfgStr.String), &cfg)
	}
	c.JSON(200, gin.H{
		"id": id, "name": name, "url": url, "local_path": nullStr(localPath),
		"config": cfg, "created_at": createdAt.String, "updated_at": updatedAt.String,
	})
}

// --- Specs ---

func (s *apiServer) listSpecs(c *gin.Context) {
	query := `SELECT id, repo_id, version, branch, commit_hash, content_hash, spec_type, metadata, created_at FROM specs WHERE 1=1`
	args := []interface{}{}

	if rid := c.Query("repo_id"); rid != "" {
		query += " AND repo_id = ?"
		args = append(args, rid)
	}
	if br := c.Query("branch"); br != "" {
		query += " AND branch = ?"
		args = append(args, br)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	specs := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, repoID int64
		var version, specType string
		var branch, commitHash, contentHash, metaStr, createdAt sql.NullString
		if err := rows.Scan(&id, &repoID, &version, &branch, &commitHash, &contentHash, &specType, &metaStr, &createdAt); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		var meta interface{}
		if metaStr.Valid {
			json.Unmarshal([]byte(metaStr.String), &meta)
		}
		specs = append(specs, map[string]interface{}{
			"id": id, "repo_id": repoID, "version": version, "branch": nullStr(branch),
			"commit_hash": nullStr(commitHash), "content_hash": nullStr(contentHash),
			"spec_type": specType, "metadata": meta, "created_at": nullStr(createdAt),
		})
	}
	c.JSON(200, gin.H{"specs": specs})
}

func (s *apiServer) getSpec(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid spec ID"})
		return
	}

	var repoID int64
	var version, specType string
	var branch, commitHash, contentHash, contentStr, metaStr, createdAt sql.NullString

	err = s.db.QueryRow(`SELECT id, repo_id, version, branch, commit_hash, content_hash, spec_type, content, metadata, created_at FROM specs WHERE id = ?`, id).
		Scan(&id, &repoID, &version, &branch, &commitHash, &contentHash, &specType, &contentStr, &metaStr, &createdAt)
	if err != nil {
		c.JSON(404, gin.H{"error": "Spec not found"})
		return
	}
	var content, meta interface{}
	if contentStr.Valid {
		json.Unmarshal([]byte(contentStr.String), &content)
	}
	if metaStr.Valid {
		json.Unmarshal([]byte(metaStr.String), &meta)
	}
	c.JSON(200, gin.H{
		"id": id, "repo_id": repoID, "version": version, "branch": nullStr(branch),
		"commit_hash": nullStr(commitHash), "content_hash": nullStr(contentHash),
		"spec_type": specType, "content": content, "metadata": meta, "created_at": nullStr(createdAt),
	})
}

func (s *apiServer) createSpec(c *gin.Context) {
	var req struct {
		RepoID      int64       `json:"repo_id" binding:"required"`
		Version     string      `json:"version" binding:"required"`
		Branch      string      `json:"branch"`
		CommitHash  string      `json:"commit_hash"`
		SpecType    string      `json:"spec_type" binding:"required"`
		Content     interface{} `json:"content"`
		ContentJSON string      `json:"content_json"`
		Metadata    interface{} `json:"metadata"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	var contentBytes []byte
	if req.ContentJSON != "" {
		contentBytes = []byte(req.ContentJSON)
	} else if req.Content != nil {
		contentBytes, _ = json.Marshal(req.Content)
	} else {
		contentBytes = []byte("{}")
	}

	hash := fmt.Sprintf("%x", sha256.Sum256(contentBytes))
	metaBytes, _ := json.Marshal(req.Metadata)

	res, err := s.db.Exec(`INSERT INTO specs (repo_id, version, branch, commit_hash, content_hash, spec_type, content, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		req.RepoID, req.Version, req.Branch, req.CommitHash, hash, req.SpecType, string(contentBytes), string(metaBytes))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	c.JSON(201, gin.H{
		"id": id, "repo_id": req.RepoID, "version": req.Version, "branch": req.Branch,
		"commit_hash": req.CommitHash, "content_hash": hash, "spec_type": req.SpecType,
		"created_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *apiServer) uploadSpec(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(400, gin.H{"error": "file is required"})
		return
	}
	defer file.Close()

	repoIDStr := c.PostForm("repo_id")
	repoID, err := strconv.ParseInt(repoIDStr, 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "repo_id is required"})
		return
	}
	version := c.PostForm("version")
	if version == "" {
		version = "1.0.0"
	}
	branch := c.PostForm("branch")
	specType := c.PostForm("spec_type")
	if specType == "" {
		if strings.HasSuffix(header.Filename, ".proto") || strings.Contains(header.Filename, "proto") {
			specType = "proto"
		} else {
			specType = "openapi"
		}
	}

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to read file"})
		return
	}

	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	meta, _ := json.Marshal(map[string]interface{}{
		"filename":   header.Filename,
		"size_bytes": len(data),
		"uploaded":   true,
	})

	res, err := s.db.Exec(`INSERT INTO specs (repo_id, version, branch, content_hash, spec_type, content, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		repoID, version, branch, hash, specType, string(data), string(meta))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	id, _ := res.LastInsertId()
	c.JSON(201, gin.H{
		"id": id, "repo_id": repoID, "version": version, "branch": branch,
		"content_hash": hash, "spec_type": specType, "filename": header.Filename,
		"size_bytes": len(data), "created_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *apiServer) scanRepository(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid repository ID"})
		return
	}

	var localPath sql.NullString
	var repoName string
	err = s.db.QueryRow(`SELECT name, local_path FROM repositories WHERE id = ?`, id).Scan(&repoName, &localPath)
	if err != nil {
		c.JSON(404, gin.H{"error": "Repository not found"})
		return
	}
	if !localPath.Valid || localPath.String == "" {
		c.JSON(400, gin.H{"error": "Repository has no local_path configured"})
		return
	}

	repoPath := localPath.String
	outDir := filepath.Join(repoPath, ".specguard", "out")
	if _, err := os.Stat(outDir); err != nil {
		c.JSON(400, gin.H{"error": fmt.Sprintf("No .specguard/out directory found at %s — run 'specguard scan' first", repoPath)})
		return
	}

	imported := []map[string]interface{}{}

	// Import snapshot specs (openapi.normalized.json, proto.normalized.json)
	snapshotDir := filepath.Join(outDir, "snapshot")
	if entries, err := os.ReadDir(snapshotDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() || entry.Name() == "manifest.json" {
				continue
			}
			if !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}

			data, err := os.ReadFile(filepath.Join(snapshotDir, entry.Name()))
			if err != nil {
				continue
			}

			specType := "openapi"
			if strings.Contains(entry.Name(), "proto") {
				specType = "proto"
			}

			hash := fmt.Sprintf("%x", sha256.Sum256(data))
			meta, _ := json.Marshal(map[string]interface{}{
				"source":     entry.Name(),
				"size_bytes": len(data),
				"scanned":    true,
			})

			res, err := s.db.Exec(`INSERT OR REPLACE INTO specs (repo_id, version, branch, content_hash, spec_type, content, metadata)
				VALUES (?, ?, ?, ?, ?, ?, ?)`,
				id, "latest", "main", hash, specType, string(data), string(meta))
			if err != nil {
				continue
			}
			specID, _ := res.LastInsertId()
			imported = append(imported, map[string]interface{}{
				"spec_id":    specID,
				"type":       specType,
				"file":       entry.Name(),
				"size_bytes": len(data),
			})
		}
	}

	// Import report_summary.json if present
	summaryPath := filepath.Join(outDir, "report_summary.json")
	if data, err := os.ReadFile(summaryPath); err == nil {
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		meta, _ := json.Marshal(map[string]interface{}{
			"source":  "report_summary.json",
			"scanned": true,
		})
		s.db.Exec(`INSERT INTO artifacts (spec_id, artifact_type, format, storage_path, size_bytes, version, metadata)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			nil, "report", "json", summaryPath, len(data), "latest", string(meta))
		_ = hash
	}

	// Import diff results if present
	diffDir := filepath.Join(outDir, "diff")
	if changesData, err := os.ReadFile(filepath.Join(diffDir, "changes.json")); err == nil {
		var changes []map[string]interface{}
		if err := json.Unmarshal(changesData, &changes); err == nil {
			for _, ch := range changes {
				changeType, _ := ch["kind"].(string)
				if changeType == "" {
					changeType, _ = ch["change_type"].(string)
				}
				if changeType == "" {
					changeType = "non_breaking"
				}
				classification, _ := ch["classification"].(string)
				if classification == "" {
					classification = "unknown"
				}
				path, _ := ch["path"].(string)
				desc, _ := ch["description"].(string)
				if desc == "" {
					desc, _ = ch["summary"].(string)
				}
				impact := 0
				if v, ok := ch["impact_score"].(float64); ok {
					impact = int(v)
				}
				s.db.Exec(`INSERT INTO changes (from_spec_id, to_spec_id, change_type, classification, path, description, impact_score)
					VALUES (?, ?, ?, ?, ?, ?, ?)`,
					nil, nil, changeType, classification, path, desc, impact)
			}
		}
	}

	c.JSON(200, gin.H{
		"message":  fmt.Sprintf("Scanned repository %q from %s", repoName, repoPath),
		"imported": imported,
		"count":    len(imported),
	})
}

func (s *apiServer) getSpecChanges(c *gin.Context) {
	specID := c.Param("id")
	rows, err := s.db.Query(`SELECT id, from_spec_id, to_spec_id, change_type, classification, path, description, ai_summary, impact_score, metadata, created_at
		FROM changes WHERE to_spec_id = ? ORDER BY impact_score DESC, created_at ASC`, specID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	c.JSON(200, gin.H{"changes": scanChanges(rows)})
}

// --- Changes ---

func (s *apiServer) listChanges(c *gin.Context) {
	query := `SELECT id, from_spec_id, to_spec_id, change_type, classification, path, description, ai_summary, impact_score, metadata, created_at FROM changes WHERE 1=1`
	args := []interface{}{}

	if sid := c.Query("spec_id"); sid != "" {
		query += " AND to_spec_id = ?"
		args = append(args, sid)
	}
	if ct := c.Query("change_type"); ct != "" {
		query += " AND change_type = ?"
		args = append(args, ct)
	}
	query += " ORDER BY impact_score DESC, created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	c.JSON(200, gin.H{"changes": scanChanges(rows)})
}

func (s *apiServer) getChange(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid change ID"})
		return
	}
	row := s.db.QueryRow(`SELECT id, from_spec_id, to_spec_id, change_type, classification, path, description, ai_summary, impact_score, metadata, created_at
		FROM changes WHERE id = ?`, id)

	var fromSpecID, toSpecID, impactScore int64
	var changeType, classification, path string
	var desc, aiSummary, metaStr, createdAt sql.NullString
	err = row.Scan(&id, &fromSpecID, &toSpecID, &changeType, &classification, &path, &desc, &aiSummary, &impactScore, &metaStr, &createdAt)
	if err != nil {
		c.JSON(404, gin.H{"error": "Change not found"})
		return
	}
	var meta interface{}
	if metaStr.Valid {
		json.Unmarshal([]byte(metaStr.String), &meta)
	}
	c.JSON(200, gin.H{
		"id": id, "from_spec_id": fromSpecID, "to_spec_id": toSpecID,
		"change_type": changeType, "classification": classification, "path": path,
		"description": nullStr(desc), "ai_summary": nullStr(aiSummary),
		"impact_score": impactScore, "metadata": meta, "created_at": nullStr(createdAt),
	})
}

// --- Artifacts ---

func (s *apiServer) listArtifacts(c *gin.Context) {
	query := `SELECT id, spec_id, artifact_type, language, format, storage_path, size_bytes, version, metadata, created_at FROM artifacts WHERE 1=1`
	args := []interface{}{}

	if sid := c.Query("spec_id"); sid != "" {
		query += " AND spec_id = ?"
		args = append(args, sid)
	}
	if at := c.Query("artifact_type"); at != "" {
		query += " AND artifact_type = ?"
		args = append(args, at)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	artifacts := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, specID int64
		var sizeBytes sql.NullInt64
		var artifactType string
		var lang, format, storagePath, version, metaStr, createdAt sql.NullString
		if err := rows.Scan(&id, &specID, &artifactType, &lang, &format, &storagePath, &sizeBytes, &version, &metaStr, &createdAt); err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		var meta interface{}
		if metaStr.Valid {
			json.Unmarshal([]byte(metaStr.String), &meta)
		}
		sb := int64(0)
		if sizeBytes.Valid {
			sb = sizeBytes.Int64
		}
		artifacts = append(artifacts, map[string]interface{}{
			"id": id, "spec_id": specID, "artifact_type": artifactType,
			"language": nullStr(lang), "format": nullStr(format),
			"storage_path": nullStr(storagePath), "size_bytes": sb,
			"version": nullStr(version), "metadata": meta, "created_at": nullStr(createdAt),
		})
	}
	c.JSON(200, gin.H{"artifacts": artifacts})
}

func (s *apiServer) getArtifact(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid artifact ID"})
		return
	}
	var specID int64
	var sizeBytes sql.NullInt64
	var artifactType string
	var lang, format, storagePath, version, metaStr, createdAt sql.NullString
	err = s.db.QueryRow(`SELECT id, spec_id, artifact_type, language, format, storage_path, size_bytes, version, metadata, created_at FROM artifacts WHERE id = ?`, id).
		Scan(&id, &specID, &artifactType, &lang, &format, &storagePath, &sizeBytes, &version, &metaStr, &createdAt)
	if err != nil {
		c.JSON(404, gin.H{"error": "Artifact not found"})
		return
	}
	var meta interface{}
	if metaStr.Valid {
		json.Unmarshal([]byte(metaStr.String), &meta)
	}
	sb := int64(0)
	if sizeBytes.Valid {
		sb = sizeBytes.Int64
	}
	c.JSON(200, gin.H{
		"id": id, "spec_id": specID, "artifact_type": artifactType,
		"language": nullStr(lang), "format": nullStr(format),
		"storage_path": nullStr(storagePath), "size_bytes": sb,
		"version": nullStr(version), "metadata": meta, "created_at": nullStr(createdAt),
	})
}

func (s *apiServer) downloadArtifact(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid artifact ID"})
		return
	}

	var storagePath, format sql.NullString
	err = s.db.QueryRow(
		`SELECT storage_path, format FROM artifacts WHERE id = ?`, id,
	).Scan(&storagePath, &format)
	if err == sql.ErrNoRows {
		c.JSON(http.StatusNotFound, gin.H{"error": "Artifact not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Database error"})
		return
	}
	if !storagePath.Valid || storagePath.String == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "Artifact has no stored file"})
		return
	}

	f, err := os.Open(storagePath.String)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Artifact file not found on disk"})
		return
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(storagePath.String))
	contentType := "application/octet-stream"
	switch ext {
	case ".json":
		contentType = "application/json"
	case ".md":
		contentType = "text/markdown"
	case ".html":
		contentType = "text/html"
	case ".pdf":
		contentType = "application/pdf"
	case ".gz", ".tar.gz":
		contentType = "application/gzip"
	case ".zip":
		contentType = "application/zip"
	}

	filename := filepath.Base(storagePath.String)
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", "attachment; filename=\""+filename+"\"")
	io.Copy(c.Writer, f)
}


// ---------------------------------------------------------------------------
// GitHub Webhook Handler
// ---------------------------------------------------------------------------

type githubWebhookPayload struct {
	Action      string `json:"action"`
	Number      int    `json:"number"`
	PullRequest struct {
		Number int    `json:"number"`
		Title  string `json:"title"`
		Head   struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
		Base struct {
			Ref string `json:"ref"`
		} `json:"base"`
	} `json:"pull_request"`
	Repository struct {
		FullName string `json:"full_name"`
		CloneURL string `json:"clone_url"`
	} `json:"repository"`
	Ref    string `json:"ref"`
	After  string `json:"after"`
	Before string `json:"before"`
}

func (s *apiServer) handleGitHubWebhook(c *gin.Context) {
	// 1. Read raw body for signature verification
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read request body"})
		return
	}

	// 2. Verify HMAC-SHA256 signature if secret is configured
	secret := os.Getenv("GITHUB_WEBHOOK_SECRET")
	if secret != "" {
		sig := c.GetHeader("X-Hub-Signature-256")
		if sig == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing X-Hub-Signature-256 header"})
			return
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(sig), []byte(expected)) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid webhook signature"})
			return
		}
	}

	// 3. Parse event type
	event := c.GetHeader("X-GitHub-Event")
	if event == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing X-GitHub-Event header"})
		return
	}

	// 4. Parse payload
	var payload githubWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid JSON payload"})
		return
	}

	// 5. Route by event type
	switch event {
	case "ping":
		c.JSON(http.StatusOK, gin.H{"message": "pong"})

	case "pull_request":
		if payload.Action == "opened" || payload.Action == "synchronize" || payload.Action == "reopened" {
			log.Printf(
				"GitHub webhook: PR #%d (%s) on %s — head=%s base=%s",
				payload.PullRequest.Number,
				payload.PullRequest.Title,
				payload.Repository.FullName,
				payload.PullRequest.Head.Ref,
				payload.PullRequest.Base.Ref,
			)
			// Record the event for future CI integration
			_, _ = s.db.Exec(
				`INSERT OR IGNORE INTO repositories (name, url) VALUES (?, ?)`,
				payload.Repository.FullName,
				payload.Repository.CloneURL,
			)
			c.JSON(http.StatusAccepted, gin.H{
				"message": "Pull request event received",
				"repo":    payload.Repository.FullName,
				"pr":      payload.PullRequest.Number,
				"action":  payload.Action,
			})
		} else {
			c.JSON(http.StatusOK, gin.H{"message": "Event ignored", "action": payload.Action})
		}

	case "push":
		if payload.Ref == "refs/heads/main" || payload.Ref == "refs/heads/master" {
			log.Printf(
				"GitHub webhook: push to %s on %s (after=%s)",
				payload.Ref, payload.Repository.FullName, payload.After,
			)
			_, _ = s.db.Exec(
				`INSERT OR IGNORE INTO repositories (name, url) VALUES (?, ?)`,
				payload.Repository.FullName,
				payload.Repository.CloneURL,
			)
			c.JSON(http.StatusAccepted, gin.H{
				"message": "Push event received",
				"repo":    payload.Repository.FullName,
				"ref":     payload.Ref,
				"after":   payload.After,
			})
		} else {
			c.JSON(http.StatusOK, gin.H{"message": "Push to non-default branch ignored"})
		}

	default:
		c.JSON(http.StatusOK, gin.H{"message": "Event type not handled", "event": event})
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func nullStr(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func scanChanges(rows *sql.Rows) []map[string]interface{} {
	changes := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, fromSpecID, toSpecID, impactScore int64
		var changeType, classification, path string
		var desc, aiSummary, metaStr, createdAt sql.NullString
		if err := rows.Scan(&id, &fromSpecID, &toSpecID, &changeType, &classification, &path, &desc, &aiSummary, &impactScore, &metaStr, &createdAt); err != nil {
			continue
		}
		var meta interface{}
		if metaStr.Valid {
			json.Unmarshal([]byte(metaStr.String), &meta)
		}
		changes = append(changes, map[string]interface{}{
			"id": id, "from_spec_id": fromSpecID, "to_spec_id": toSpecID,
			"change_type": changeType, "classification": classification, "path": path,
			"description": nullStr(desc), "ai_summary": nullStr(aiSummary),
			"impact_score": impactScore, "metadata": meta, "created_at": nullStr(createdAt),
		})
	}
	return changes
}
