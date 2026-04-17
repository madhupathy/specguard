package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// ---------------------------------------------------------------------------
// Connectors (LLM, Git) — similar to sonarfix-agent pattern
// ---------------------------------------------------------------------------

var defaultConnectors = map[string]map[string]string{
	"llm": {
		"name":        "LLM / AI Provider",
		"description": "OpenAI-compatible API for AI-powered recommendations and enrichment",
	},
	"git": {
		"name":        "Git / SCM",
		"description": "Source code repository access for cloning and scanning",
	},
}

func (s *apiServer) listConnections(c *gin.Context) {
	result := map[string]interface{}{}
	for id, def := range defaultConnectors {
		entry := map[string]interface{}{
			"name":        def["name"],
			"description": def["description"],
			"status":      "not_connected",
			"auth_type":   nil,
		}

		var status, authType, configStr, connectedAt sql.NullString
		err := s.db.QueryRow(`SELECT status, auth_type, config, connected_at FROM connectors WHERE id = ?`, id).
			Scan(&status, &authType, &configStr, &connectedAt)
		if err == nil {
			entry["status"] = status.String
			if authType.Valid {
				entry["auth_type"] = authType.String
			}
			if connectedAt.Valid {
				entry["connected_at"] = connectedAt.String
			}
			if configStr.Valid {
				var cfg map[string]interface{}
				json.Unmarshal([]byte(configStr.String), &cfg)
				// Mask secrets
				if _, ok := cfg["api_key"]; ok {
					cfg["api_key"] = "••••••••"
				}
				if _, ok := cfg["token"]; ok {
					cfg["token"] = "••••••••"
				}
				if _, ok := cfg["password"]; ok {
					cfg["password"] = "••••••••"
				}
				entry["config"] = cfg
			}
		}
		result[id] = entry
	}
	c.JSON(200, gin.H{"connections": result})
}

func (s *apiServer) saveConnection(c *gin.Context) {
	connectorID := c.Param("connector")
	if _, ok := defaultConnectors[connectorID]; !ok {
		c.JSON(400, gin.H{"error": fmt.Sprintf("Unknown connector: %s", connectorID)})
		return
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	authType, _ := req["auth_type"].(string)
	configBytes, _ := json.Marshal(req)
	now := time.Now().UTC().Format(time.RFC3339)

	_, err := s.db.Exec(`INSERT INTO connectors (id, name, description, status, auth_type, config, connected_at, updated_at)
		VALUES (?, ?, ?, 'connected', ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET status='connected', auth_type=?, config=?, connected_at=?, updated_at=?`,
		connectorID, defaultConnectors[connectorID]["name"], defaultConnectors[connectorID]["description"],
		authType, string(configBytes), now, now,
		authType, string(configBytes), now, now)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"status": "connected", "connector": connectorID})
}

func (s *apiServer) deleteConnection(c *gin.Context) {
	connectorID := c.Param("connector")
	s.db.Exec(`DELETE FROM connectors WHERE id = ?`, connectorID)
	c.JSON(200, gin.H{"status": "disconnected", "connector": connectorID})
}

func (s *apiServer) testConnection(c *gin.Context) {
	connectorID := c.Param("connector")
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}

	switch connectorID {
	case "llm":
		c.JSON(200, s.testLLMConnection(req))
	case "git":
		c.JSON(200, s.testGitConnection(req))
	default:
		c.JSON(400, gin.H{"error": fmt.Sprintf("Unknown connector: %s", connectorID)})
	}
}

func (s *apiServer) testLLMConnection(cfg map[string]interface{}) gin.H {
	apiKey, _ := cfg["api_key"].(string)
	if apiKey == "" {
		return gin.H{"success": false, "message": "API Key is required"}
	}
	baseURL, _ := cfg["base_url"].(string)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	baseURL = strings.TrimRight(baseURL, "/")
	model, _ := cfg["model"].(string)
	if model == "" {
		model = "gpt-4o-mini"
	}

	// Try a lightweight models list call
	client := &http.Client{Timeout: 10 * time.Second}
	reqBody, _ := json.Marshal(map[string]interface{}{
		"model":      model,
		"messages":   []map[string]string{{"role": "user", "content": "Say OK"}},
		"max_tokens": 5,
	})
	httpReq, _ := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(reqBody))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := client.Do(httpReq)
	if err != nil {
		return gin.H{"success": false, "message": fmt.Sprintf("Connection failed: %s", err.Error()[:200])}
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		return gin.H{"success": true, "message": fmt.Sprintf("Connected — model %s responding", model)}
	}
	body, _ := io.ReadAll(resp.Body)
	return gin.H{"success": false, "message": fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)[:200])}
}

func (s *apiServer) testGitConnection(cfg map[string]interface{}) gin.H {
	path, _ := cfg["path"].(string)
	if path == "" {
		return gin.H{"success": false, "message": "Repository path is required"}
	}
	info, err := os.Stat(path)
	if err != nil {
		return gin.H{"success": false, "message": fmt.Sprintf("Path not found: %s", path)}
	}
	if !info.IsDir() {
		return gin.H{"success": false, "message": "Path is not a directory"}
	}
	// Check for .git
	if _, err := os.Stat(filepath.Join(path, ".git")); err != nil {
		return gin.H{"success": true, "message": fmt.Sprintf("Directory exists but no .git found at %s", path)}
	}
	return gin.H{"success": true, "message": fmt.Sprintf("Git repository found at %s", path)}
}

// ---------------------------------------------------------------------------
// Documents (PDF/doc upload, chunking, RAG)
// ---------------------------------------------------------------------------

func (s *apiServer) listDocuments(c *gin.Context) {
	query := `SELECT id, repo_id, filename, doc_type, size_bytes, chunk_count, metadata, created_at FROM documents WHERE 1=1`
	args := []interface{}{}
	if rid := c.Query("repo_id"); rid != "" {
		query += " AND repo_id = ?"
		args = append(args, rid)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	docs := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, repoID int64
		var sizeBytes sql.NullInt64
		var chunkCount int64
		var filename, docType string
		var metaStr, createdAt sql.NullString
		if err := rows.Scan(&id, &repoID, &filename, &docType, &sizeBytes, &chunkCount, &metaStr, &createdAt); err != nil {
			continue
		}
		var meta interface{}
		if metaStr.Valid {
			json.Unmarshal([]byte(metaStr.String), &meta)
		}
		sb := int64(0)
		if sizeBytes.Valid {
			sb = sizeBytes.Int64
		}
		docs = append(docs, map[string]interface{}{
			"id": id, "repo_id": repoID, "filename": filename, "doc_type": docType,
			"size_bytes": sb, "chunk_count": chunkCount, "metadata": meta,
			"created_at": nullStr(createdAt),
		})
	}
	c.JSON(200, gin.H{"documents": docs})
}

func (s *apiServer) uploadDocument(c *gin.Context) {
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

	docType := c.PostForm("doc_type")
	if docType == "" {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		switch ext {
		case ".pdf":
			docType = "pdf"
		case ".md", ".markdown":
			docType = "markdown"
		case ".txt":
			docType = "text"
		case ".json":
			docType = "json"
		case ".yaml", ".yml":
			docType = "yaml"
		default:
			docType = "text"
		}
	}

	data, err := io.ReadAll(file)
	if err != nil {
		c.JSON(500, gin.H{"error": "Failed to read file"})
		return
	}

	// Store the document
	meta, _ := json.Marshal(map[string]interface{}{
		"filename":     header.Filename,
		"content_hash": fmt.Sprintf("%x", sha256.Sum256(data)),
		"uploaded":     true,
	})

	res, err := s.db.Exec(`INSERT INTO documents (repo_id, filename, doc_type, content, size_bytes, metadata)
		VALUES (?, ?, ?, ?, ?, ?)`,
		repoID, header.Filename, docType, string(data), len(data), string(meta))
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	docID, _ := res.LastInsertId()

	// Chunk the document for RAG
	chunks := chunkText(string(data), header.Filename, docType)
	for i, chunk := range chunks {
		chunkMeta, _ := json.Marshal(map[string]interface{}{
			"source":      header.Filename,
			"chunk_index": i,
			"char_count":  len(chunk),
		})
		s.db.Exec(`INSERT INTO doc_chunks (doc_id, repo_id, chunk_index, content, metadata)
			VALUES (?, ?, ?, ?, ?)`, docID, repoID, i, chunk, string(chunkMeta))
	}

	// Update chunk count
	s.db.Exec(`UPDATE documents SET chunk_count = ? WHERE id = ?`, len(chunks), docID)

	c.JSON(201, gin.H{
		"id": docID, "repo_id": repoID, "filename": header.Filename,
		"doc_type": docType, "size_bytes": len(data), "chunk_count": len(chunks),
		"created_at": time.Now().UTC().Format(time.RFC3339),
	})
}

func (s *apiServer) getDocument(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid document ID"})
		return
	}
	var repoID int64
	var sizeBytes sql.NullInt64
	var chunkCount int64
	var filename, docType string
	var content, metaStr, createdAt sql.NullString
	err = s.db.QueryRow(`SELECT id, repo_id, filename, doc_type, content, size_bytes, chunk_count, metadata, created_at FROM documents WHERE id = ?`, id).
		Scan(&id, &repoID, &filename, &docType, &content, &sizeBytes, &chunkCount, &metaStr, &createdAt)
	if err != nil {
		c.JSON(404, gin.H{"error": "Document not found"})
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
		"id": id, "repo_id": repoID, "filename": filename, "doc_type": docType,
		"content": nullStr(content), "size_bytes": sb, "chunk_count": chunkCount,
		"metadata": meta, "created_at": nullStr(createdAt),
	})
}

func (s *apiServer) deleteDocument(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid document ID"})
		return
	}
	s.db.Exec(`DELETE FROM doc_chunks WHERE doc_id = ?`, id)
	s.db.Exec(`DELETE FROM documents WHERE id = ?`, id)
	c.JSON(200, gin.H{"status": "deleted", "id": id})
}

func (s *apiServer) getDocumentChunks(c *gin.Context) {
	docID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid document ID"})
		return
	}
	rows, err := s.db.Query(`SELECT id, chunk_index, content, metadata, created_at FROM doc_chunks WHERE doc_id = ? ORDER BY chunk_index`, docID)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	chunks := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id int64
		var chunkIdx int
		var content string
		var metaStr, createdAt sql.NullString
		if err := rows.Scan(&id, &chunkIdx, &content, &metaStr, &createdAt); err != nil {
			continue
		}
		var meta interface{}
		if metaStr.Valid {
			json.Unmarshal([]byte(metaStr.String), &meta)
		}
		chunks = append(chunks, map[string]interface{}{
			"id": id, "chunk_index": chunkIdx, "content": content,
			"char_count": len(content), "metadata": meta, "created_at": nullStr(createdAt),
		})
	}
	c.JSON(200, gin.H{"chunks": chunks, "count": len(chunks)})
}

// ---------------------------------------------------------------------------
// Reports
// ---------------------------------------------------------------------------

func (s *apiServer) listReports(c *gin.Context) {
	query := `SELECT id, repo_id, report_type, title, summary, metadata, created_at FROM reports WHERE 1=1`
	args := []interface{}{}
	if rid := c.Query("repo_id"); rid != "" {
		query += " AND repo_id = ?"
		args = append(args, rid)
	}
	if rt := c.Query("report_type"); rt != "" {
		query += " AND report_type = ?"
		args = append(args, rt)
	}
	query += " ORDER BY created_at DESC"

	rows, err := s.db.Query(query, args...)
	if err != nil {
		c.JSON(500, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	reports := make([]map[string]interface{}, 0)
	for rows.Next() {
		var id, repoID int64
		var reportType, title string
		var summary, metaStr, createdAt sql.NullString
		if err := rows.Scan(&id, &repoID, &reportType, &title, &summary, &metaStr, &createdAt); err != nil {
			continue
		}
		var meta interface{}
		if metaStr.Valid {
			json.Unmarshal([]byte(metaStr.String), &meta)
		}
		reports = append(reports, map[string]interface{}{
			"id": id, "repo_id": repoID, "report_type": reportType, "title": title,
			"summary": nullStr(summary), "metadata": meta, "created_at": nullStr(createdAt),
		})
	}
	c.JSON(200, gin.H{"reports": reports})
}

func (s *apiServer) getReport(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid report ID"})
		return
	}
	var repoID int64
	var reportType, title string
	var content, contentMD, summary, metaStr, createdAt sql.NullString
	err = s.db.QueryRow(`SELECT id, repo_id, report_type, title, content, content_md, summary, metadata, created_at FROM reports WHERE id = ?`, id).
		Scan(&id, &repoID, &reportType, &title, &content, &contentMD, &summary, &metaStr, &createdAt)
	if err != nil {
		c.JSON(404, gin.H{"error": "Report not found"})
		return
	}
	var meta interface{}
	if metaStr.Valid {
		json.Unmarshal([]byte(metaStr.String), &meta)
	}
	// Try to parse content as JSON
	var contentParsed interface{}
	if content.Valid {
		if err := json.Unmarshal([]byte(content.String), &contentParsed); err != nil {
			contentParsed = content.String
		}
	}
	c.JSON(200, gin.H{
		"id": id, "repo_id": repoID, "report_type": reportType, "title": title,
		"content": contentParsed, "content_md": nullStr(contentMD),
		"summary": nullStr(summary), "metadata": meta, "created_at": nullStr(createdAt),
	})
}

func (s *apiServer) generateReports(c *gin.Context) {
	repoID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid repository ID"})
		return
	}

	var localPath sql.NullString
	var repoName string
	err = s.db.QueryRow(`SELECT name, local_path FROM repositories WHERE id = ?`, repoID).Scan(&repoName, &localPath)
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
	reportsDir := filepath.Join(outDir, "reports")

	// Delete old reports for this repo
	s.db.Exec(`DELETE FROM reports WHERE repo_id = ?`, repoID)

	imported := []map[string]interface{}{}

	// Import all markdown reports
	reportFiles := []struct {
		filename   string
		reportType string
		title      string
	}{
		{"risk.md", "risk", "Risk Assessment"},
		{"standards.md", "standards", "Standards Compliance"},
		{"doc_consistency.md", "doc_consistency", "Documentation Consistency"},
		{"protocol_recommendation.md", "protocol_recommendation", "Protocol Recommendations (SOAF)"},
		{"enrichment_summary.md", "enrichment", "Swagger Enrichment Summary"},
	}

	for _, rf := range reportFiles {
		mdPath := filepath.Join(reportsDir, rf.filename)
		data, err := os.ReadFile(mdPath)
		if err != nil {
			continue
		}

		// Extract a summary (first few lines)
		summary := extractSummary(string(data))

		// Check for corresponding JSON
		jsonPath := strings.TrimSuffix(mdPath, ".md") + ".json"
		var jsonContent string
		if jd, err := os.ReadFile(jsonPath); err == nil {
			jsonContent = string(jd)
		}

		meta, _ := json.Marshal(map[string]interface{}{
			"source_file": rf.filename,
			"size_bytes":  len(data),
			"has_json":    jsonContent != "",
		})

		res, err := s.db.Exec(`INSERT INTO reports (repo_id, report_type, title, content, content_md, summary, metadata)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			repoID, rf.reportType, rf.title, jsonContent, string(data), summary, string(meta))
		if err != nil {
			continue
		}
		id, _ := res.LastInsertId()
		imported = append(imported, map[string]interface{}{
			"id": id, "report_type": rf.reportType, "title": rf.title,
		})
	}

	// Import report_summary.json
	summaryPath := filepath.Join(outDir, "report_summary.json")
	if data, err := os.ReadFile(summaryPath); err == nil {
		meta, _ := json.Marshal(map[string]interface{}{"source_file": "report_summary.json"})
		res, err := s.db.Exec(`INSERT INTO reports (repo_id, report_type, title, content, summary, metadata)
			VALUES (?, ?, ?, ?, ?, ?)`,
			repoID, "summary", "Report Summary", string(data), "Overall report summary with totals and warnings", string(meta))
		if err == nil {
			id, _ := res.LastInsertId()
			imported = append(imported, map[string]interface{}{
				"id": id, "report_type": "summary", "title": "Report Summary",
			})
		}
	}

	// Import knowledge_model.json
	kmPath := filepath.Join(outDir, "knowledge_model.json")
	if data, err := os.ReadFile(kmPath); err == nil {
		meta, _ := json.Marshal(map[string]interface{}{"source_file": "knowledge_model.json"})
		res, err := s.db.Exec(`INSERT INTO reports (repo_id, report_type, title, content, summary, metadata)
			VALUES (?, ?, ?, ?, ?, ?)`,
			repoID, "knowledge_model", "Knowledge Model", string(data), "Enriched API knowledge model with doc context", string(meta))
		if err == nil {
			id, _ := res.LastInsertId()
			imported = append(imported, map[string]interface{}{
				"id": id, "report_type": "knowledge_model", "title": "Knowledge Model",
			})
		}
	}

	// Import doc_index chunks into doc_chunks table
	chunksPath := filepath.Join(outDir, "doc_index", "chunks.jsonl")
	if f, err := os.Open(chunksPath); err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
		idx := 0
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			var chunk map[string]interface{}
			if err := json.Unmarshal([]byte(line), &chunk); err != nil {
				continue
			}
			text, _ := chunk["text"].(string)
			if text == "" {
				text, _ = chunk["content"].(string)
			}
			if text == "" {
				continue
			}
			source, _ := chunk["source"].(string)
			chunkMeta, _ := json.Marshal(chunk)

			// Check if we already have a document for this source
			var docID int64
			err := s.db.QueryRow(`SELECT id FROM documents WHERE repo_id = ? AND filename = ?`, repoID, source).Scan(&docID)
			if err != nil {
				// Create a doc entry for this source
				res, err := s.db.Exec(`INSERT INTO documents (repo_id, filename, doc_type, size_bytes, metadata)
					VALUES (?, ?, 'indexed', 0, '{}')`, repoID, source)
				if err != nil {
					continue
				}
				docID, _ = res.LastInsertId()
			}

			s.db.Exec(`INSERT INTO doc_chunks (doc_id, repo_id, chunk_index, content, metadata)
				VALUES (?, ?, ?, ?, ?)`, docID, repoID, idx, text, string(chunkMeta))
			idx++
		}
		// Update chunk counts
		s.db.Exec(`UPDATE documents SET chunk_count = (SELECT COUNT(*) FROM doc_chunks WHERE doc_chunks.doc_id = documents.id) WHERE repo_id = ?`, repoID)
	}

	// Also import specs if not already done
	s.importSpecsFromDisk(repoID, outDir)

	c.JSON(200, gin.H{
		"message":  fmt.Sprintf("Generated reports for %q", repoName),
		"imported": imported,
		"count":    len(imported),
	})
}

func (s *apiServer) importSpecsFromDisk(repoID int64, outDir string) {
	snapshotDir := filepath.Join(outDir, "snapshot")
	specFiles := []struct {
		filename string
		specType string
	}{
		{"openapi.normalized.json", "openapi"},
		{"proto.normalized.json", "proto"},
	}
	for _, sf := range specFiles {
		data, err := os.ReadFile(filepath.Join(snapshotDir, sf.filename))
		if err != nil {
			continue
		}
		hash := fmt.Sprintf("%x", sha256.Sum256(data))
		meta, _ := json.Marshal(map[string]interface{}{
			"source": sf.filename, "size_bytes": len(data), "scanned": true,
		})
		// Use spec_type as version discriminator to avoid UNIQUE collision
		s.db.Exec(`INSERT OR REPLACE INTO specs (repo_id, version, branch, content_hash, spec_type, content, metadata)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			repoID, "latest-"+sf.specType, "main", hash, sf.specType, string(data), string(meta))
	}
}

// ---------------------------------------------------------------------------
// Swagger / OpenAPI viewer
// ---------------------------------------------------------------------------

func (s *apiServer) getSwagger(c *gin.Context) {
	repoID, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(400, gin.H{"error": "Invalid repository ID"})
		return
	}

	// Try to get from specs table first
	var contentStr sql.NullString
	err = s.db.QueryRow(`SELECT content FROM specs WHERE repo_id = ? AND spec_type = 'openapi' ORDER BY created_at DESC LIMIT 1`, repoID).Scan(&contentStr)
	if err == nil && contentStr.Valid {
		var parsed interface{}
		if err := json.Unmarshal([]byte(contentStr.String), &parsed); err == nil {
			c.JSON(200, gin.H{"swagger": parsed, "source": "database"})
			return
		}
	}

	// Fallback: read from local path
	var localPath sql.NullString
	s.db.QueryRow(`SELECT local_path FROM repositories WHERE id = ?`, repoID).Scan(&localPath)
	if localPath.Valid && localPath.String != "" {
		openapiPath := filepath.Join(localPath.String, ".specguard", "out", "snapshot", "openapi.normalized.json")
		if data, err := os.ReadFile(openapiPath); err == nil {
			var parsed interface{}
			if err := json.Unmarshal(data, &parsed); err == nil {
				c.JSON(200, gin.H{"swagger": parsed, "source": "local_file"})
				return
			}
		}
	}

	c.JSON(404, gin.H{"error": "No OpenAPI spec found for this repository"})
}

// ---------------------------------------------------------------------------
// LLM helper — get config from DB
// ---------------------------------------------------------------------------

func (s *apiServer) getLLMConfig() (apiKey, baseURL, model string, ok bool) {
	var configStr sql.NullString
	err := s.db.QueryRow(`SELECT config FROM connectors WHERE id = 'llm' AND status = 'connected'`).Scan(&configStr)
	if err != nil || !configStr.Valid {
		return "", "", "", false
	}
	var cfg map[string]interface{}
	json.Unmarshal([]byte(configStr.String), &cfg)
	apiKey, _ = cfg["api_key"].(string)
	baseURL, _ = cfg["base_url"].(string)
	model, _ = cfg["model"].(string)
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return apiKey, baseURL, model, apiKey != ""
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func chunkText(text, filename, docType string) []string {
	if docType == "pdf" {
		// For PDFs we just store the raw content as-is since we can't parse binary PDF in Go easily
		// The user should upload text-extracted content or use the CLI doc_index
		if len(text) > 2000 {
			return chunkBySize(text, 1500)
		}
		return []string{text}
	}

	// For text/markdown, chunk by paragraphs or fixed size
	paragraphs := strings.Split(text, "\n\n")
	if len(paragraphs) <= 1 {
		return chunkBySize(text, 1500)
	}

	var chunks []string
	current := ""
	for _, p := range paragraphs {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(current)+len(p) > 1500 {
			if current != "" {
				chunks = append(chunks, strings.TrimSpace(current))
			}
			current = p
		} else {
			if current != "" {
				current += "\n\n"
			}
			current += p
		}
	}
	if current != "" {
		chunks = append(chunks, strings.TrimSpace(current))
	}
	if len(chunks) == 0 {
		chunks = []string{text}
	}
	return chunks
}

func chunkBySize(text string, size int) []string {
	var chunks []string
	for len(text) > size {
		// Find a good break point
		cut := size
		if idx := strings.LastIndex(text[:size], "\n"); idx > size/2 {
			cut = idx + 1
		} else if idx := strings.LastIndex(text[:size], ". "); idx > size/2 {
			cut = idx + 2
		}
		chunks = append(chunks, strings.TrimSpace(text[:cut]))
		text = text[cut:]
	}
	if strings.TrimSpace(text) != "" {
		chunks = append(chunks, strings.TrimSpace(text))
	}
	return chunks
}

func extractSummary(md string) string {
	lines := strings.Split(md, "\n")
	var summary []string
	count := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "| ---") || strings.HasPrefix(line, "| #") {
			continue
		}
		if strings.HasPrefix(line, "Generated:") || strings.HasPrefix(line, "Overall") || strings.HasPrefix(line, "Rules checked") ||
			strings.HasPrefix(line, "Issues found") || strings.HasPrefix(line, "Total endpoints") ||
			strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "Violations") {
			summary = append(summary, line)
			count++
			if count >= 6 {
				break
			}
		}
	}
	return strings.Join(summary, "\n")
}
