package scan

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/specguard/specguard/internal/projectconfig"
)

func TestRunnerAutoDiscoversOpenAPI(t *testing.T) {
	repoDir := t.TempDir()
	docsDir := filepath.Join(repoDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	openapiPath := filepath.Join(docsDir, "apidocs.swagger.json")
	if err := os.WriteFile(openapiPath, []byte(`{
        "openapi":"3.0.0",
        "info":{"title":"Test API","version":"1.0.0"},
        "paths":{}
    }`), 0o644); err != nil {
		t.Fatalf("write openapi: %v", err)
	}

	outDir := filepath.Join(repoDir, ".specguard", "out")
	cfg := projectconfig.Config{
		Outputs: projectconfig.Outputs{Dir: outDir},
	}

	r := NewRunner()
	if err := r.Run(context.Background(), repoDir, cfg, outDir); err != nil {
		t.Fatalf("runner run: %v", err)
	}

	snapshotPath := filepath.Join(outDir, "snapshot", "openapi.normalized.json")
	if _, err := os.Stat(snapshotPath); err != nil {
		t.Fatalf("expected normalized openapi, err=%v", err)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(outDir, "snapshot", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var mani manifest
	if err := json.Unmarshal(manifestBytes, &mani); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(mani.Entries) != 1 || mani.Entries[0].Type != "openapi" {
		t.Fatalf("expected single openapi entry, manifest=%+v", mani)
	}
}

func TestRunnerMarkdownFallback(t *testing.T) {
	repoDir := t.TempDir()
	docsDir := filepath.Join(repoDir, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	mdPath := filepath.Join(docsDir, "api-grpc.md")
	if err := os.WriteFile(mdPath, []byte("# gRPC Overview"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}

	outDir := filepath.Join(repoDir, ".specguard", "out")
	cfg := projectconfig.Config{
		Outputs: projectconfig.Outputs{Dir: outDir},
	}

	r := NewRunner()
	if err := r.Run(context.Background(), repoDir, cfg, outDir); err != nil {
		t.Fatalf("runner run: %v", err)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(outDir, "snapshot", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var mani manifest
	if err := json.Unmarshal(manifestBytes, &mani); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(mani.Entries) != 1 || mani.Entries[0].Type != "markdown" {
		t.Fatalf("expected markdown entry, manifest=%+v", mani)
	}
}

func TestRunnerNormalizesProtobuf(t *testing.T) {
	repoDir := t.TempDir()
	protoDir := filepath.Join(repoDir, "proto", "greeting")
	if err := os.MkdirAll(protoDir, 0o755); err != nil {
		t.Fatalf("mkdir proto: %v", err)
	}
	protoPath := filepath.Join(protoDir, "hello.proto")
	protoContents := `syntax = "proto3";
package greeting;

message HelloRequest {
  string name = 1;
}
`
	if err := os.WriteFile(protoPath, []byte(protoContents), 0o644); err != nil {
		t.Fatalf("write proto: %v", err)
	}

	outDir := filepath.Join(repoDir, ".specguard", "out")
	cfg := projectconfig.Config{
		Outputs: projectconfig.Outputs{Dir: outDir},
		Inputs: projectconfig.Inputs{
			Protobuf: []projectconfig.ProtobufInput{
				{Root: "proto"},
			},
		},
	}

	r := NewRunner()
	if err := r.Run(context.Background(), repoDir, cfg, outDir); err != nil {
		t.Fatalf("runner run: %v", err)
	}

	protoSnapshot := filepath.Join(outDir, "snapshot", "proto.normalized.json")
	if _, err := os.Stat(protoSnapshot); err != nil {
		t.Fatalf("expected proto snapshot, err=%v", err)
	}

	manifestBytes, err := os.ReadFile(filepath.Join(outDir, "snapshot", "manifest.json"))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var mani manifest
	if err := json.Unmarshal(manifestBytes, &mani); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(mani.Entries) != 1 || mani.Entries[0].Type != "protobuf" {
		t.Fatalf("expected protobuf entry, manifest=%+v", mani)
	}
	if mani.Entries[0].Count != 1 {
		t.Fatalf("expected single proto file counted, manifest=%+v", mani)
	}
}
