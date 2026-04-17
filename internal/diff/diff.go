package diff

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Snapshot mirrors the spec_snapshot.json schema emitted during scan.
type Snapshot struct {
	GeneratedAt  string          `json:"generated_at"`
	ManifestPath string          `json:"manifest_path"`
	EntryCount   int             `json:"entry_count"`
	Entries      []SnapshotEntry `json:"entries"`
}

// SnapshotEntry represents an individual normalized artifact.
type SnapshotEntry struct {
	Type      string `json:"type"`
	Source    string `json:"source"`
	Output    string `json:"output"`
	SHA256    string `json:"sha256"`
	SizeBytes int64  `json:"size_bytes"`
	Count     int    `json:"count,omitempty"`
}

// Change describes a difference between the snapshots.
type Change struct {
	ID       string            `json:"id"`
	Kind     string            `json:"kind"`
	Severity string            `json:"severity"`
	Source   string            `json:"source"`
	Details  map[string]string `json:"details"`
}

// Summary captures aggregate stats for the diff run.
type Summary struct {
	GeneratedAt       string `json:"generated_at"`
	TotalChanges      int    `json:"total_changes"`
	Breaking          int    `json:"breaking"`
	PotentialBreaking int    `json:"potential_breaking"`
	NonBreaking       int    `json:"non_breaking"`
	DocumentationOnly int    `json:"documentation_only"`
	Additions         int    `json:"additions"`
	Removals          int    `json:"removals"`
	Mutations         int    `json:"mutations"`
}

// Result bundles the full change list and summary.
type Result struct {
	Changes []Change `json:"changes"`
	Summary Summary  `json:"summary"`
}

// LoadSnapshot parses a spec snapshot from disk.
func LoadSnapshot(path string) (Snapshot, error) {
	var snap Snapshot
	raw, err := os.ReadFile(path)
	if err != nil {
		return snap, fmt.Errorf("read snapshot: %w", err)
	}
	if err := json.Unmarshal(raw, &snap); err != nil {
		return snap, fmt.Errorf("parse snapshot: %w", err)
	}
	return snap, nil
}

// Compare performs a shallow diff against the two snapshots.
func Compare(base Snapshot, head Snapshot) Result {
	result := Result{Changes: []Change{}, Summary: Summary{GeneratedAt: timestampOrNow(head.GeneratedAt)}}

	baseEntries := indexEntries(base.Entries)
	headEntries := indexEntries(head.Entries)

	// Detect removals and mutations.
	for key, entry := range baseEntries {
		headEntry, ok := headEntries[key]
		if !ok {
			ch := newChange("entry.removed", entry.Type, entry.Source, entry.Output)
			classifySeverity(&ch, entry.Type, "removed")
			result.Summary.Removals++
			accumulate(&result, ch)
			continue
		}
		if entry.SHA256 != headEntry.SHA256 {
			ch := newChange("entry.changed", entry.Type, entry.Source, headEntry.Output)
			ch.Details["from_sha"] = entry.SHA256
			ch.Details["to_sha"] = headEntry.SHA256
			classifySeverity(&ch, entry.Type, "changed")
			result.Summary.Mutations++
			accumulate(&result, ch)
		}
	}

	// Detect additions.
	for key, entry := range headEntries {
		if _, ok := baseEntries[key]; !ok {
			ch := newChange("entry.added", entry.Type, entry.Source, entry.Output)
			classifySeverity(&ch, entry.Type, "added")
			result.Summary.Additions++
			accumulate(&result, ch)
		}
	}

	return result
}

// Write persists the change list and summary to disk.
func (r Result) Write(changesPath, summaryPath string) error {
	if err := ensureDir(changesPath); err != nil {
		return err
	}
	if err := ensureDir(summaryPath); err != nil {
		return err
	}

	changeBytes, err := json.MarshalIndent(r.Changes, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal changes: %w", err)
	}
	if err := os.WriteFile(changesPath, changeBytes, 0o644); err != nil {
		return fmt.Errorf("write changes: %w", err)
	}

	summaryBytes, err := json.MarshalIndent(r.Summary, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal summary: %w", err)
	}
	if err := os.WriteFile(summaryPath, summaryBytes, 0o644); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}

	return nil
}

func ensureDir(path string) error {
	return os.MkdirAll(filepath.Dir(path), 0o755)
}

func indexEntries(entries []SnapshotEntry) map[string]SnapshotEntry {
	out := make(map[string]SnapshotEntry, len(entries))
	for _, entry := range entries {
		key := entryKey(entry)
		out[key] = entry
	}
	return out
}

func entryKey(entry SnapshotEntry) string {
	return fmt.Sprintf("%s::%s", entry.Type, entry.Source)
}

func newChange(kind, entryType, source, output string) Change {
	return Change{
		ID:       fmt.Sprintf("%s:%s", kind, entryKey(SnapshotEntry{Type: entryType, Source: source})),
		Kind:     kind,
		Severity: "non_breaking",
		Source:   source,
		Details: map[string]string{
			"type":   entryType,
			"output": output,
		},
	}
}

func classifySeverity(ch *Change, entryType, action string) {
	switch entryType {
	case "openapi", "protobuf":
		switch action {
		case "removed":
			ch.Severity = "breaking"
		case "changed":
			ch.Severity = "potential_breaking"
		default:
			ch.Severity = "potential_breaking"
		}
	case "doc_index", "markdown":
		ch.Severity = "documentation_only"
	default:
		ch.Severity = "non_breaking"
	}
}

func accumulate(result *Result, ch Change) {
	result.Changes = append(result.Changes, ch)
	result.Summary.TotalChanges++
	switch ch.Severity {
	case "breaking":
		result.Summary.Breaking++
	case "potential_breaking":
		result.Summary.PotentialBreaking++
	case "documentation_only":
		result.Summary.DocumentationOnly++
	default:
		result.Summary.NonBreaking++
	}
}

func timestampOrNow(ts string) string {
	if ts != "" {
		return ts
	}
	return time.Now().UTC().Format(time.RFC3339Nano)
}
