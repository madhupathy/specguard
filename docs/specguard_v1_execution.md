---
description: SpecGuard v1 pipeline and architecture guide
---

# SpecGuard v1 — Step-by-Step Execution Guide

This document captures the final SpecGuard v1 flow requested for the combined CLI/UI experience. Each step is deterministic where truth is required and explicitly calls out how AI (LLMs), MCP connectors, RAG, and the vector database participate. Use this as the implementation source of truth for both CLI development and control-plane orchestration.

## Overview

1. Inputs arrive via CLI flags or UI uploads.
2. Specs + docs are normalized into a unified knowledge model.
3. Deterministic diff + classification emits change metadata.
4. Reports and regenerated docs/SDK/specs are produced.
5. Artifacts are validated, packaged, and surfaced for CI + control plane.

All artifacts live under `.specguard/out/` to match the finalized contract.

---

## Step 1 — Ingest Inputs (CLI or UI Upload)

**Accepted sources**

- **API contracts:** OpenAPI (JSON/YAML), Swagger 2.0, Protocol Buffers (`.proto` roots or descriptor sets), existing JSON Schema fragments.
- **Product documentation:** manuals, guides, FAQs, troubleshooting docs, release notes, API docs (Markdown, HTML, PDF, plaintext).
- **Git context (optional):** `base_ref`, `head_ref`, or stored snapshot metadata when running in PR mode.

**Processing flow**

1. **Spec normalization**
   - Parse OpenAPI/Swagger via kin-openapi, resolve `$ref`, sort operations, and emit canonical `snapshot/openapi.normalized.json`.
   - Parse Proto trees via `buf`/`protoc`, emit descriptor JSON as `snapshot/proto.normalized.json` when `.proto` sources exist.
2. **Documentation ingestion**
   - UI upload or CLI `--docs-dir` pipes files through MCP connectors (e.g., `mcp.filesystem`, `mcp.google-drive`) so assets can be fetched uniformly.
   - Each document is chunked (by section/page) and embedded; embeddings are written to `.specguard/out/doc_index/*.jsonl` and persisted into the vector DB (local SQLite/pgvector in CLI, managed service in SaaS).
3. **Knowledge model synthesis**
   - An agentic workflow orchestrator reads normalized specs + doc chunks, links passages to endpoints/schemas/tags, and writes `.specguard/out/knowledge_model.json`.
   - The knowledge model also stores doc-only entities so later steps can materialize OpenAPI/Proto even when only prose exists.
4. **Snapshot manifest**
   - `.specguard/out/spec_snapshot.json` captures hash, source, git metadata, and doc corpus references for provenance.

**Outputs**

```
.specguard/out/
  knowledge_model.json
  spec_snapshot.json
  doc_index/
    chunks.jsonl
    embeddings.jsonl
```

---

## Step 2 — Detect Spec Changes and Drift

**Comparisons**

- Base vs head OpenAPI (or stored snapshot vs current).
- Base vs head Proto descriptors.
- Spec vs documentation alignment (leveraging knowledge model links).

**Deterministic detections**

- Endpoint/service added or removed.
- Request/response schema edits (field add/remove/type/required changes).
- Enum value changes, auth/security deltas, pagination style shifts.
- Documentation inconsistencies (doc claims behavior missing in spec, spec fields lacking doc coverage).

**Outputs**

```
.specguard/out/diff/
  changes.json     # canonical change list
  summary.json     # counts, scan metadata
  reports/drift.md # includes structured changelog + migration tips
```

`summary.json` is the authoritative “counts + scope of scan” artifact for downstream automation.

---

## Step 3 — Classify Changes and Compute Risk

**Classification**

- Every change in `diff/changes.json` is labeled `breaking`, `potential_breaking`, `non_breaking`, or `documentation_only` via rule tables.
- Governance+lint violations (operationId missing, auth scheme drift, etc.) are attached here so they can influence risk.

**Risk scoring inputs**

1. Breaking / potential-breaking totals (+ weights).
2. Standards violations (from lint engine).
3. Documentation consistency findings (Step 6).
4. Example coverage (from Step 5 regeneration step).
5. Spec completeness (missing descriptions/tags).

**Outputs**

```
.specguard/out/reports/
  risk.json
  risk.md   # narrative w/ mitigation guidance + surfaced changelog entries
```

Choose deterministic scoring (0–100) so CI gating is reliable.

---

## Step 4 — Changelog + Migration Guidance (Embedded)

- No standalone step: drift and risk reports embed changelog sections and migration checklists.
- Narrative text can leverage LLMs, but only to summarize facts already present in `diff/changes.json` and lint results.
- Include “breaking spotlight” tables and client upgrade callouts directly in `drift.md` / `risk.md`.

---

## Step 5 — Regenerate OpenAPI + Proto + Markdown Docs (Doc-Enriched, With Examples)

**Authoritative contract outputs**

1. **OpenAPI regeneration**
   - Merge knowledge model annotations (improved summaries/descriptions, normalized tags, consistent naming) into `generated/openapi.swagger.json`.
   - Inject validated examples for every operation/response that passes schema validation. If validation fails, omit and log `example_status: "missing"`.
2. **Proto regeneration**
   - Even when only docs exist, the agentic synthesis pipeline drafts `.proto` definitions by:
     - Extracting RPC/Message candidates from doc chunks via structured prompts.
     - Validating field types/names against OpenAPI schemas when available.
     - Emitting `generated/proto/combined.proto` plus descriptor JSON.
3. **Reference docs**
   - `generated/reference.md` groups endpoints by tags/modules with usage guidance extracted from linked doc passages.
4. **Examples folder**
   - REST examples under `examples/rest/...`, gRPC under `examples/grpc/...`, plus `examples/snippets/` for language-specific samples.

**Outputs**

```
.specguard/out/generated/
  openapi.swagger.json
  proto/combined.proto
  reference.md
  examples/
    rest/
    grpc/
    snippets/
```

---

## Step 6 — Validate Documentation Consistency

- Compare docs vs specs vs release notes to flag outdated sections, missing endpoints, incorrect descriptions, or undocumented fields.
- Emit Markdown report with highlighted passages and remediation links back to source doc chunk IDs.

**Outputs**

```
.specguard/out/reports/
  doc_consistency.md
```

(Optional future: JSON variant for programmatic consumption.)

---

## Step 7 — Recommend REST vs gRPC Usage

- Analyze payload size / schema complexity, streaming requirements, latency hints, client environment cues, and existing protocol coverage per tag.
- Blend deterministic heuristics with doc-derived hints (keywords like “stream”, “browser-only”, “batch”).
- Produce side-by-side recommendation matrix plus rationale.

**Outputs**

```
.specguard/out/reports/
  protocol_recommendation.md
```

---

## Final Output Contract (Exact)

```
.specguard/out/
  knowledge_model.json
  spec_snapshot.json
  doc_index/

  diff/
    summary.json

  generated/
    openapi.swagger.json
    reference.md
    examples/
    proto/combined.proto

  reports/
    drift.md
    risk.md
    standards.md
    doc_consistency.md
    protocol_recommendation.md

  # Optional but recommended for debugging
  diff/changes.json
```

`changes.json` remains recommended for internal debugging even if not part of the “public” contract.

---

## Proto + OpenAPI Generation From Docs Only

When no machine-readable spec exists, SpecGuard must still produce authoritative contracts:

1. **Doc parsing via MCP connectors** — ingest PDFs/HTML/MD equally, preserving headings, tables, and parameter lists.
2. **Agentic extraction** — specialized planners break the task into:
   - Operation discovery (identify verbs, paths, RPC names).
   - Schema inference (enumerate fields, data types, constraints).
   - Example drafting (derive sample payloads).
3. **Cross-validation** — inferred schemas are validated against historical snapshots (if any) and doc consistency rules.
4. **Emitter** — outputs both OpenAPI and Proto representations so downstream SDK/doc tooling works uniformly.
5. **Confidence reporting** — attach `source_passages` + confidence scores per endpoint to `knowledge_model.json` so reviewers know what prose drove the spec.

---

## Architecture — Where MCP, RAG, Agentic AI, and Vector DB Fit

```
┌──────────────┐      ┌────────────────┐      ┌────────────────────┐
│ CLI / UI     │  →   │ MCP Connectors │  →   │ Ingestion Workers  │
│ (SpecGuard)  │      │ (files, GDrive)│      │ (Go + Go routines) │
└──────────────┘      └────────────────┘      └────────────────────┘
          │                                     │
          ▼                                     ▼
┌──────────────────────┐       ┌────────────────────────┐
│ Vector DB (pgvector) │◀────▶│ RAG Indexer            │
│ + doc_index/*.jsonl  │       │ (chunking + embeddings)│
└──────────────────────┘       └────────────────────────┘
          │                                     │
          ▼                                     ▼
┌──────────────────────────────┐
│ Agentic Orchestrator         │
│ - Builds knowledge model     │
│ - Generates specs/examples   │
│ - Coordinates doc-only flow  │
└──────────────────────────────┘
          │
          ▼
┌──────────────────────────────┐
│ Deterministic Engines        │
│ - Normalize OpenAPI/Proto    │
│ - Diff + classification      │
│ - Standards/risk scoring     │
└──────────────────────────────┘
          │
          ▼
┌──────────────────────────────┐
│ Artifact Builder             │
│ - Reports, regenerated specs │
│ - Docs, examples, protos     │
└──────────────────────────────┘
          │
          ▼
┌──────────────────────────────┐
│ Storage + Control Plane      │
│ - `.specguard/out` locally   │
│ - R2/S3 via upload command   │
│ - Dashboards + governance    │
└──────────────────────────────┘
```

**Key placements**

- **MCP** — abstracts doc acquisition (filesystem, Confluence, Drive) so ingestion is pluggable.
- **RAG + Vector DB** — power doc⇄spec linking, doc consistency checks, and doc-only spec generation prompts.
- **Agentic AI** — orchestrates multi-tool flows (normalize → align → synthesize) but never overrides deterministic truth.
- **Deterministic Go services** — enforce rule-based diffing, severity, risk, and standards.

---

## Implementation Checklist

1. **Wire up CLI flags/config** for doc inputs (`--docs-dir`, `--docs-upload`), proto roots, and git refs.
2. **Implement MCP ingestion adapters** inside `internal/ingest` for filesystem + remote sources.
3. **Add doc chunker + vector DB writer** (`internal/docindex`) with pluggable embedding provider.
4. **Build knowledge model composer** that merges spec + doc signals and emits mappings + provenance.
5. **Extend diff engine** to read knowledge model links for doc consistency checks.
6. **Update report generators** (`drift.md`, `risk.md`, `doc_consistency.md`, `protocol_recommendation.md`) to match final contract.
7. **Create doc-to-spec synthesizers** for OpenAPI + Proto when only docs are present; gate outputs behind validation + confidence metadata.
8. **Ensure artifact layout matches** the final directory contract before publishing.

Deliver these in order to align with the “SpecGuard v1 — Step-by-Step Detailed Execution (Final)” requirements.
