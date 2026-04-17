package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/specguard/specguard/internal/diff"
	"github.com/specguard/specguard/internal/docconsistency"
	"github.com/specguard/specguard/internal/enrich"
	"github.com/specguard/specguard/internal/llm"
	"github.com/specguard/specguard/internal/projectconfig"
	"github.com/specguard/specguard/internal/protocol"
	"github.com/specguard/specguard/internal/report"
	"github.com/specguard/specguard/internal/risk"
	"github.com/specguard/specguard/internal/scan"
	"github.com/specguard/specguard/internal/standards"
	"github.com/spf13/cobra"
)

func main() {
	rootCmd := newRootCmd()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "specguard",
		Short: "SpecGuard keeps OpenAPI + Proto specs, SDKs, and docs in lockstep",
		Long: `SpecGuard is a production-grade CLI that normalizes specs, computes diffs,
generates reports, and orchestrates docs + SDK artifacts for CI-first workflows.`,
	}

	cmd.PersistentFlags().String("repo", ".", "Path to the API repository")
	cmd.SilenceUsage = true

	cmd.AddCommand(
		newInitCmd(),
		newScanCmd(),
		newDiffCmd(),
		newReportCmd(),
		newSDKCmd(),
		newDocsCmd(),
		newCICmd(),
		newUploadCmd(),
		newServeCmd(),
	)

	return cmd
}

func newInitCmd() *cobra.Command {
	var projectName string
	var force bool

	cmd := &cobra.Command{
		Use:   "init",
		Short: "Create .specguard/config.yaml for the current repository",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, err := resolveRepoPath(cmd)
			if err != nil {
				return err
			}

			if projectName == "" {
				projectName = filepath.Base(repoPath)
			}

			cfg := projectconfig.Default(projectName)
			cfgPath := projectconfig.ConfigPath(repoPath)

			if err := projectconfig.EnsureWritable(cfgPath, force); err != nil {
				return err
			}

			if err := projectconfig.Write(cfgPath, cfg); err != nil {
				return err
			}

			if err := ensureWorkspace(repoPath, cfg); err != nil {
				return err
			}

			relPath, _ := filepath.Rel(repoPath, cfgPath)
			fmt.Fprintf(cmd.OutOrStdout(), "✅ SpecGuard config written to %s\n", relPath)
			return nil
		},
	}

	cmd.Flags().StringVar(&projectName, "project-name", "", "Project name to store in config")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite existing config if present")
	return cmd
}

func newScanCmd() *cobra.Command {
	var outputOverride string

	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Normalize OpenAPI + Proto inputs into deterministic snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, err := resolveRepoPath(cmd)
			if err != nil {
				return err
			}

			cfg, cfgPath, err := loadProjectConfig(repoPath)
			if err != nil {
				return fmt.Errorf("load config: %w (did you run 'specguard init'?)", err)
			}

			if len(cfg.Inputs.OpenAPI) == 0 && len(cfg.Inputs.Protobuf) == 0 {
				return fmt.Errorf("config.inputs requires at least one openapi or protobuf entry")
			}

			outDir := cfg.ResolveOutputDir(repoPath)
			if outputOverride != "" {
				if filepath.IsAbs(outputOverride) {
					outDir = outputOverride
				} else {
					outDir = filepath.Join(repoPath, outputOverride)
				}
			}

			runner := scan.NewRunner()
			if err := runner.Run(cmd.Context(), repoPath, cfg, outDir); err != nil {
				return fmt.Errorf("scan failed: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✅ scan complete (config: %s)\n", relToRepo(repoPath, cfgPath))
			fmt.Fprintf(cmd.OutOrStdout(), "📦 snapshot written to %s\n", relToRepo(repoPath, filepath.Join(outDir, "snapshot")))
			return nil
		},
	}

	cmd.Flags().StringVar(&outputOverride, "out", "", "Override output directory (defaults to config.outputs.dir)")
	return cmd
}

func newDiffCmd() *cobra.Command {
	var baseSnapshot string
	var headSnapshot string
	var outputOverride string

	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compute canonical change sets between snapshots",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, err := resolveRepoPath(cmd)
			if err != nil {
				return err
			}

			cfg, cfgPath, err := loadProjectConfig(repoPath)
			if err != nil {
				return fmt.Errorf("load config: %w (did you run 'specguard init'?)", err)
			}

			if baseSnapshot == "" || headSnapshot == "" {
				return fmt.Errorf("specify --base and --head snapshot paths")
			}

			outDir := cfg.ResolveOutputDir(repoPath)
			if outputOverride != "" {
				if filepath.IsAbs(outputOverride) {
					outDir = outputOverride
				} else {
					outDir = filepath.Join(repoPath, outputOverride)
				}
			}

			basePath := baseSnapshot
			headPath := headSnapshot
			if !filepath.IsAbs(basePath) {
				basePath = filepath.Join(repoPath, basePath)
			}
			if !filepath.IsAbs(headPath) {
				headPath = filepath.Join(repoPath, headPath)
			}

			baseSnap, err := diff.LoadSnapshot(basePath)
			if err != nil {
				return fmt.Errorf("load base snapshot: %w", err)
			}
			headSnap, err := diff.LoadSnapshot(headPath)
			if err != nil {
				return fmt.Errorf("load head snapshot: %w", err)
			}

			result := diff.DeepCompare(baseSnap, headSnap, repoPath)
			diffDir := filepath.Join(outDir, "diff")
			if err := os.MkdirAll(diffDir, 0o755); err != nil {
				return fmt.Errorf("create diff dir: %w", err)
			}

			if err := result.Write(filepath.Join(diffDir, "changes.json"), filepath.Join(diffDir, "summary.json")); err != nil {
				return fmt.Errorf("write diff artifacts: %w", err)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "✅ diff complete (config: %s)\n", relToRepo(repoPath, cfgPath))
			fmt.Fprintf(cmd.OutOrStdout(), "📄 changes: %s\n", relToRepo(repoPath, filepath.Join(diffDir, "changes.json")))
			fmt.Fprintf(cmd.OutOrStdout(), "📊 summary: %s\n", relToRepo(repoPath, filepath.Join(diffDir, "summary.json")))
			return nil
		},
	}

	cmd.Flags().StringVar(&baseSnapshot, "base", "", "Path to base spec_snapshot.json")
	cmd.Flags().StringVar(&headSnapshot, "head", "", "Path to head spec_snapshot.json")
	cmd.Flags().StringVar(&outputOverride, "out", "", "Output directory for diff artifacts (defaults to config outputs dir)")
	return cmd
}

func newReportCmd() *cobra.Command {
	var manifestPath string
	var outPath string
	var diffSummaryPath string
	var diffChangesPath string
	var knowledgePath string
	var reportsDir string
	var specPath string
	var chunksPath string
	var generatePDF bool

	cmd := &cobra.Command{
		Use:   "report",
		Short: "Render scan summaries, standards, doc consistency, drift, and risk reports",
		RunE: func(cmd *cobra.Command, args []string) error {
			repoPath, err := resolveRepoPath(cmd)
			if err != nil {
				return err
			}

			cfg, _, err := loadProjectConfig(repoPath)
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			manifest := manifestPath
			if manifest == "" {
				manifest = filepath.Join(cfg.ResolveOutputDir(repoPath), "snapshot", "manifest.json")
			}
			if !filepath.IsAbs(manifest) {
				manifest = filepath.Join(repoPath, manifest)
			}

			reportPath := outPath
			if reportPath == "" {
				reportPath = filepath.Join(filepath.Dir(manifest), "report.summary.json")
			}
			if !filepath.IsAbs(reportPath) {
				reportPath = filepath.Join(repoPath, reportPath)
			}

			if err := report.Generate(manifest, reportPath); err != nil {
				return fmt.Errorf("generate report: %w", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "📄 report written to %s\n", relToRepo(repoPath, reportPath))

			reportsOut := reportsDir
			if reportsOut == "" {
				reportsOut = filepath.Join(cfg.ResolveOutputDir(repoPath), "reports")
			} else if !filepath.IsAbs(reportsOut) {
				reportsOut = filepath.Join(repoPath, reportsOut)
			}

			standardsViolations := 0
			var riskFindings []risk.RiskFinding

			// Initialize LLM provider for summaries
			llmCfg := llm.Config{
				Provider:        cfg.LLM.Provider,
				BaseURL:         cfg.LLM.BaseURL,
				GenerationModel: cfg.LLM.GenerationModel,
				EmbeddingModel:  cfg.LLM.EmbeddingModel,
				APIKey:          cfg.LLM.APIKey,
				TLSSkipVerify:   cfg.LLM.TLSSkipVerify,
			}
			llmProvider, _ := llm.NewProvider(llmCfg)
			summarizer := llm.NewSummarizer(llmProvider)
			var sections llm.SectionSummaries

			// Standards analysis
			if specPath != "" {
				if !filepath.IsAbs(specPath) {
					specPath = filepath.Join(repoPath, specPath)
				}
				stdReport, err := standards.Analyze(specPath)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "⚠️ standards analysis failed: %v\n", err)
				} else {
					if err := standards.WriteReport(stdReport, reportsOut); err != nil {
						return fmt.Errorf("write standards report: %w", err)
					}
					standardsViolations = stdReport.TotalViolation
					for _, v := range stdReport.Violations {
						riskFindings = append(riskFindings, risk.RiskFinding{
							Category:    "Standards Violation (" + v.RuleID + ")",
							Severity:    v.Severity,
							Path:        v.Path,
							Description: v.Description,
							Remediation: v.Remediation,
						})
					}
					fmt.Fprintf(cmd.OutOrStdout(), "📏 standards report: %d violations (%s)\n",
						stdReport.TotalViolation, relToRepo(repoPath, filepath.Join(reportsOut, "standards.md")))
					if summarizer != nil {
						var vDescs []string
						for _, v := range stdReport.Violations {
							vDescs = append(vDescs, fmt.Sprintf("- [%s] %s: %s", v.Severity, v.RuleID, v.Description))
						}
						sections.Standards = summarizer.SummarizeStandards(stdReport.TotalChecked, stdReport.TotalViolation, vDescs)
					}
				}

				// Doc consistency (requires both spec and chunks)
				if chunksPath != "" {
					if !filepath.IsAbs(chunksPath) {
						chunksPath = filepath.Join(repoPath, chunksPath)
					}
					docReport, err := docconsistency.Analyze(specPath, chunksPath)
					if err != nil {
						fmt.Fprintf(cmd.ErrOrStderr(), "⚠️ doc consistency analysis failed: %v\n", err)
					} else {
						if err := docconsistency.WriteReport(docReport, reportsOut); err != nil {
							return fmt.Errorf("write doc consistency report: %w", err)
						}
						for _, issue := range docReport.Issues {
							riskFindings = append(riskFindings, risk.RiskFinding{
								Category:    "Doc Consistency",
								Severity:    issue.Severity,
								Path:        issue.Path,
								Description: issue.Description,
								Remediation: issue.Remediation,
							})
						}
						fmt.Fprintf(cmd.OutOrStdout(), "📋 doc consistency: %d issues (%s)\n",
							docReport.TotalIssues, relToRepo(repoPath, filepath.Join(reportsOut, "doc_consistency.md")))
						if summarizer != nil {
							var undoc []string
							for _, issue := range docReport.Issues {
								undoc = append(undoc, issue.Path)
							}
							sections.DocConsistency = summarizer.SummarizeDocConsistency(docReport.TotalIssues, undoc)
						}
					}
				}
			}

			// Enriched swagger generation (requires both spec and chunks)
			if specPath != "" && chunksPath != "" {
				enrichResult, err := enrich.EnrichSpec(specPath, chunksPath, reportsOut)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "⚠️ swagger enrichment failed: %v\n", err)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "📖 enrichment: %d/%d endpoints matched to docs (%.0f%% coverage) → %s\n",
						enrichResult.EnrichedEndpoints, enrichResult.TotalEndpoints,
						float64(enrichResult.EnrichedEndpoints)/float64(max(enrichResult.TotalEndpoints, 1))*100,
						relToRepo(repoPath, filepath.Join(reportsOut, "enrichment_summary.md")))
					if summarizer != nil {
						sections.Enrichment = summarizer.SummarizeEnrichment(enrichResult.TotalEndpoints, enrichResult.EnrichedEndpoints, enrichResult.ChunksMatched)
					}
				}
			}

			// SOAF per-endpoint protocol recommendation (only needs spec)
			if specPath != "" {
				hasProto := false
				protoNorm := filepath.Join(cfg.ResolveOutputDir(repoPath), "snapshot", "proto.normalized.json")
				if _, err := os.Stat(protoNorm); err == nil {
					hasProto = true
				}
				protoReport, err := protocol.AnalyzeSpec(specPath, hasProto)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "⚠️ protocol analysis failed: %v\n", err)
				} else {
					if err := protocol.WriteReport(protoReport, reportsOut); err != nil {
						return fmt.Errorf("write protocol report: %w", err)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "⚡ protocol recommendation: %d REST, %d gRPC, %d either (%s)\n",
						protoReport.RESTPreferred, protoReport.GRPCPreferred, protoReport.Either,
						relToRepo(repoPath, filepath.Join(reportsOut, "protocol_recommendation.md")))
					if summarizer != nil {
						sections.Protocol = summarizer.SummarizeProtocol(protoReport.RESTPreferred, protoReport.GRPCPreferred, protoReport.Either)
					}
				}
			}

			// Drift report from diff changes
			if diffChangesPath != "" {
				changesFile := diffChangesPath
				if !filepath.IsAbs(changesFile) {
					changesFile = filepath.Join(repoPath, changesFile)
				}
				summaryFile := diffSummaryPath
				if summaryFile != "" && !filepath.IsAbs(summaryFile) {
					summaryFile = filepath.Join(repoPath, summaryFile)
				}
				diffResult, err := loadDiffResult(changesFile, summaryFile)
				if err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "⚠️ drift report failed: %v\n", err)
				} else {
					if err := diff.WriteDriftReport(diffResult, reportsOut); err != nil {
						return fmt.Errorf("write drift report: %w", err)
					}
					fmt.Fprintf(cmd.OutOrStdout(), "🔍 drift report: %s\n", relToRepo(repoPath, filepath.Join(reportsOut, "drift.md")))
				}
			}

			// Risk scoring
			if diffSummaryPath != "" && knowledgePath != "" {
				dsPath := diffSummaryPath
				if !filepath.IsAbs(dsPath) {
					dsPath = filepath.Join(repoPath, dsPath)
				}
				kPath := knowledgePath
				if !filepath.IsAbs(kPath) {
					kPath = filepath.Join(repoPath, kPath)
				}
				if err := risk.GenerateWithStandards(dsPath, kPath, reportsOut, standardsViolations); err != nil {
					return fmt.Errorf("generate risk reports: %w", err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "⚖️ risk reports written under %s\n", relToRepo(repoPath, reportsOut))
			} else if standardsViolations > 0 || len(riskFindings) > 0 {
				// Generate risk report from standards violations alone (no diff needed)
				if err := risk.GenerateStandalone(reportsOut, standardsViolations, riskFindings); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "⚠️ standalone risk report failed: %v\n", err)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "⚖️ risk report (standards-only): %s\n", relToRepo(repoPath, filepath.Join(reportsOut, "risk.md")))
				}
			} else if diffSummaryPath != "" || knowledgePath != "" {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠️ both --diff-summary and --knowledge are required to emit full risk reports\n")
			}

			// LLM risk summary
			if summarizer != nil && (standardsViolations > 0 || len(riskFindings) > 0) {
				var findingDescs []string
				for _, f := range riskFindings {
					findingDescs = append(findingDescs, fmt.Sprintf("- [%s] %s: %s", f.Severity, f.Category, f.Description))
				}
				score := standardsViolations * 3
				grade := "INFO"
				if score >= 85 {
					grade = "CRITICAL"
				} else if score >= 65 {
					grade = "HIGH"
				} else if score >= 40 {
					grade = "MEDIUM"
				} else if score >= 20 {
					grade = "LOW"
				}
				sections.Risk = summarizer.SummarizeRisk(score, grade, findingDescs)
			}

			// Generate executive summary and PDF
			if summarizer != nil {
				sections.Executive = summarizer.GenerateExecutiveSummary(sections)
				if sections.Executive != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "🤖 LLM summaries generated via %s\n", llmProvider.Name())
				}
			}

			if generatePDF {
				pdfPath := filepath.Join(reportsOut, "specguard_report.pdf")
				if err := generatePDFReport(pdfPath, cfg.Project.Name, reportsOut, sections); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "⚠️ PDF generation failed: %v\n", err)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "📄 PDF report: %s\n", relToRepo(repoPath, pdfPath))
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVar(&manifestPath, "manifest", "", "Path to manifest.json (defaults to config outputs)")
	cmd.Flags().StringVar(&outPath, "out", "", "Path to write the summary JSON (defaults to sibling of manifest)")
	cmd.Flags().StringVar(&diffSummaryPath, "diff-summary", "", "Path to diff/summary.json for risk scoring")
	cmd.Flags().StringVar(&diffChangesPath, "diff-changes", "", "Path to diff/changes.json for drift report")
	cmd.Flags().StringVar(&knowledgePath, "knowledge", "", "Path to knowledge_model.json")
	cmd.Flags().StringVar(&reportsDir, "reports-dir", "", "Directory to write reports (defaults to config outputs dir / reports)")
	cmd.Flags().StringVar(&specPath, "spec", "", "Path to normalized OpenAPI JSON for standards analysis")
	cmd.Flags().StringVar(&chunksPath, "chunks", "", "Path to doc_index/chunks.jsonl for doc consistency")
	cmd.Flags().BoolVar(&generatePDF, "pdf", false, "Generate a unified PDF summary report")
	return cmd
}

func newSDKCmd() *cobra.Command {
	sdkCmd := &cobra.Command{
		Use:   "sdk",
		Short: "SDK generation + packaging commands",
	}

	sdkCmd.AddCommand(&cobra.Command{
		Use:   "generate",
		Short: "Generate language-specific SDK assets",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("specguard sdk generate")
		},
	})

	sdkCmd.AddCommand(&cobra.Command{
		Use:   "package",
		Short: "Package SDK outputs into deterministic zips",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("specguard sdk package")
		},
	})

	return sdkCmd
}

func newDocsCmd() *cobra.Command {
	docsCmd := &cobra.Command{
		Use:   "docs",
		Short: "Documentation generation pipeline",
	}

	docsCmd.AddCommand(&cobra.Command{
		Use:   "generate",
		Short: "Generate markdown + static docs site",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("specguard docs generate")
		},
	})

	return docsCmd
}

func newCICmd() *cobra.Command {
	ciCmd := &cobra.Command{
		Use:   "ci",
		Short: "CI-specific helpers",
	}

	ciCmd.AddCommand(&cobra.Command{
		Use:   "github",
		Short: "Emit GitHub-friendly PR comment markdown + exit codes",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("specguard ci github")
		},
	})

	return ciCmd
}

func newUploadCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "upload",
		Short: "Upload artifacts + run metadata to the SpecGuard control plane",
		RunE: func(cmd *cobra.Command, args []string) error {
			return notImplemented("specguard upload")
		},
	}

	cmd.Flags().String("org", "", "Organization identifier")
	cmd.Flags().String("repo-id", "", "Repository identifier")
	cmd.Flags().String("run", "", "Run identifier")
	cmd.Flags().String("dir", ".specguard/out", "Output directory to upload")
	cmd.Flags().String("token", "", "API token for upload")
	return cmd
}

func loadDiffResult(changesPath, summaryPath string) (diff.Result, error) {
	var result diff.Result

	changesRaw, err := os.ReadFile(changesPath)
	if err != nil {
		return result, fmt.Errorf("read changes: %w", err)
	}
	if err := json.Unmarshal(changesRaw, &result.Changes); err != nil {
		return result, fmt.Errorf("parse changes: %w", err)
	}

	if summaryPath != "" {
		summaryRaw, err := os.ReadFile(summaryPath)
		if err != nil {
			return result, fmt.Errorf("read summary: %w", err)
		}
		if err := json.Unmarshal(summaryRaw, &result.Summary); err != nil {
			return result, fmt.Errorf("parse summary: %w", err)
		}
	}

	return result, nil
}

func resolveRepoPath(cmd *cobra.Command) (string, error) {
	repoPath, err := filepath.Abs(cmd.Flags().Lookup("repo").Value.String())
	if err != nil {
		return "", fmt.Errorf("resolve repo path: %w", err)
	}
	return repoPath, nil
}

func loadProjectConfig(repoPath string) (projectconfig.Config, string, error) {
	cfgPath := projectconfig.ConfigPath(repoPath)
	cfg, err := projectconfig.Load(cfgPath)
	return cfg, cfgPath, err
}

func ensureWorkspace(repoPath string, cfg projectconfig.Config) error {
	workspace := projectconfig.WorkspaceDir(repoPath)
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		return err
	}
	return os.MkdirAll(cfg.ResolveOutputDir(repoPath), 0o755)
}

func relToRepo(repoPath, path string) string {
	if rel, err := filepath.Rel(repoPath, path); err == nil {
		return rel
	}
	return path
}

func notImplemented(name string) error {
	return fmt.Errorf("%s is not implemented yet. Follow the roadmap in README.md for milestone progress.", name)
}
