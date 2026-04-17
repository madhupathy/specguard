package drift_detector

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/specguard/specguard/internal/database"
	"github.com/specguard/specguard/internal/scanner"
	"github.com/specguard/specguard/internal/spec_loader"
	"github.com/specguard/specguard/internal/storage"
)

type DriftDetector struct {
	db      *database.Database
	storage storage.Storage
	client  *http.Client
}

type DriftIssue struct {
	Type        string `json:"type"`
	Severity    string `json:"severity"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Expected    string `json:"expected,omitempty"`
	Actual      string `json:"actual,omitempty"`
}

type EnvironmentConfig struct {
	BaseURL string            `json:"base_url"`
	Headers map[string]string `json:"headers"`
	Auth    AuthConfig        `json:"auth"`
}

type AuthConfig struct {
	Type     string `json:"type"` // "bearer", "basic", "apikey"
	Token    string `json:"token"`
	Username string `json:"username"`
	Password string `json:"password"`
	APIKey   string `json:"apikey"`
}

func New(db *database.Database, storage storage.Storage) *DriftDetector {
	return &DriftDetector{
		db:      db,
		storage: storage,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (d *DriftDetector) DetectDrift(repoPath, compareWith string) ([]DriftIssue, error) {
	var issues []DriftIssue

	fmt.Printf("🔍 Detecting drift for repository: %s\n", repoPath)

	// Scan repository for specs
	scanner := scanner.New(d.db, d.storage)
	scanResult, err := scanner.ScanRepository(repoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to scan repository: %w", err)
	}

	// If comparing with live environment
	if compareWith != "" {
		envConfig := d.getEnvironmentConfig(compareWith)

		// Detect REST API drift
		restIssues, err := d.detectRESTDrift(scanResult.RESTSpecs, envConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to detect REST drift: %w", err)
		}
		issues = append(issues, restIssues...)

		// Detect gRPC API drift (if gRPC server is available)
		grpcIssues, err := d.detectGRPCDrift(scanResult.GRPCSpecs, envConfig)
		if err != nil {
			// gRPC drift detection is optional, don't fail if not available
			fmt.Printf("⚠️  gRPC drift detection not available: %v\n", err)
		} else {
			issues = append(issues, grpcIssues...)
		}
	}

	// Detect internal drift between specs
	internalIssues := d.detectInternalDrift(scanResult)
	issues = append(issues, internalIssues...)

	return issues, nil
}

func (d *DriftDetector) detectRESTDrift(specs []spec_loader.Spec, envConfig EnvironmentConfig) ([]DriftIssue, error) {
	var issues []DriftIssue

	for _, spec := range specs {
		if spec.SpecType != spec_loader.SpecTypeOpenAPI {
			continue
		}

		fmt.Printf("🔍 Checking REST API drift for spec: %s\n", spec.Metadata["file_path"])

		// Extract endpoints from spec
		if paths, ok := spec.Content["paths"].(map[string]interface{}); ok {
			for path, pathInfo := range paths {
				if pathInfoMap, ok := pathInfo.(map[string]interface{}); ok {
					for method := range pathInfoMap {
						if d.isHTTPMethod(method) {
							// Test endpoint against live API
							issue := d.testEndpoint(path, method, envConfig)
							if issue != nil {
								issues = append(issues, *issue)
							}
						}
					}
				}
			}
		}
	}

	return issues, nil
}

func (d *DriftDetector) detectGRPCDrift(specs []spec_loader.Spec, envConfig EnvironmentConfig) ([]DriftIssue, error) {
	var issues []DriftIssue

	for _, spec := range specs {
		if spec.SpecType != spec_loader.SpecTypeProto {
			continue
		}

		fmt.Printf("🔍 Checking gRPC API drift for spec: %s\n", spec.Metadata["file_path"])

		// gRPC drift detection would require gRPC client setup
		// For now, we'll do basic validation
		issues = append(issues, d.validateGRPCSpec(spec)...)
	}

	return issues, nil
}

func (d *DriftDetector) detectInternalDrift(scanResult *scanner.ScanResult) []DriftIssue {
	var issues []DriftIssue

	// Check for inconsistencies between REST and gRPC specs
	restEndpoints := d.extractRESTEndpoints(scanResult.RESTSpecs)
	grpcServices := d.extractGRPCServices(scanResult.GRPCSpecs)

	// Look for missing gRPC equivalents
	for restPath := range restEndpoints {
		grpcEquivalent := d.pathToGRPCService(restPath)
		if _, exists := grpcServices[grpcEquivalent]; !exists {
			issues = append(issues, DriftIssue{
				Type:        "grpc_missing",
				Severity:    "medium",
				Description: fmt.Sprintf("REST endpoint %s has no gRPC equivalent %s", restPath, grpcEquivalent),
				Path:        restPath,
			})
		}
	}

	// Check protobuf consistency
	protoIssues := d.validateProtobufConsistency(scanResult.Protobufs)
	issues = append(issues, protoIssues...)

	return issues
}

func (d *DriftDetector) testEndpoint(path, method string, envConfig EnvironmentConfig) *DriftIssue {
	url := envConfig.BaseURL + path

	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return &DriftIssue{
			Type:        "request_error",
			Severity:    "high",
			Description: fmt.Sprintf("Failed to create request for %s %s: %v", method, path, err),
			Path:        path,
		}
	}

	// Add headers
	for key, value := range envConfig.Headers {
		req.Header.Set(key, value)
	}

	// Add authentication
	d.addAuth(req, envConfig.Auth)

	resp, err := d.client.Do(req)
	if err != nil {
		return &DriftIssue{
			Type:        "connection_error",
			Severity:    "high",
			Description: fmt.Sprintf("Failed to connect to %s %s: %v", method, path, err),
			Path:        path,
		}
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode >= 400 {
		severity := "medium"
		if resp.StatusCode >= 500 {
			severity = "high"
		}

		return &DriftIssue{
			Type:        "endpoint_error",
			Severity:    severity,
			Description: fmt.Sprintf("Endpoint %s %s returned status %d", method, path, resp.StatusCode),
			Path:        path,
			Expected:    "2xx status code",
			Actual:      fmt.Sprintf("%d status code", resp.StatusCode),
		}
	}

	return nil
}

func (d *DriftDetector) validateGRPCSpec(spec spec_loader.Spec) []DriftIssue {
	var issues []DriftIssue

	// Basic validation for gRPC specs
	if spec.Content == nil {
		issues = append(issues, DriftIssue{
			Type:        "empty_spec",
			Severity:    "high",
			Description: "gRPC spec is empty",
			Path:        fmt.Sprintf("%v", spec.Metadata["file_path"]),
		})
	}

	return issues
}

func (d *DriftDetector) validateProtobufConsistency(protobufs []scanner.ProtobufFile) []DriftIssue {
	var issues []DriftIssue

	// Check for duplicate service names
	serviceNames := make(map[string]string)
	for _, proto := range protobufs {
		for _, service := range proto.Services {
			if existingPath, exists := serviceNames[service.Name]; exists {
				issues = append(issues, DriftIssue{
					Type:        "duplicate_service",
					Severity:    "medium",
					Description: fmt.Sprintf("Service %s defined in multiple files: %s and %s", service.Name, existingPath, proto.Path),
					Path:        proto.Path,
				})
			} else {
				serviceNames[service.Name] = proto.Path
			}
		}
	}

	return issues
}

func (d *DriftDetector) addAuth(req *http.Request, auth AuthConfig) {
	switch auth.Type {
	case "bearer":
		if auth.Token != "" {
			req.Header.Set("Authorization", "Bearer "+auth.Token)
		}
	case "basic":
		if auth.Username != "" && auth.Password != "" {
			req.SetBasicAuth(auth.Username, auth.Password)
		}
	case "apikey":
		if auth.APIKey != "" {
			req.Header.Set("X-API-Key", auth.APIKey)
		}
	}
}

func (d *DriftDetector) getEnvironmentConfig(envName string) EnvironmentConfig {
	// In a real implementation, this would load from config file or database
	// For now, return basic configs
	switch envName {
	case "production":
		return EnvironmentConfig{
			BaseURL: "https://api.production.com",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Auth: AuthConfig{
				Type:  "bearer",
				Token: "prod-token",
			},
		}
	case "staging":
		return EnvironmentConfig{
			BaseURL: "https://api.staging.com",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
			Auth: AuthConfig{
				Type:  "bearer",
				Token: "staging-token",
			},
		}
	default:
		return EnvironmentConfig{
			BaseURL: "http://localhost:8080",
			Headers: map[string]string{
				"Content-Type": "application/json",
			},
		}
	}
}

func (d *DriftDetector) extractRESTEndpoints(specs []spec_loader.Spec) map[string][]string {
	endpoints := make(map[string][]string)

	for _, spec := range specs {
		if paths, ok := spec.Content["paths"].(map[string]interface{}); ok {
			for path := range paths {
				endpoints[path] = []string{}
				if pathInfo, ok := paths[path].(map[string]interface{}); ok {
					for method := range pathInfo {
						if d.isHTTPMethod(method) {
							endpoints[path] = append(endpoints[path], method)
						}
					}
				}
			}
		}
	}

	return endpoints
}

func (d *DriftDetector) extractGRPCServices(specs []spec_loader.Spec) map[string]bool {
	services := make(map[string]bool)

	for _, spec := range specs {
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

func (d *DriftDetector) pathToGRPCService(path string) string {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return "Service"
	}

	lastPart := parts[len(parts)-1]
	serviceName := strings.Title(lastPart) + "Service"

	return serviceName
}

func (d *DriftDetector) isHTTPMethod(method string) bool {
	methods := []string{"get", "post", "put", "delete", "patch", "head", "options"}
	for _, m := range methods {
		if strings.ToLower(method) == m {
			return true
		}
	}
	return false
}
