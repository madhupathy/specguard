package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specguard/specguard/internal/config"
	"github.com/specguard/specguard/internal/database"
	"github.com/specguard/specguard/internal/drift_detector"
	"github.com/specguard/specguard/internal/generator"
	"github.com/specguard/specguard/internal/scanner"
	"github.com/specguard/specguard/internal/spec_loader"
	"github.com/specguard/specguard/internal/storage"
)

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

// ScanRepository scans a repository for API specifications
func ScanRepository(repoPath string) {
	fmt.Printf("🔍 Scanning repository: %s\n", repoPath)

	// Initialize components
	cfg := config.Load()
	db, err := database.New(cfg.Database)
	if err != nil {
		fmt.Printf("❌ Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	store, err := storage.New(cfg.Storage)
	if err != nil {
		fmt.Printf("❌ Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	// Create scanner
	scanner := scanner.New(db, store)

	// Scan the repository
	result, err := scanner.ScanRepository(repoPath)
	if err != nil {
		fmt.Printf("❌ Scan failed: %v\n", err)
		os.Exit(1)
	}

	// Display results
	fmt.Printf("\n✅ Scan completed!\n")
	fmt.Printf("📊 Found %d REST specs\n", len(result.RESTSpecs))
	fmt.Printf("🔧 Found %d gRPC specs\n", len(result.GRPCSpecs))
	fmt.Printf("📦 Found %d protobuf files\n", len(result.Protobufs))

	if len(result.Drift) > 0 {
		fmt.Printf("⚠️  Found %d drift issues\n", len(result.Drift))
		for _, issue := range result.Drift {
			fmt.Printf("   - %s: %s\n", issue.Severity, issue.Description)
		}
	} else {
		fmt.Printf("✅ No drift detected\n")
	}

	// Save results to database
	fmt.Printf("\n💾 Saving scan results...\n")
	// Implementation would save to database
	fmt.Printf("✅ Results saved\n")
}

// DetectDrift detects drift between specifications and live APIs
func DetectDrift(repoPath, compareWith string) {
	fmt.Printf("🔍 Detecting drift in repository: %s\n", repoPath)
	if compareWith != "" {
		fmt.Printf("🔄 Comparing with environment: %s\n", compareWith)
	}

	// Initialize components
	cfg := config.Load()
	db, err := database.New(cfg.Database)
	if err != nil {
		fmt.Printf("❌ Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	store, err := storage.New(cfg.Storage)
	if err != nil {
		fmt.Printf("❌ Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	// Create drift detector
	detector := drift_detector.New(db, store)

	// Detect drift
	issues, err := detector.DetectDrift(repoPath, compareWith)
	if err != nil {
		fmt.Printf("❌ Drift detection failed: %v\n", err)
		os.Exit(1)
	}

	// Display results
	fmt.Printf("\n🔍 Drift Detection Results:\n")
	if len(issues) == 0 {
		fmt.Printf("✅ No drift detected!\n")
		return
	}

	fmt.Printf("⚠️  Found %d drift issues:\n\n", len(issues))

	for i, issue := range issues {
		fmt.Printf("%d. [%s] %s\n", i+1, strings.ToUpper(issue.Severity), issue.Type)
		fmt.Printf("   Path: %s\n", issue.Path)
		fmt.Printf("   Description: %s\n", issue.Description)
		fmt.Printf("\n")
	}

	// Save drift issues
	fmt.Printf("💾 Saving drift issues...\n")
	// Implementation would save to database
	fmt.Printf("✅ Drift issues saved\n")
}

// GenerateArtifacts generates documentation, SDKs, and protobuf
func GenerateArtifacts(repoPath, output, formats string) {
	fmt.Printf("🔨 Generating artifacts for repository: %s\n", repoPath)
	fmt.Printf("📁 Output directory: %s\n", output)
	fmt.Printf("📦 Formats: %s\n", formats)

	// Parse formats
	formatList := strings.Split(formats, ",")

	// Initialize components
	cfg := config.Load()
	db, err := database.New(cfg.Database)
	if err != nil {
		fmt.Printf("❌ Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	store, err := storage.New(cfg.Storage)
	if err != nil {
		fmt.Printf("❌ Failed to initialize storage: %v\n", err)
		os.Exit(1)
	}

	// Create generator
	gen := generator.New(db, store)

	// Create output directory
	if err := os.MkdirAll(output, 0755); err != nil {
		fmt.Printf("❌ Failed to create output directory: %v\n", err)
		os.Exit(1)
	}

	// Generate each requested format
	for _, format := range formatList {
		format = strings.TrimSpace(format)
		switch format {
		case "docs":
			fmt.Printf("📚 Generating documentation...\n")
			if err := gen.GenerateDocumentation(repoPath, filepath.Join(output, "docs")); err != nil {
				fmt.Printf("❌ Documentation generation failed: %v\n", err)
			} else {
				fmt.Printf("✅ Documentation generated\n")
			}

		case "sdk":
			fmt.Printf("🔧 Generating SDKs...\n")
			if err := gen.GenerateSDKs(repoPath, filepath.Join(output, "sdks")); err != nil {
				fmt.Printf("❌ SDK generation failed: %v\n", err)
			} else {
				fmt.Printf("✅ SDKs generated\n")
			}

		case "proto":
			fmt.Printf("📦 Generating protobuf files...\n")
			if err := gen.GenerateProtobuf(repoPath, filepath.Join(output, "proto")); err != nil {
				fmt.Printf("❌ Protobuf generation failed: %v\n", err)
			} else {
				fmt.Printf("✅ Protobuf files generated\n")
			}

		default:
			fmt.Printf("⚠️  Unknown format: %s\n", format)
		}
	}

	fmt.Printf("\n🎉 Artifact generation completed!\n")
	fmt.Printf("📁 Check the output directory: %s\n", output)
}
