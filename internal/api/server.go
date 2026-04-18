package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/specguard/specguard/internal/config"
	"github.com/specguard/specguard/internal/database"
	"github.com/specguard/specguard/internal/storage"
)

type Server struct {
	config  *config.Config
	db      *database.Database
	storage storage.Storage
	server  *http.Server
}

func New(cfg *config.Config, db *database.Database, store storage.Storage) *Server {
	return &Server{
		config:  cfg,
		db:      db,
		storage: store,
	}
}

func (s *Server) Start() error {
	router := gin.Default()

	// Middleware
	router.Use(gin.Logger())
	router.Use(gin.Recovery())
	router.Use(corsMiddleware())

	// API routes
	v1 := router.Group("/api/v1")
	{
		// Webhook endpoints
		v1.POST("/webhooks/github", s.handleGitHubWebhook)

		// Spec endpoints
		v1.GET("/specs", s.listSpecs)
		v1.GET("/specs/:id", s.getSpec)
		v1.POST("/specs", s.createSpec)

		// Change endpoints
		v1.GET("/changes", s.listChanges)
		v1.GET("/changes/:id", s.getChange)
		v1.GET("/specs/:specId/changes", s.getSpecChanges)

		// Artifact endpoints
		v1.GET("/artifacts", s.listArtifacts)
		v1.GET("/artifacts/:id", s.getArtifact)
		v1.GET("/artifacts/:id/download", s.downloadArtifact)

		// Repository endpoints
		v1.GET("/repositories", s.listRepositories)
		v1.POST("/repositories", s.createRepository)
		v1.GET("/repositories/:id", s.getRepository)

		// Health check
		v1.GET("/health", s.healthCheck)
	}

	s.server = &http.Server{
		Addr:         s.config.Server.Host + ":" + s.config.Server.Port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	}

	return s.server.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Health check endpoint
func (s *Server) healthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":    "healthy",
		"timestamp": time.Now().UTC(),
		"version":   "1.0.0",
	})
}

// Repository endpoints
func (s *Server) listRepositories(c *gin.Context) {
	query := `
		SELECT id, name, url, config, created_at, updated_at
		FROM repositories
		ORDER BY created_at DESC
	`

	rows, err := s.db.GetPool().Query(c.Request.Context(), query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var repos []map[string]interface{}
	for rows.Next() {
		var id int64
		var name, url string
		var configJSON []byte
		var createdAt, updatedAt time.Time

		err := rows.Scan(&id, &name, &url, &configJSON, &createdAt, &updatedAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var config map[string]interface{}
		if configJSON != nil {
			json.Unmarshal(configJSON, &config)
		}

		repos = append(repos, map[string]interface{}{
			"id":         id,
			"name":       name,
			"url":        url,
			"config":     config,
			"created_at": createdAt,
			"updated_at": updatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"repositories": repos})
}

func (s *Server) createRepository(c *gin.Context) {
	var req struct {
		Name   string                 `json:"name" binding:"required"`
		URL    string                 `json:"url" binding:"required"`
		Config map[string]interface{} `json:"config"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	query := `
		INSERT INTO repositories (name, url, config)
		VALUES ($1, $2, $3)
		RETURNING id, created_at, updated_at
	`

	var id int64
	var createdAt, updatedAt time.Time
	configBytes, _ := json.Marshal(req.Config)

	err := s.db.GetPool().QueryRow(c.Request.Context(), query,
		req.Name, req.URL, configBytes,
	).Scan(&id, &createdAt, &updatedAt)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"id":         id,
		"name":       req.Name,
		"url":        req.URL,
		"config":     req.Config,
		"created_at": createdAt,
		"updated_at": updatedAt,
	})
}

func (s *Server) getRepository(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid repository ID"})
		return
	}

	query := `
		SELECT id, name, url, config, created_at, updated_at
		FROM repositories
		WHERE id = $1
	`

	var name, url string
	var configJSON []byte
	var createdAt, updatedAt time.Time

	err = s.db.GetPool().QueryRow(c.Request.Context(), query, id).Scan(
		&id, &name, &url, &configJSON, &createdAt, &updatedAt,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Repository not found"})
		return
	}

	var config map[string]interface{}
	if configJSON != nil {
		json.Unmarshal(configJSON, &config)
	}

	c.JSON(http.StatusOK, gin.H{
		"id":         id,
		"name":       name,
		"url":        url,
		"config":     config,
		"created_at": createdAt,
		"updated_at": updatedAt,
	})
}

// Spec endpoints
func (s *Server) listSpecs(c *gin.Context) {
	repoIDStr := c.Query("repo_id")
	branch := c.Query("branch")

	query := `
		SELECT id, repo_id, version, branch, commit_hash, content_hash, spec_type, metadata, created_at
		FROM specs
		WHERE 1=1
	`
	args := []interface{}{}
	argIndex := 1

	if repoIDStr != "" {
		repoID, err := strconv.ParseInt(repoIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid repo_id"})
			return
		}
		query += " AND repo_id = $" + strconv.Itoa(argIndex)
		args = append(args, repoID)
		argIndex++
	}

	if branch != "" {
		query += " AND branch = $" + strconv.Itoa(argIndex)
		args = append(args, branch)
		argIndex++
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.db.GetPool().Query(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var specs []map[string]interface{}
	for rows.Next() {
		var id, repoID int64
		var version, branch, commitHash, contentHash, specType string
		var metadataJSON []byte
		var createdAt time.Time

		err := rows.Scan(&id, &repoID, &version, &branch, &commitHash, &contentHash,
			&specType, &metadataJSON, &createdAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var metadata map[string]interface{}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &metadata)
		}

		specs = append(specs, map[string]interface{}{
			"id":           id,
			"repo_id":      repoID,
			"version":      version,
			"branch":       branch,
			"commit_hash":  commitHash,
			"content_hash": contentHash,
			"spec_type":    specType,
			"metadata":     metadata,
			"created_at":   createdAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"specs": specs})
}

func (s *Server) getSpec(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid spec ID"})
		return
	}

	query := `
		SELECT id, repo_id, version, branch, commit_hash, content_hash, spec_type, content, metadata, created_at
		FROM specs
		WHERE id = $1
	`

	var repoID int64
	var version, branch, commitHash, contentHash, specType string
	var contentJSON, metadataJSON []byte
	var createdAt time.Time

	err = s.db.GetPool().QueryRow(c.Request.Context(), query, id).Scan(
		&id, &repoID, &version, &branch, &commitHash, &contentHash, &specType,
		&contentJSON, &metadataJSON, &createdAt,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Spec not found"})
		return
	}

	var content, metadata map[string]interface{}
	if contentJSON != nil {
		json.Unmarshal(contentJSON, &content)
	}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &metadata)
	}

	c.JSON(http.StatusOK, gin.H{
		"id":           id,
		"repo_id":      repoID,
		"version":      version,
		"branch":       branch,
		"commit_hash":  commitHash,
		"content_hash": contentHash,
		"spec_type":    specType,
		"content":      content,
		"metadata":     metadata,
		"created_at":   createdAt,
	})
}

func (s *Server) createSpec(c *gin.Context) {
	// This would typically be called internally or via webhook
	// For now, return not implemented
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Use webhook to create specs"})
}

// Change endpoints
func (s *Server) listChanges(c *gin.Context) {
	specIDStr := c.Query("spec_id")
	changeType := c.Query("change_type")

	query := `
		SELECT id, from_spec_id, to_spec_id, change_type, classification, path, description, ai_summary, impact_score, metadata, created_at
		FROM changes
		WHERE 1=1
	`
	args := []interface{}{}
	argIndex := 1

	if specIDStr != "" {
		specID, err := strconv.ParseInt(specIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid spec_id"})
			return
		}
		query += " AND to_spec_id = $" + strconv.Itoa(argIndex)
		args = append(args, specID)
		argIndex++
	}

	if changeType != "" {
		query += " AND change_type = $" + strconv.Itoa(argIndex)
		args = append(args, changeType)
		argIndex++
	}

	query += " ORDER BY impact_score DESC, created_at DESC"

	rows, err := s.db.GetPool().Query(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var changes []map[string]interface{}
	for rows.Next() {
		var id, fromSpecID, toSpecID int64
		var changeType, classification, path, description, aiSummary string
		var impactScore int
		var metadataJSON []byte
		var createdAt time.Time

		err := rows.Scan(&id, &fromSpecID, &toSpecID, &changeType, &classification,
			&path, &description, &aiSummary, &impactScore, &metadataJSON, &createdAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var metadata map[string]interface{}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &metadata)
		}

		changes = append(changes, map[string]interface{}{
			"id":             id,
			"from_spec_id":   fromSpecID,
			"to_spec_id":     toSpecID,
			"change_type":    changeType,
			"classification": classification,
			"path":           path,
			"description":    description,
			"ai_summary":     aiSummary,
			"impact_score":   impactScore,
			"metadata":       metadata,
			"created_at":     createdAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"changes": changes})
}

func (s *Server) getChange(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid change ID"})
		return
	}

	query := `
		SELECT id, from_spec_id, to_spec_id, change_type, classification, path, description, ai_summary, impact_score, metadata, created_at
		FROM changes
		WHERE id = $1
	`

	var fromSpecID, toSpecID int64
	var changeType, classification, path, description, aiSummary string
	var impactScore int
	var metadataJSON []byte
	var createdAt time.Time

	err = s.db.GetPool().QueryRow(c.Request.Context(), query, id).Scan(
		&id, &fromSpecID, &toSpecID, &changeType, &classification, &path,
		&description, &aiSummary, &impactScore, &metadataJSON, &createdAt,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Change not found"})
		return
	}

	var metadata map[string]interface{}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &metadata)
	}

	c.JSON(http.StatusOK, gin.H{
		"id":             id,
		"from_spec_id":   fromSpecID,
		"to_spec_id":     toSpecID,
		"change_type":    changeType,
		"classification": classification,
		"path":           path,
		"description":    description,
		"ai_summary":     aiSummary,
		"impact_score":   impactScore,
		"metadata":       metadata,
		"created_at":     createdAt,
	})
}

func (s *Server) getSpecChanges(c *gin.Context) {
	specIDStr := c.Param("specId")
	specID, err := strconv.ParseInt(specIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid spec ID"})
		return
	}

	query := `
		SELECT id, from_spec_id, to_spec_id, change_type, classification, path, description, ai_summary, impact_score, metadata, created_at
		FROM changes
		WHERE to_spec_id = $1
		ORDER BY impact_score DESC, created_at ASC
	`

	rows, err := s.db.GetPool().Query(c.Request.Context(), query, specID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var changes []map[string]interface{}
	for rows.Next() {
		var id, fromSpecID, toSpecID int64
		var changeType, classification, path, description, aiSummary string
		var impactScore int
		var metadataJSON []byte
		var createdAt time.Time

		err := rows.Scan(&id, &fromSpecID, &toSpecID, &changeType, &classification,
			&path, &description, &aiSummary, &impactScore, &metadataJSON, &createdAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var metadata map[string]interface{}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &metadata)
		}

		changes = append(changes, map[string]interface{}{
			"id":             id,
			"from_spec_id":   fromSpecID,
			"to_spec_id":     toSpecID,
			"change_type":    changeType,
			"classification": classification,
			"path":           path,
			"description":    description,
			"ai_summary":     aiSummary,
			"impact_score":   impactScore,
			"metadata":       metadata,
			"created_at":     createdAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"changes": changes})
}

// Artifact endpoints
func (s *Server) listArtifacts(c *gin.Context) {
	specIDStr := c.Query("spec_id")
	artifactType := c.Query("artifact_type")

	query := `
		SELECT id, spec_id, artifact_type, language, format, storage_path, size_bytes, version, metadata, created_at
		FROM artifacts
		WHERE 1=1
	`
	args := []interface{}{}
	argIndex := 1

	if specIDStr != "" {
		specID, err := strconv.ParseInt(specIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid spec_id"})
			return
		}
		query += " AND spec_id = $" + strconv.Itoa(argIndex)
		args = append(args, specID)
		argIndex++
	}

	if artifactType != "" {
		query += " AND artifact_type = $" + strconv.Itoa(argIndex)
		args = append(args, artifactType)
		argIndex++
	}

	query += " ORDER BY created_at DESC"

	rows, err := s.db.GetPool().Query(c.Request.Context(), query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var artifacts []map[string]interface{}
	for rows.Next() {
		var id, specID, sizeBytes int64
		var artifactType, language, format, storagePath, version string
		var metadataJSON []byte
		var createdAt time.Time

		err := rows.Scan(&id, &specID, &artifactType, &language, &format, &storagePath,
			&sizeBytes, &version, &metadataJSON, &createdAt)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		var metadata map[string]interface{}
		if metadataJSON != nil {
			json.Unmarshal(metadataJSON, &metadata)
		}

		artifacts = append(artifacts, map[string]interface{}{
			"id":            id,
			"spec_id":       specID,
			"artifact_type": artifactType,
			"language":      language,
			"format":        format,
			"storage_path":  storagePath,
			"size_bytes":    sizeBytes,
			"version":       version,
			"metadata":      metadata,
			"created_at":    createdAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{"artifacts": artifacts})
}

func (s *Server) getArtifact(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid artifact ID"})
		return
	}

	query := `
		SELECT id, spec_id, artifact_type, language, format, storage_path, size_bytes, version, metadata, created_at
		FROM artifacts
		WHERE id = $1
	`

	var specID, sizeBytes int64
	var artifactType, language, format, storagePath, version string
	var metadataJSON []byte
	var createdAt time.Time

	err = s.db.GetPool().QueryRow(c.Request.Context(), query, id).Scan(
		&id, &specID, &artifactType, &language, &format, &storagePath,
		&sizeBytes, &version, &metadataJSON, &createdAt,
	)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Artifact not found"})
		return
	}

	var metadata map[string]interface{}
	if metadataJSON != nil {
		json.Unmarshal(metadataJSON, &metadata)
	}

	c.JSON(http.StatusOK, gin.H{
		"id":            id,
		"spec_id":       specID,
		"artifact_type": artifactType,
		"language":      language,
		"format":        format,
		"storage_path":  storagePath,
		"size_bytes":    sizeBytes,
		"version":       version,
		"metadata":      metadata,
		"created_at":    createdAt,
	})
}

func (s *Server) downloadArtifact(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid artifact ID"})
		return
	}

	query := `
		SELECT storage_path, format
		FROM artifacts
		WHERE id = $1
	`

	var storagePath, format string
	err = s.db.GetPool().QueryRow(c.Request.Context(), query, id).Scan(&storagePath, &format)

	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Artifact not found"})
		return
	}

	// Stream from storage
	reader, err := s.storage.Retrieve(c.Request.Context(), storagePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve artifact"})
		return
	}
	defer reader.Close()

	// Set appropriate content type
	contentType := "application/octet-stream"
	switch format {
	case "tar.gz":
		contentType = "application/gzip"
	case "html":
		contentType = "text/html"
	case "pdf":
		contentType = "application/pdf"
	case "markdown":
		contentType = "text/markdown"
	}

	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", "attachment; filename=artifact."+format)

	// Stream the content
	_, err = io.Copy(c.Writer, reader)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to stream artifact"})
		return
	}
}

// GitHub webhook endpoint — HMAC-SHA256 verified
func (s *Server) handleGitHubWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to read body"})
		return
	}

	// Verify HMAC-SHA256 signature when secret is configured
	if s.cfg.GitHub.WebhookSecret != "" {
		sig := c.GetHeader("X-Hub-Signature-256")
		if sig == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing X-Hub-Signature-256"})
			return
		}
		mac := hmac.New(sha256.New, []byte(s.cfg.GitHub.WebhookSecret))
		mac.Write(body)
		expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		if !hmac.Equal([]byte(sig), []byte(expected)) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid signature"})
			return
		}
	}

	event := c.GetHeader("X-GitHub-Event")
	if event == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Missing X-GitHub-Event header"})
		return
	}

	if event == "ping" {
		c.JSON(http.StatusOK, gin.H{"message": "pong"})
		return
	}

	c.JSON(http.StatusAccepted, gin.H{"message": "Event received", "event": event})
}

// Middleware
func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Origin, Content-Type, Accept, Authorization")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
