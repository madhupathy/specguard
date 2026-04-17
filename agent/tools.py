"""SpecGuard CLI tool wrappers for LangChain agent integration."""

import json
import subprocess
from pathlib import Path
from typing import Optional

from langchain_core.tools import tool


def _run_specguard(args: list[str], repo_path: str = ".") -> dict:
    """Execute a specguard CLI command and return parsed output."""
    cmd = ["specguard", "--repo", repo_path] + args
    try:
        result = subprocess.run(
            cmd, capture_output=True, text=True, timeout=120
        )
        return {
            "success": result.returncode == 0,
            "stdout": result.stdout,
            "stderr": result.stderr,
            "returncode": result.returncode,
        }
    except FileNotFoundError:
        return {
            "success": False,
            "stdout": "",
            "stderr": "specguard binary not found. Ensure it is built and on PATH.",
            "returncode": -1,
        }
    except subprocess.TimeoutExpired:
        return {
            "success": False,
            "stdout": "",
            "stderr": "Command timed out after 120 seconds.",
            "returncode": -1,
        }


@tool
def specguard_init(repo_path: str = ".") -> str:
    """Initialize a SpecGuard project in the given repository path.
    Creates .specguard/config.yaml with default settings."""
    result = _run_specguard(["init"], repo_path)
    if result["success"]:
        return f"Project initialized successfully.\n{result['stdout']}"
    return f"Init failed: {result['stderr']}"


@tool
def specguard_scan(repo_path: str = ".", output_dir: Optional[str] = None) -> str:
    """Scan a repository to normalize OpenAPI and Protobuf specs into snapshots.
    Produces spec_snapshot.json, knowledge_model.json, and doc_index."""
    args = ["scan"]
    if output_dir:
        args += ["--out", output_dir]
    result = _run_specguard(args, repo_path)
    if result["success"]:
        return f"Scan complete.\n{result['stdout']}"
    return f"Scan failed: {result['stderr']}"


@tool
def specguard_diff(
    repo_path: str = ".",
    base_snapshot: str = "",
    head_snapshot: str = "",
) -> str:
    """Compare two spec snapshots to detect API changes.
    Requires --base and --head paths to spec_snapshot.json files."""
    if not base_snapshot or not head_snapshot:
        return "Error: both base_snapshot and head_snapshot paths are required."
    args = ["diff", "--base", base_snapshot, "--head", head_snapshot]
    result = _run_specguard(args, repo_path)
    if result["success"]:
        return f"Diff complete.\n{result['stdout']}"
    return f"Diff failed: {result['stderr']}"


@tool
def specguard_report(
    repo_path: str = ".",
    spec_path: Optional[str] = None,
    diff_summary: Optional[str] = None,
    diff_changes: Optional[str] = None,
    knowledge: Optional[str] = None,
    chunks: Optional[str] = None,
) -> str:
    """Generate comprehensive reports: summary, standards, doc consistency,
    drift, risk, and protocol recommendations.

    Optional flags:
    - spec_path: OpenAPI JSON for standards analysis
    - diff_summary + knowledge: for risk scoring
    - diff_changes: for drift report
    - chunks: doc_index/chunks.jsonl for doc consistency
    """
    args = ["report"]
    if spec_path:
        args += ["--spec", spec_path]
    if diff_summary:
        args += ["--diff-summary", diff_summary]
    if diff_changes:
        args += ["--diff-changes", diff_changes]
    if knowledge:
        args += ["--knowledge", knowledge]
    if chunks:
        args += ["--chunks", chunks]
    result = _run_specguard(args, repo_path)
    if result["success"]:
        return f"Reports generated.\n{result['stdout']}"
    return f"Report failed: {result['stderr']}"


@tool
def read_report_file(file_path: str) -> str:
    """Read and return the contents of a SpecGuard report file (JSON or Markdown)."""
    path = Path(file_path)
    if not path.exists():
        return f"File not found: {file_path}"
    content = path.read_text()
    if path.suffix == ".json":
        try:
            data = json.loads(content)
            return json.dumps(data, indent=2)
        except json.JSONDecodeError:
            return content
    return content


@tool
def list_report_artifacts(repo_path: str = ".") -> str:
    """List all SpecGuard output artifacts in the .specguard/out directory."""
    out_dir = Path(repo_path) / ".specguard" / "out"
    if not out_dir.exists():
        return f"No output directory found at {out_dir}. Run 'specguard scan' first."

    artifacts = []
    for p in sorted(out_dir.rglob("*")):
        if p.is_file():
            rel = p.relative_to(out_dir)
            size = p.stat().st_size
            artifacts.append(f"  {rel} ({size} bytes)")

    if not artifacts:
        return "Output directory exists but contains no files."
    return "SpecGuard artifacts:\n" + "\n".join(artifacts)


def get_all_tools() -> list:
    """Return all SpecGuard tools for agent registration."""
    return [
        specguard_init,
        specguard_scan,
        specguard_diff,
        specguard_report,
        read_report_file,
        list_report_artifacts,
    ]
