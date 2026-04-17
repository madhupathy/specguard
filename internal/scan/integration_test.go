package scan

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/specguard/specguard/internal/projectconfig"
)

func TestRunnerAgainstSDKRepo(t *testing.T) {
	const sdkRepoPath = "/tmp/test-api-sdk"
	if _, err := os.Stat(sdkRepoPath); err != nil {
		t.Skipf("sdk repo not available: %v", err)
	}

	outDir := filepath.Join(t.TempDir(), "out")
	cfg := projectconfig.Config{
		Outputs: projectconfig.Outputs{Dir: outDir},
	}

	runner := NewRunner()
	if err := runner.Run(context.Background(), sdkRepoPath, cfg, outDir); err != nil {
		t.Fatalf("runner failed for sdk repo: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outDir, "snapshot", "manifest.json")); err != nil {
		t.Fatalf("manifest not produced: %v", err)
	}
}
