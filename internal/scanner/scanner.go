package scanner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specguard/specguard/internal/database"
	"github.com/specguard/specguard/internal/spec_loader"
	"github.com/specguard/specguard/internal/storage"
)

type Scanner struct {
	db      *database.Database
	storage storage.Storage
}

type ScanResult struct {
	RESTSpecs []spec_loader.Spec `json:"rest_specs"`
	GRPCSpecs []spec_loader.Spec `json:"grpc_specs"`
	Protobufs []ProtobufFile     `json:"protobufs"`
	Drift     []DriftIssue       `json:"drift"`
}

type ProtobufFile struct {
	Path     string    `json:"path"`
	Content  string    `json:"content"`
	Services []Service `json:"services"`
}

type Service struct {
	Name    string   `json:"name"`
	Methods []Method `json:"methods"`
}

type Method struct {
	Name    string   `json:"name"`
	Input   string   `json:"input"`
	Output  string   `json:"output"`
	Options []string `json:"options"`
}

type DriftIssue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Path        string `json:"path"`
}

func New(db *database.Database, storage storage.Storage) *Scanner {
	return &Scanner{
		db:      db,
		storage: storage,
	}
}

func (s *Scanner) ScanRepository(repoPath string) (*ScanResult, error) {
	result := &ScanResult{}

	fmt.Printf("📁 Scanning repository: %s\n", repoPath)

	// Walk the repository to find API specs
	err := filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Skip common non-API directories
			dirName := strings.ToLower(filepath.Base(path))
			skipDirs := []string{".git", "node_modules", "vendor", "build", "dist", "target"}
			for _, skip := range skipDirs {
				if dirName == skip {
					return filepath.SkipDir
				}
			}
			return nil
		}

		// Check file extensions
		ext := strings.ToLower(filepath.Ext(path))
		baseName := strings.ToLower(filepath.Base(path))

		switch {
		case ext == ".json" && (strings.Contains(baseName, "openapi") || strings.Contains(baseName, "swagger") || strings.Contains(baseName, "api")):
			// OpenAPI JSON spec
			spec, err := s.loadOpenAPISpec(path)
			if err == nil {
				result.RESTSpecs = append(result.RESTSpecs, *spec)
			}

		case ext == ".yaml" || ext == ".yml":
			if strings.Contains(baseName, "openapi") || strings.Contains(baseName, "swagger") || strings.Contains(baseName, "api") {
				// OpenAPI YAML spec
				spec, err := s.loadOpenAPISpec(path)
				if err == nil {
					result.RESTSpecs = append(result.RESTSpecs, *spec)
				}
			}

		case ext == ".proto":
			// Protobuf file
			protoFile, err := s.loadProtobufFile(path)
			if err == nil {
				result.Protobufs = append(result.Protobufs, *protoFile)
			}

		case strings.Contains(baseName, "grpc") && (ext == ".json" || ext == ".yaml" || ext == ".yml"):
			// gRPC API spec
			spec, err := s.loadGRPCSpec(path)
			if err == nil {
				result.GRPCSpecs = append(result.GRPCSpecs, *spec)
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk repository: %w", err)
	}

	// Detect basic drift inconsistencies
	result.Drift = s.detectBasicDrift(result)

	return result, nil
}

func (s *Scanner) loadOpenAPISpec(path string) (*spec_loader.Spec, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	loader := spec_loader.New(s.db)
	spec, err := loader.LoadFromContent(context.Background(), 0, content, path)
	if err != nil {
		return nil, err
	}

	spec.Metadata = map[string]interface{}{
		"file_path": path,
		"spec_type": "openapi",
	}

	return spec, nil
}

func (s *Scanner) loadGRPCSpec(path string) (*spec_loader.Spec, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	loader := spec_loader.New(s.db)
	spec, err := loader.LoadFromContent(context.Background(), 0, content, path)
	if err != nil {
		return nil, err
	}

	spec.Metadata = map[string]interface{}{
		"file_path": path,
		"spec_type": "grpc",
	}

	return spec, nil
}

func (s *Scanner) loadProtobufFile(path string) (*ProtobufFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	protoFile := &ProtobufFile{
		Path:     path,
		Content:  string(content),
		Services: []Service{},
	}

	// Simple parsing for services and methods
	// In a real implementation, this would use proper protobuf parsing
	lines := strings.Split(string(content), "\n")
	var currentService *Service

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.HasPrefix(line, "service ") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				serviceName := strings.TrimSuffix(parts[1], "{")
				currentService = &Service{
					Name:    serviceName,
					Methods: []Method{},
				}
			}
		} else if strings.HasPrefix(line, "rpc ") && currentService != nil {
			// Parse RPC method
			method := s.parseRPCMethod(line)
			currentService.Methods = append(currentService.Methods, method)
		} else if line == "}" && currentService != nil {
			// End of service
			protoFile.Services = append(protoFile.Services, *currentService)
			currentService = nil
		}
	}

	return protoFile, nil
}

func (s *Scanner) parseRPCMethod(line string) Method {
	// Simple RPC parsing - would be more sophisticated in production
	parts := strings.Fields(line)
	method := Method{
		Name:    "",
		Input:   "",
		Output:  "",
		Options: []string{},
	}

	for i, part := range parts {
		switch part {
		case "rpc":
			if i+1 < len(parts) {
				method.Name = strings.TrimSuffix(parts[i+1], "(")
			}
		case "returns":
			if i+1 < len(parts) {
				method.Output = strings.TrimSuffix(parts[i+1], "(")
			}
		default:
			if strings.Contains(part, "(") && strings.Contains(part, ")") {
				// This is likely the input parameter
				method.Input = strings.Trim(strings.Trim(part, "()"), " ")
			}
		}
	}

	return method
}

func (s *Scanner) detectBasicDrift(result *ScanResult) []DriftIssue {
	var issues []DriftIssue

	// Check for REST/gRPC inconsistencies
	restEndpoints := s.extractRESTEndpoints(result.RESTSpecs)
	grpcServices := s.extractGRPCServices(result.GRPCSpecs)

	// Look for potential naming mismatches
	for restPath := range restEndpoints {
		grpcEquivalent := s.pathToGRPCService(restPath)
		if _, exists := grpcServices[grpcEquivalent]; !exists {
			issues = append(issues, DriftIssue{
				Type:        "naming_mismatch",
				Severity:    "info",
				Description: fmt.Sprintf("REST endpoint %s has no gRPC equivalent %s", restPath, grpcEquivalent),
				Path:        restPath,
			})
		}
	}

	return issues
}

func (s *Scanner) extractRESTEndpoints(specs []spec_loader.Spec) map[string][]string {
	endpoints := make(map[string][]string)

	for _, spec := range specs {
		if paths, ok := spec.Content["paths"].(map[string]interface{}); ok {
			for path := range paths {
				endpoints[path] = []string{}
				if pathInfo, ok := paths[path].(map[string]interface{}); ok {
					for method := range pathInfo {
						if s.isHTTPMethod(method) {
							endpoints[path] = append(endpoints[path], method)
						}
					}
				}
			}
		}
	}

	return endpoints
}

func (s *Scanner) extractGRPCServices(specs []spec_loader.Spec) map[string]bool {
	services := make(map[string]bool)

	for _, spec := range specs {
		// Extract service names from gRPC specs
		if spec.Metadata != nil {
			if file, ok := spec.Metadata["file_path"].(string); ok {
				base := filepath.Base(file)
				serviceName := strings.TrimSuffix(base, filepath.Ext(base))
				services[serviceName] = true
			}
		}
	}

	return services
}

func (s *Scanner) pathToGRPCService(path string) string {
	// Convert REST path to gRPC service name
	// Example: /api/v1/users -> UserService
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return "Service"
	}

	lastPart := parts[len(parts)-1]
	serviceName := strings.Title(lastPart) + "Service"

	return serviceName
}

func (s *Scanner) isHTTPMethod(method string) bool {
	methods := []string{"get", "post", "put", "delete", "patch", "head", "options"}
	for _, m := range methods {
		if strings.ToLower(method) == m {
			return true
		}
	}
	return false
}
