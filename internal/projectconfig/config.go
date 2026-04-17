package projectconfig

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const (
	workspaceDirName  = ".specguard"
	configFileName    = "config.yaml"
	defaultOutputPath = ".specguard/out"
)

// Config represents the user-facing SpecGuard project configuration stored in .specguard/config.yaml.
type Config struct {
	Version   int            `yaml:"version"`
	Project   Project        `yaml:"project"`
	Inputs    Inputs         `yaml:"inputs"`
	Outputs   Outputs        `yaml:"outputs"`
	SDKs      SDKs           `yaml:"sdks"`
	Docs      Docs           `yaml:"docs"`
	Policies  Policies       `yaml:"policies"`
	Standards StandardsRules `yaml:"standards"`
	LLM       LLMConfig      `yaml:"llm"`
}

// LLMConfig holds settings for the LLM provider used for summaries and embeddings.
type LLMConfig struct {
	Provider        string `yaml:"provider"` // "ollama", "vllm", or "none"
	BaseURL         string `yaml:"base_url"`
	GenerationModel string `yaml:"generation_model"`
	EmbeddingModel  string `yaml:"embedding_model"`
	APIKey          string `yaml:"api_key"`
	TLSSkipVerify   bool   `yaml:"tls_skip_verify"`
}

type Project struct {
	Name string `yaml:"name"`
}

type Inputs struct {
	OpenAPI  []OpenAPIInput  `yaml:"openapi"`
	Protobuf []ProtobufInput `yaml:"protobuf"`
}

type OpenAPIInput struct {
	Path    string `yaml:"path"`
	BaseRef string `yaml:"base_ref"`
}

type ProtobufInput struct {
	Root    string   `yaml:"root"`
	Include []string `yaml:"include"`
	BaseRef string   `yaml:"base_ref"`
}

type Outputs struct {
	Dir string `yaml:"dir"`
}

type SDKs struct {
	Languages []string `yaml:"languages"`
	Mode      string   `yaml:"mode"`
}

type Docs struct {
	Site      bool   `yaml:"site"`
	Markdown  bool   `yaml:"markdown"`
	SourceDir string `yaml:"source_dir"`
}

type Policies struct {
	FailOnBreaking  bool `yaml:"fail_on_breaking"`
	WarnOnPotential bool `yaml:"warn_on_potential"`
}

type StandardsRules struct {
	Ruleset string `yaml:"ruleset"`
}

// Default returns a starter configuration tailored to the provided repository name.
func Default(projectName string) Config {
	return Config{
		Version: 1,
		Project: Project{Name: projectName},
		Inputs: Inputs{
			OpenAPI: []OpenAPIInput{
				{Path: "api/openapi.yaml", BaseRef: "origin/main"},
			},
			Protobuf: []ProtobufInput{
				{Root: "proto", Include: []string{"**/*.proto"}, BaseRef: "origin/main"},
			},
		},
		Outputs: Outputs{Dir: defaultOutputPath},
		SDKs: SDKs{
			Languages: []string{"typescript", "go"},
			Mode:      "artifact",
		},
		Docs: Docs{Site: true, Markdown: true, SourceDir: "docs"},
		Policies: Policies{
			FailOnBreaking:  true,
			WarnOnPotential: true,
		},
		Standards: StandardsRules{Ruleset: "specguard-default"},
		LLM: LLMConfig{
			Provider:        "none",
			BaseURL:         "http://localhost:11434",
			GenerationModel: "llama3.1",
			EmbeddingModel:  "nomic-embed-text",
		},
	}
}

// Load reads the configuration file from disk.
func Load(path string) (Config, error) {
	var cfg Config
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Write persists the configuration to the given path, creating parent directories when necessary.
func Write(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// ConfigPath returns the canonical location of the SpecGuard configuration file for a repository.
func ConfigPath(repoPath string) string {
	return filepath.Join(repoPath, workspaceDirName, configFileName)
}

// WorkspaceDir returns the expected SpecGuard workspace directory inside the repository.
func WorkspaceDir(repoPath string) string {
	return filepath.Join(repoPath, workspaceDirName)
}

// ResolveOutputDir resolves the output directory to an absolute path.
func (c Config) ResolveOutputDir(repoPath string) string {
	out := c.Outputs.Dir
	if out == "" {
		out = defaultOutputPath
	}
	if filepath.IsAbs(out) {
		return out
	}
	return filepath.Join(repoPath, out)
}

// EnsureWritable validates that the config location can be written to, returning an error if it already exists and overwrite is false.
func EnsureWritable(path string, overwrite bool) error {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("unable to check config: %w", err)
	}
	if overwrite {
		return nil
	}
	return fmt.Errorf("config already exists at %s", path)
}
