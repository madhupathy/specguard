package report

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Manifest mirrors the scan manifest schema.
type Manifest struct {
	Version     int             `json:"version"`
	GeneratedAt string          `json:"generated_at"`
	Entries     []ManifestEntry `json:"entries"`
}

type ManifestEntry struct {
	Type      string `json:"type"`
	Source    string `json:"source"`
	Output    string `json:"output"`
	SizeBytes int64  `json:"size_bytes"`
	SHA256    string `json:"sha256"`
	Count     int    `json:"count,omitempty"`
}

// Summary captures the derived report information persisted for users.
type Summary struct {
	GeneratedAt     string         `json:"generated_at"`
	ManifestPath    string         `json:"manifest_path"`
	Totals          Totals         `json:"totals"`
	Entries         []SummaryEntry `json:"entries"`
	Warnings        []string       `json:"warnings"`
	Recommendations []string       `json:"recommendations"`
}

type Totals struct {
	Entries  int `json:"entries"`
	OpenAPI  int `json:"openapi"`
	Protobuf int `json:"protobuf"`
	Markdown int `json:"markdown"`
}

type SummaryEntry struct {
	Type      string `json:"type"`
	Source    string `json:"source"`
	Output    string `json:"output"`
	SizeBytes int64  `json:"size_bytes"`
	SizeKB    int64  `json:"size_kb"`
	SHA256    string `json:"sha256"`
	Count     int    `json:"count,omitempty"`
}

// Generate reads the manifest JSON and emits a higher-level summary to outPath.
func Generate(manifestPath, outPath string) error {
	mani, err := loadManifest(manifestPath)
	if err != nil {
		return err
	}

	summary := summarize(mani, manifestPath)

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create report dir: %w", err)
	}

	raw, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}

	if err := os.WriteFile(outPath, raw, 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	return nil
}

func loadManifest(path string) (Manifest, error) {
	var mani Manifest
	raw, err := os.ReadFile(path)
	if err != nil {
		return mani, fmt.Errorf("read manifest: %w", err)
	}
	if err := json.Unmarshal(raw, &mani); err != nil {
		return mani, fmt.Errorf("parse manifest: %w", err)
	}
	return mani, nil
}

func summarize(mani Manifest, manifestPath string) Summary {
	totals := Totals{}
	entries := make([]SummaryEntry, 0, len(mani.Entries))
	warnings := []string{}
	recommendations := []string{}

	for _, entry := range mani.Entries {
		totals.Entries++
		switch entry.Type {
		case "openapi":
			totals.OpenAPI++
		case "protobuf":
			totals.Protobuf++
		case "markdown":
			totals.Markdown++
		}

		entries = append(entries, SummaryEntry{
			Type:      entry.Type,
			Source:    entry.Source,
			Output:    entry.Output,
			SizeBytes: entry.SizeBytes,
			SizeKB:    entry.SizeBytes / 1024,
			SHA256:    entry.SHA256,
			Count:     entry.Count,
		})
	}

	if totals.OpenAPI == 0 {
		warnings = append(warnings, "No OpenAPI inputs were normalized. Provide config.inputs.openapi or ensure docs/*.json exists.")
	}
	if totals.Protobuf == 0 {
		warnings = append(warnings, "No protobuf descriptors were generated. Missing proto roots or imports may need configuration.")
		recommendations = append(recommendations, "Enable protobuf inputs once github/api/validate protos are vendored.")
	}

	generated := mani.GeneratedAt
	if generated == "" {
		generated = time.Now().UTC().Format(time.RFC3339Nano)
	}

	return Summary{
		GeneratedAt:     generated,
		ManifestPath:    manifestPath,
		Totals:          totals,
		Entries:         entries,
		Warnings:        warnings,
		Recommendations: recommendations,
	}
}
