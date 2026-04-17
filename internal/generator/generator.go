package generator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specguard/specguard/internal/database"
	"github.com/specguard/specguard/internal/scanner"
	"github.com/specguard/specguard/internal/spec_loader"
	"github.com/specguard/specguard/internal/storage"
)

type Generator struct {
	db      *database.Database
	storage storage.Storage
}

func New(db *database.Database, storage storage.Storage) *Generator {
	return &Generator{
		db:      db,
		storage: storage,
	}
}

// GenerateDocumentation generates both REST and gRPC documentation
func (g *Generator) GenerateDocumentation(repoPath, outputPath string) error {
	fmt.Printf("📚 Generating documentation for: %s\n", repoPath)

	// Scan repository
	scanner := scanner.New(g.db, g.storage)
	scanResult, err := scanner.ScanRepository(repoPath)
	if err != nil {
		return fmt.Errorf("failed to scan repository: %w", err)
	}

	// Create output directories
	restDocsPath := filepath.Join(outputPath, "rest")
	grpcDocsPath := filepath.Join(outputPath, "grpc")

	if err := os.MkdirAll(restDocsPath, 0755); err != nil {
		return fmt.Errorf("failed to create REST docs directory: %w", err)
	}

	if err := os.MkdirAll(grpcDocsPath, 0755); err != nil {
		return fmt.Errorf("failed to create gRPC docs directory: %w", err)
	}

	// Generate REST API documentation
	if err := g.generateRESTDocs(scanResult.RESTSpecs, restDocsPath); err != nil {
		return fmt.Errorf("failed to generate REST docs: %w", err)
	}

	// Generate gRPC documentation
	if err := g.generateGRPCDocs(scanResult.GRPCSpecs, scanResult.Protobufs, grpcDocsPath); err != nil {
		return fmt.Errorf("failed to generate gRPC docs: %w", err)
	}

	// Generate unified index
	if err := g.generateUnifiedIndex(scanResult, outputPath); err != nil {
		return fmt.Errorf("failed to generate unified index: %w", err)
	}

	return nil
}

// GenerateSDKs generates SDKs for multiple languages
func (g *Generator) GenerateSDKs(repoPath, outputPath string) error {
	fmt.Printf("🔧 Generating SDKs for: %s\n", repoPath)

	// Scan repository
	scanner := scanner.New(g.db, g.storage)
	scanResult, err := scanner.ScanRepository(repoPath)
	if err != nil {
		return fmt.Errorf("failed to scan repository: %w", err)
	}

	// Generate SDKs for different languages
	languages := []string{"go", "python", "typescript", "java"}

	for _, lang := range languages {
		langPath := filepath.Join(outputPath, lang)
		if err := os.MkdirAll(langPath, 0755); err != nil {
			return fmt.Errorf("failed to create %s SDK directory: %w", lang, err)
		}

		if err := g.generateSDKForLanguage(scanResult, lang, langPath); err != nil {
			fmt.Printf("⚠️  Failed to generate %s SDK: %v\n", lang, err)
		} else {
			fmt.Printf("✅ Generated %s SDK\n", lang)
		}
	}

	return nil
}

// GenerateProtobuf generates protobuf files from OpenAPI specs
func (g *Generator) GenerateProtobuf(repoPath, outputPath string) error {
	fmt.Printf("📦 Generating protobuf files for: %s\n", repoPath)

	// Scan repository
	scanner := scanner.New(g.db, g.storage)
	scanResult, err := scanner.ScanRepository(repoPath)
	if err != nil {
		return fmt.Errorf("failed to scan repository: %w", err)
	}

	// Create output directory
	if err := os.MkdirAll(outputPath, 0755); err != nil {
		return fmt.Errorf("failed to create protobuf output directory: %w", err)
	}

	// Generate protobuf from REST specs
	for _, spec := range scanResult.RESTSpecs {
		if err := g.generateProtobufFromOpenAPI(spec, outputPath); err != nil {
			fmt.Printf("⚠️  Failed to generate protobuf from %s: %v\n", spec.Metadata["file_path"], err)
		}
	}

	// Copy existing protobuf files
	for _, protoFile := range scanResult.Protobufs {
		outputPath := filepath.Join(outputPath, filepath.Base(protoFile.Path))
		if err := os.WriteFile(outputPath, []byte(protoFile.Content), 0644); err != nil {
			return fmt.Errorf("failed to copy protobuf file %s: %w", protoFile.Path, err)
		}
	}

	return nil
}

func (g *Generator) generateRESTDocs(specs []spec_loader.Spec, outputPath string) error {
	if len(specs) == 0 {
		fmt.Printf("⚠️  No REST specs found\n")
		return nil
	}

	// Generate HTML documentation
	indexHTML := `<!DOCTYPE html>
<html>
<head>
    <title>API Documentation</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .endpoint { margin: 20px 0; padding: 15px; border: 1px solid #ddd; }
        .method { padding: 4px 8px; color: white; border-radius: 4px; font-size: 12px; }
        .get { background: #61affe; }
        .post { background: #49cc90; }
        .put { background: #fca130; }
        .delete { background: #f93e3e; }
    </style>
</head>
<body>
    <h1>REST API Documentation</h1>
`

	for _, spec := range specs {
		if paths, ok := spec.Content["paths"].(map[string]interface{}); ok {
			for path, pathInfo := range paths {
				if pathInfoMap, ok := pathInfo.(map[string]interface{}); ok {
					for method, methodInfo := range pathInfoMap {
						if methodInfoMap, ok := methodInfo.(map[string]interface{}); ok {
							description := ""
							if desc, ok := methodInfoMap["description"].(string); ok {
								description = desc
							}

							indexHTML += fmt.Sprintf(`
    <div class="endpoint">
        <h3><span class="method %s">%s</span> %s</h3>
        <p>%s</p>
    </div>`, strings.ToLower(method), strings.ToUpper(method), path, description)
						}
					}
				}
			}
		}
	}

	indexHTML += `
</body>
</html>`

	return os.WriteFile(filepath.Join(outputPath, "index.html"), []byte(indexHTML), 0644)
}

func (g *Generator) generateGRPCDocs(specs []spec_loader.Spec, protobufs []scanner.ProtobufFile, outputPath string) error {
	// Generate gRPC documentation
	indexHTML := `<!DOCTYPE html>
<html>
<head>
    <title>gRPC API Documentation</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .service { margin: 20px 0; padding: 15px; border: 1px solid #ddd; }
        .method { margin: 10px 0; padding: 10px; background: #f5f5f5; }
    </style>
</head>
<body>
    <h1>gRPC API Documentation</h1>
`

	for _, proto := range protobufs {
		indexHTML += fmt.Sprintf(`
    <div class="service">
        <h2>%s</h2>`, filepath.Base(proto.Path))

		for _, service := range proto.Services {
			indexHTML += fmt.Sprintf(`
        <h3>%s</h3>`, service.Name)

			for _, method := range service.Methods {
				indexHTML += fmt.Sprintf(`
        <div class="method">
            <h4>%s</h4>
            <p>Request: %s</p>
            <p>Response: %s</p>
        </div>`, method.Name, method.Input, method.Output)
			}
		}

		indexHTML += `    </div>`
	}

	indexHTML += `
</body>
</html>`

	return os.WriteFile(filepath.Join(outputPath, "index.html"), []byte(indexHTML), 0644)
}

func (g *Generator) generateUnifiedIndex(scanResult *scanner.ScanResult, outputPath string) error {
	indexHTML := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>API Documentation</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; }
        .summary { margin: 20px 0; padding: 15px; background: #f8f9fa; }
        .section { margin: 30px 0; }
        .link { display: inline-block; margin: 10px; padding: 10px 20px; background: #007bff; color: white; text-decoration: none; border-radius: 4px; }
    </style>
</head>
<body>
    <h1>API Documentation</h1>
    
    <div class="summary">
        <h2>Summary</h2>
        <p>REST Specs: %d</p>
        <p>gRPC Specs: %d</p>
        <p>Protobuf Files: %d</p>
        <p>Drift Issues: %d</p>
    </div>
    
    <div class="section">
        <h2>Documentation</h2>
        <a href="rest/index.html" class="link">REST API Documentation</a>
        <a href="grpc/index.html" class="link">gRPC API Documentation</a>
    </div>
</body>
</html>`, len(scanResult.RESTSpecs), len(scanResult.GRPCSpecs), len(scanResult.Protobufs), len(scanResult.Drift))

	return os.WriteFile(filepath.Join(outputPath, "index.html"), []byte(indexHTML), 0644)
}

func (g *Generator) generateSDKForLanguage(scanResult *scanner.ScanResult, language, outputPath string) error {
	// Basic SDK generation - in production, this would be more sophisticated
	switch language {
	case "go":
		return g.generateGoSDK(scanResult, outputPath)
	case "python":
		return g.generatePythonSDK(scanResult, outputPath)
	case "typescript":
		return g.generateTypeScriptSDK(scanResult, outputPath)
	case "java":
		return g.generateJavaSDK(scanResult, outputPath)
	default:
		return fmt.Errorf("unsupported language: %s", language)
	}
}

func (g *Generator) generateGoSDK(scanResult *scanner.ScanResult, outputPath string) error {
	// Generate basic Go SDK
	clientCode := `package api

import (
	"fmt"
	"net/http"
)

type Client struct {
	BaseURL string
	Client  *http.Client
}

func NewClient(baseURL string) *Client {
	return &Client{
		BaseURL: baseURL,
		Client:  &http.Client{},
	}
}

// Add generated methods here based on API specs
`

	return os.WriteFile(filepath.Join(outputPath, "client.go"), []byte(clientCode), 0644)
}

func (g *Generator) generatePythonSDK(scanResult *scanner.ScanResult, outputPath string) error {
	var clientCode = `import requests

class APIClient:
    def __init__(self, base_url):
        self.base_url = base_url
        self.session = requests.Session()
    
    # Add generated methods here based on API specs
`

	return os.WriteFile(filepath.Join(outputPath, "client.py"), []byte(clientCode), 0644)
}

func (g *Generator) generateTypeScriptSDK(scanResult *scanner.ScanResult, outputPath string) error {
	var clientCode = `export class APIClient {
    private baseUrl: string;
    
    constructor(baseUrl: string) {
        this.baseUrl = baseUrl;
    }
    
    // Add generated methods here based on API specs
}
`

	return os.WriteFile(filepath.Join(outputPath, "client.ts"), []byte(clientCode), 0644)
}

func (g *Generator) generateJavaSDK(scanResult *scanner.ScanResult, outputPath string) error {
	var clientCode = `package api;

import java.net.http.HttpClient;

public class APIClient {
    private String baseUrl;
    private HttpClient client;
    
    public APIClient(String baseUrl) {
        this.baseUrl = baseUrl;
        this.client = HttpClient.newHttpClient();
    }
    
    // Add generated methods here based on API specs
}
`

	return os.WriteFile(filepath.Join(outputPath, "APIClient.java"), []byte(clientCode), 0644)
}

func (g *Generator) generateProtobufFromOpenAPI(spec spec_loader.Spec, outputPath string) error {
	// Basic OpenAPI to protobuf conversion
	// In production, this would be more sophisticated
	var protoContent = `syntax = "proto3";

package api;

// Generated from OpenAPI spec
// Add message definitions based on OpenAPI schemas

service APIService {
    // Add service methods based on OpenAPI paths
}
`

	filename := fmt.Sprintf("generated_%s.proto", strings.ReplaceAll(filepath.Base(spec.Metadata["file_path"].(string)), ".", "_"))
	return os.WriteFile(filepath.Join(outputPath, filename), []byte(protoContent), 0644)
}
