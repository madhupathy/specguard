package spec_loader

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/specguard/specguard/internal/database"
	"github.com/ghodss/yaml"
)

type SpecType string

const (
	SpecTypeOpenAPI  SpecType = "openapi"
	SpecTypeProto    SpecType = "proto"
	SpecTypeAsyncAPI SpecType = "asyncapi"
)

type Spec struct {
	ID          int64                  `json:"id"`
	RepoID      int64                  `json:"repo_id"`
	Version     string                 `json:"version"`
	Branch      string                 `json:"branch"`
	CommitHash  string                 `json:"commit_hash"`
	ContentHash string                 `json:"content_hash"`
	SpecType    SpecType               `json:"spec_type"`
	Content     map[string]interface{} `json:"content"`
	Metadata    map[string]interface{} `json:"metadata"`
}

type Loader struct {
	db *database.Database
}

func New(db *database.Database) *Loader {
	return &Loader{db: db}
}

// LoadFromGitHub loads a spec from a GitHub repository
func (l *Loader) LoadFromGitHub(ctx context.Context, repoID int64, repoURL, version, branch, commitHash string, filePaths []string) (*Spec, error) {
	var combinedContent map[string]interface{}
	
	for _, filePath := range filePaths {
		content, err := l.fetchFileFromGitHub(ctx, repoURL, filePath, commitHash)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch file %s: %w", filePath, err)
		}

		parsedContent, err := l.parseSpec(content, filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to parse spec file %s: %w", filePath, err)
		}

		if combinedContent == nil {
			combinedContent = parsedContent
		} else {
			// Merge specs (for multi-file OpenAPI)
			combinedContent = l.mergeSpecs(combinedContent, parsedContent)
		}
	}

	// Calculate content hash
	contentBytes, _ := json.Marshal(combinedContent)
	hash := sha256.Sum256(contentBytes)
	contentHash := hex.EncodeToString(hash[:])

	spec := &Spec{
		RepoID:      repoID,
		Version:     version,
		Branch:      branch,
		CommitHash:  commitHash,
		ContentHash: contentHash,
		SpecType:    l.detectSpecType(filePaths[0]),
		Content:     combinedContent,
		Metadata: map[string]interface{}{
			"source_files": filePaths,
			"repo_url":     repoURL,
		},
	}

	return spec, nil
}

// LoadFromContent loads a spec from raw content
func (l *Loader) LoadFromContent(ctx context.Context, repoID int64, content []byte, filePath string) (*Spec, error) {
	parsedContent, err := l.parseSpec(content, filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse spec: %w", err)
	}

	hash := sha256.Sum256(content)
	contentHash := hex.EncodeToString(hash[:])

	return &Spec{
		RepoID:      repoID,
		ContentHash: contentHash,
		SpecType:    l.detectSpecType(filePath),
		Content:     parsedContent,
		Metadata: map[string]interface{}{
			"source_file": filePath,
		},
	}, nil
}

// Save saves the spec to the database
func (l *Loader) Save(ctx context.Context, spec *Spec) error {
	query := `
		INSERT INTO specs (repo_id, version, branch, commit_hash, content_hash, spec_type, content, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (repo_id, version, branch) 
		DO UPDATE SET 
			content_hash = EXCLUDED.content_hash,
			content = EXCLUDED.content,
			metadata = EXCLUDED.metadata,
			updated_at = NOW()
		RETURNING id
	`

	var id int64
	contentBytes, _ := json.Marshal(spec.Content)
	metadataBytes, _ := json.Marshal(spec.Metadata)

	err := l.db.GetPool().QueryRow(ctx, query,
		spec.RepoID, spec.Version, spec.Branch, spec.CommitHash,
		spec.ContentHash, spec.SpecType, contentBytes, metadataBytes,
	).Scan(&id)

	if err != nil {
		return fmt.Errorf("failed to save spec: %w", err)
	}

	spec.ID = id
	return nil
}

// GetLatest retrieves the latest spec for a repository
func (l *Loader) GetLatest(ctx context.Context, repoID int64, branch string) (*Spec, error) {
	query := `
		SELECT id, repo_id, version, branch, commit_hash, content_hash, spec_type, content, metadata
		FROM specs
		WHERE repo_id = $1 AND ($2 = '' OR branch = $2)
		ORDER BY created_at DESC
		LIMIT 1
	`

	var spec Spec
	var contentBytes, metadataBytes []byte

	err := l.db.GetPool().QueryRow(ctx, query, repoID, branch).Scan(
		&spec.ID, &spec.RepoID, &spec.Version, &spec.Branch, &spec.CommitHash,
		&spec.ContentHash, &spec.SpecType, &contentBytes, &metadataBytes,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get latest spec: %w", err)
	}

	if err := json.Unmarshal(contentBytes, &spec.Content); err != nil {
		return nil, fmt.Errorf("failed to unmarshal content: %w", err)
	}

	if err := json.Unmarshal(metadataBytes, &spec.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &spec, nil
}

// GetByVersion retrieves a spec by version
func (l *Loader) GetByVersion(ctx context.Context, repoID int64, version string) (*Spec, error) {
	query := `
		SELECT id, repo_id, version, branch, commit_hash, content_hash, spec_type, content, metadata
		FROM specs
		WHERE repo_id = $1 AND version = $2
		ORDER BY created_at DESC
		LIMIT 1
	`

	var spec Spec
	var contentBytes, metadataBytes []byte

	err := l.db.GetPool().QueryRow(ctx, query, repoID, version).Scan(
		&spec.ID, &spec.RepoID, &spec.Version, &spec.Branch, &spec.CommitHash,
		&spec.ContentHash, &spec.SpecType, &contentBytes, &metadataBytes,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get spec by version: %w", err)
	}

	if err := json.Unmarshal(contentBytes, &spec.Content); err != nil {
		return nil, fmt.Errorf("failed to unmarshal content: %w", err)
	}

	if err := json.Unmarshal(metadataBytes, &spec.Metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
	}

	return &spec, nil
}

// Helper methods

func (l *Loader) fetchFileFromGitHub(ctx context.Context, repoURL, filePath, commitHash string) ([]byte, error) {
	// Convert GitHub URL to raw content URL
	// Example: https://github.com/owner/repo -> https://raw.githubusercontent.com/owner/repo/commit/path
	rawURL := strings.Replace(repoURL, "github.com", "raw.githubusercontent.com", 1)
	rawURL = fmt.Sprintf("%s/%s/%s", rawURL, commitHash, filePath)

	resp, err := http.Get(rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch file: HTTP %d", resp.StatusCode)
	}

	content := make([]byte, resp.ContentLength)
	_, err = resp.Body.Read(content)
	return content, err
}

func (l *Loader) parseSpec(content []byte, filePath string) (map[string]interface{}, error) {
	var result map[string]interface{}
	
	ext := strings.ToLower(filepath.Ext(filePath))
	
	switch ext {
	case ".json":
		if err := json.Unmarshal(content, &result); err != nil {
			return nil, err
		}
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(content, &result); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported file extension: %s", ext)
	}

	// Validate OpenAPI spec if applicable
	if isOpenAPI(result) {
		loader := openapi3.NewLoader()
		doc, err := loader.LoadFromData(content)
		if err != nil {
			return nil, fmt.Errorf("invalid OpenAPI spec: %w", err)
		}
		
		// Convert back to map for consistent storage
		validated, err := doc.MarshalJSON()
		if err != nil {
			return nil, fmt.Errorf("failed to marshal validated OpenAPI: %w", err)
		}
		
		if err := json.Unmarshal(validated, &result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (l *Loader) detectSpecType(filePath string) SpecType {
	ext := strings.ToLower(filepath.Ext(filePath))
	
	switch ext {
	case ".proto":
		return SpecTypeProto
	case ".json", ".yaml", ".yml":
		// Further inspection needed for OpenAPI vs AsyncAPI
		return SpecTypeOpenAPI // Default for now
	default:
		return SpecTypeOpenAPI // Default
	}
}

func (l *Loader) mergeSpecs(base, addition map[string]interface{}) map[string]interface{} {
	// Simple merge strategy - in a real implementation, this would be more sophisticated
	result := make(map[string]interface{})
	
	// Copy base
	for k, v := range base {
		result[k] = v
	}
	
	// Add from addition
	for k, v := range addition {
		result[k] = v
	}
	
	return result
}

func isOpenAPI(content map[string]interface{}) bool {
	if swagger, ok := content["swagger"]; ok {
		return swagger == "2.0"
	}
	if openapi, ok := content["openapi"]; ok {
		return strings.HasPrefix(openapi.(string), "3.0")
	}
	return false
}
