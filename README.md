@ -1,304 +1,338 @@
# SpecGuard

SpecGuard is a production-grade API change guardrail. The CLI delivers deterministic diffs, reports, SDKs, and docs straight from customer CI, while the control plane stores metadata, powers dashboards, and handles artifact retrieval. This document captures the reference architecture, CLI contract, artifact layout, and the execution plan for shipping v1.

> **Need the finalized SpecGuard v1 pipeline?** See [docs/specguard_v1_execution.md](docs/specguard_v1_execution.md) for the authoritative step-by-step instructions, architecture diagram, and artifact contract that the CLI + UI must implement.

## 1. Production Architecture

### 1.1 Two-plane model

- **Data plane (customer CI / runners)**
  - Runs inside GitHub Actions (PR + main) or any containerized runner.
  - Executes `specguard` commands to generate deterministic artifacts:
    - `snapshot/openapi.normalized.json`
    - `snapshot/proto.normalized.json`
    - `diff/changes.json`, `diff/summary.json`
    - Report JSON/Markdown pairs, SDK zips, docs tarball, CI comment payload
  - Uploads artifacts to GH Actions storage and/or to the SpecGuard control plane via presigned URLs.
  - No long-lived repo credentials required.

- **Control plane (SpecGuard SaaS)**
  - Receives run metadata, stores org/repo config, exposes dashboards for Drift/Risk/Standards.
  - Issues presigned upload/download URLs for artifact storage (R2/S3).
  - Later: scheduled scans, hosted runners, runtime traffic drift.

### 1.2 Services (production-grade)

| Service | Responsibilities |
| --- | --- |
| **GitHub App** | Receives PR opened/sync + push-to-main webhooks; creates check-runs linking back to SpecGuard; can delegate PR comments to CI initially. |
| **SpecGuard API (Go)** | Auth via GitHub installation tokens/JWTs; org/repo registration; `POST /v1/runs/ingest`; `POST /v1/runs/{run_id}/artifacts/presign`; report retrieval. |
| **Worker pool** | Optional early; later executes hosted analyses, scheduled scans, doc builds, SDK packaging. |
| **Postgres** | Multi-tenant metadata: orgs, repos, installations, runs, spec snapshots, changes, artifacts, policies. |
| **Artifact store (R2/S3)** | Immutable storage with stable keys: `org/<orgId>/repo/<repoId>/run/<runId>/...`. |

## 2. CLI Specification (V1)

### 2.1 Philosophy

- CLI is the engine; SaaS is control-plane UX. Everything runs offline-first and deterministically.
- Commands share the layout `.specguard/` inside the repo.
- Severity decisions and change IDs are deterministic—LLMs are only for narrative text (later).

### 2.2 Commands and outputs

| Command | Purpose | Key Outputs |
| --- | --- | --- |
| `specguard init --repo .` | Create `.specguard/config.yaml` + workspace. | Config per template below. |
| `specguard scan --repo . --out .specguard/out` | Normalize OpenAPI + Proto specs into canonical JSON/descriptors. | `snapshot/openapi.normalized.json`, `snapshot/proto.normalized.json`, `snapshot/manifest.json`. |
| `specguard diff --base-ref origin/main --head-ref HEAD --out .specguard/out` | Produce canonical change list for REST + gRPC. | `diff/changes.json`, `diff/summary.json`. |
| `specguard report --in .../changes.json --out .../reports` | Render drift, risk, standards, SDK sync, docs quality (JSON + Markdown). | `reports/*.json`, `reports/*.md`. |
| `specguard sdk generate ...` | Generate SDK assets (TS from OpenAPI, Go from Proto). | Language-specific folders. |
| `specguard sdk package --out .../sdk` | Zip SDKs + emit checksums. | `specguard-typescript-sdk.zip`, `specguard-go-sdk.zip`, `sdk/checksums.json`. |
| `specguard docs generate --openapi ... --proto ... --out .../docs` | Produce markdown + static site + tarball. | `docs/reference.md`, `docs/site/`, `docs/site.tar.gz`. |
| `specguard ci github --reports ... --comment-out ... --fail-on-breaking` | Compose PR comment markdown + exit codes (0 ok, 2 breaking, 3 tool error). | `.specguard/out/ci/comment.md`, stdout. |
| `specguard upload --org ... --repo-id ... --run ... --dir ... --token ...` | Push artifacts + metadata to control plane. | None (API interaction). |

### 2.3 Config template (`.specguard/config.yaml`)

```yaml
version: 1
project:
  name: payments-api

inputs:
  openapi:
    - path: api/openapi.yaml
      base_ref: origin/main
  protobuf:
    - root: proto
      include:
        - "**/*.proto"
      base_ref: origin/main

outputs:
  dir: .specguard/out

sdks:
  languages: ["typescript", "go"]
  mode: artifact

docs:
  site: true
  markdown: true

policies:
  fail_on_breaking: true
  warn_on_potential: true

standards:
  ruleset: specguard-default
```

### 2.4 Canonical change model (`diff/changes.json`)

```json
{
  "id": "chg_xxx",
  "protocol": "rest",
  "type": "endpoint.removed",
  "severity": "breaking",
  "resource": {
    "rest": {"method": "POST", "path": "/payments"},
    "grpc": null
  },
  "details": {"from": "v1", "to": null},
  "remediation": "Update client routing; endpoint removed."
}
```

The schema covers both REST (`resource.rest`) and gRPC (`resource.grpc.service`, `rpc`, `message`, `field`). Severity is rule-driven and deterministic.

## 3. Artifact layout (stable)

```
.specguard/out/
  snapshot/
    manifest.json
    openapi.normalized.json
    proto.normalized.json
  diff/
    changes.json
    summary.json
  reports/
    drift.json        drift.md
    risk.json         risk.md
    standards.json    standards.md
    sdk_sync.json     sdk_sync.md
    docs_quality.json docs_quality.md
  sdk/
    typescript/
    go/
    specguard-typescript-sdk.zip
    specguard-go-sdk.zip
    checksums.json
  docs/
    reference.md
    site/
    site.tar.gz
  ci/
    comment.md
```

## 4. Standards + Risk Reports

### 4.1 Standards rules (V1)

1. Consistent error shape documented (REST + gRPC guidance).
2. Pagination style consistent across list endpoints.
3. Versioning present and consistent (URI or headers).
4. `operationId` present for REST endpoints.
5. Request + response examples for public endpoints.
6. Auth documented (global security scheme / per-RPC notes).
7. Field naming convention consistency (snake_case vs camelCase).
8. Nullable/optional discipline (no ambiguous nulls; proto optional usage correct).
9. HTTP status coverage documented per REST operation.
10. Deprecation markers set when endpoints/fields are sunset.

Each rule emits violations with remediation suggestions inside `reports/standards.*`.

### 4.2 Risk scoring (0–100)

Risk aggregates deterministic signals:

- Breaking changes (highest weight)
- Potential breaking changes (medium)
- Standards violations (medium)
- Missing docs/examples (medium)
- SDK out-of-sync (high)

`reports/risk.*` includes the numeric score, contributing factors, and guidance.

## 5. GitHub Actions reference

### 5.1 PR workflow (`.github/workflows/specguard-pr.yml`)

```yaml
name: SpecGuard (PR)

on:
  pull_request:
    types: [opened, synchronize, reopened]

jobs:
  specguard:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      pull-requests: write

    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - uses: actions/setup-go@v5
        with:
          go-version: "1.22"

      - name: Install SpecGuard CLI
        run: go install github.com/YOURORG/specguard/cmd/specguard@latest

      - name: Run SpecGuard
        run: |
          specguard scan --repo . --out .specguard/out
          specguard diff --base-ref origin/main --head-ref HEAD --out .specguard/out
          specguard report --in .specguard/out/diff/changes.json --out .specguard/out/reports
          specguard docs generate --openapi .specguard/out/snapshot/openapi.normalized.json --proto .specguard/out/snapshot/proto.normalized.json --out .specguard/out/docs
          specguard sdk generate --lang typescript --spec .specguard/out/snapshot/openapi.normalized.json --out .specguard/out/sdk/typescript
          specguard sdk generate --lang go --proto .specguard/out/snapshot/proto.normalized.json --out .specguard/out/sdk/go
          specguard sdk package --out .specguard/out/sdk
          specguard ci github --reports .specguard/out/reports --comment-out .specguard/out/ci/comment.md --fail-on-breaking

      - uses: actions/upload-artifact@v4
        with:
          name: specguard-artifacts
          path: .specguard/out

      - name: Post PR comment
        uses: actions/github-script@v7
        with:
          script: |
            const fs = require('fs');
            const body = fs.readFileSync('.specguard/out/ci/comment.md', 'utf8');
            await github.rest.issues.createComment({
              ...context.repo,
              issue_number: context.payload.pull_request.number,
              body
            });
```

### 5.2 Main workflow (artifact publishing)

Mirror the PR job but trigger on `push` to `main`. Use `HEAD~1` as base or the previous stored snapshot, upload artifacts with retention, and optionally publish docs to Pages or the control plane.

## 6. Execution plan (Milestones)

1. **Milestone 0 — Repo skeleton**: Cobra-based CLI (`specguard init|scan|diff|report|sdk|docs|ci|upload`) plus internal packages (`config`, `openapi`, `proto`, `normalize`, `diff`, `classify`, `report`, `standards`, `sdk`, `docs`, `ci`, `artifacts`). Deliverable: `specguard --help` + `specguard init` scaffold.
2. **Milestone 1 — OpenAPI scan**: Normalize OpenAPI (kin-openapi, `$ref` resolution, sorted keys). Output `snapshot/openapi.normalized.json` + manifest entries.
3. **Milestone 2 — Proto scan**: Generate descriptor set (via `buf` or `protoc`), normalize to JSON, produce `snapshot/proto.normalized.json` + manifest update.
4. **Milestone 3 — Diff engine**: Build REST + gRPC diff pipelines, emit canonical `changes.json`, `summary.json`.
5. **Milestone 4 — Classification rules**: Deterministic severity rule-set, unit tests.
6. **Milestone 5 — Reports**: Drift, risk, standards, sdk-sync, docs-quality (JSON + Markdown templates).
7. **Milestone 6 — Docs generator**: `docs/reference.md`, static site, `site.tar.gz` bundle.
8. **Milestone 7 — SDK generation**: TS (OpenAPI-based), Go (protoc-gen-go/grpc) with checksums and zips.
9. **Milestone 8 — CI GitHub integration**: `specguard ci github`, PR comment rendering, exit codes.
10. **Milestone 9 — Control plane stub**: Run ingestion endpoint, artifact presign, minimal dashboard (post-CLI adoption).

## 7. PR comment format (reference)

```
SpecGuard Report ✅/❌

Drift: 12 changes (3 breaking, 4 potential, 5 non-breaking)
Risk Score: 82/100 (HIGH)
Standards: 6 violations
Docs: 74% endpoints have examples
SDK: TS ✅ regenerated, Go ✅ regenerated

Breaking changes
- REST POST /payments: required field added `currency`
- gRPC Payments.CreatePayment: `amount` type changed `int32 → int64`

Artifacts
📄 Reports: attached via CI artifacts
📦 SDK zips: specguard-typescript-sdk.zip, specguard-go-sdk.zip
🌐 Docs site: site.tar.gz
```

## 8. Web GUI (Dashboard)

SpecGuard ships a modern web dashboard built with **Next.js 14 + Tailwind CSS + shadcn/ui** that connects to the Go backend API server.

### 8.1 Quick Start

```bash
# 1. Start the backend API server (SQLite, no Postgres needed)
cd ~/specguard
go build -o specguard ./cmd/specguard/
./specguard serve --port 8080

# 2. Start the frontend dev server
cd ~/specguard/web
npm install
npm run dev
```

Open **http://localhost:3001** in your browser.

### 8.2 Pages

| Page | URL | Description |
|------|-----|-------------|
| Dashboard | `/` | Health status, stats cards (repos/specs/changes/artifacts), recent activity |
| Repositories | `/repositories` | List, create (with local path), scan from disk, detail view |
| Specs | `/specs` | Filterable list, upload spec files, detail with content/changes/metadata tabs |
| Changes | `/changes` | Breaking/deprecation/non-breaking stats, severity badges, AI summary |
| Artifacts | `/artifacts` | List with type icons, size, download links |
| Settings | `/settings` | System health, config reference (DB, Storage, GitHub, AI) |

### 8.3 How to Add Data

**Option A — Point to a local repository (recommended)**

1. Go to **Repositories** → **Add Repository**
2. Enter a **Repository Name** (e.g. `my-api-project`)
3. Enter the **Local Path** on disk (e.g. `/path/to/your/api-project`)
4. Click **Create Repository**

SpecGuard auto-scans the `.specguard/out/` directory and imports:
- Normalized specs (`snapshot/openapi.normalized.json`, `proto.normalized.json`)
- Report summaries (`report_summary.json`)
- Diff changes (`diff/changes.json`)

You can re-scan anytime using the **Scan** button on the repo card.

> **Tip:** Run `specguard scan` in the target repo first to generate `.specguard/out/` artifacts, then point the GUI to it.

**Option B — Upload a spec file directly**

1. Go to **Specs** → **Upload Spec**
2. Select a file (`.json`, `.yaml`, `.proto`)
3. Pick the target repository, version, branch, and spec type
4. Click **Upload**

### 8.4 Backend API Endpoints

The `specguard serve` command starts a Gin-based API server backed by SQLite.

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/health` | Health check (status, version, timestamp) |
| `GET` | `/api/v1/repositories` | List all repositories |
| `POST` | `/api/v1/repositories` | Create repository (accepts `name`, `url`, `local_path`) |
| `GET` | `/api/v1/repositories/:id` | Get repository detail |
| `POST` | `/api/v1/repositories/:id/scan` | Scan local `.specguard/out/` and import specs/diffs/artifacts |
| `GET` | `/api/v1/specs` | List specs (filterable by `repo_id`, `branch`) |
| `POST` | `/api/v1/specs` | Create spec via JSON |
| `POST` | `/api/v1/specs/upload` | Upload spec file (multipart form) |
| `GET` | `/api/v1/specs/:id` | Get spec detail with content |
| `GET` | `/api/v1/specs/:id/changes` | List changes for a spec |
| `GET` | `/api/v1/changes` | List all changes (filterable by `spec_id`, `change_type`) |
| `GET` | `/api/v1/changes/:id` | Get change detail with AI summary |
| `GET` | `/api/v1/artifacts` | List artifacts (filterable by `spec_id`, `artifact_type`) |
| `GET` | `/api/v1/artifacts/:id` | Get artifact detail |
| `GET` | `/api/v1/artifacts/:id/download` | Download artifact |

### 8.5 Architecture

```
Browser (:3001)  →  Next.js (proxy /api/* → :8080)  →  Go Gin API  →  SQLite
```

- Frontend rewrites `/api/*` to the backend via `next.config.mjs`
- Backend uses SQLite (file: `specguard.db`) — no Postgres required for local dev
- Data persists across restarts in `specguard.db`

## 9. Development setup

- Go ≥ 1.22
- Node.js ≥ 18 (for the web GUI)
- `go mod tidy` to sync dependencies (`cobra`, `kin-openapi`, etc.).
- Run `specguard init` inside any API repo to bootstrap `.specguard/config.yaml`.

## 10. License & Support

- MIT License — see [LICENSE](LICENSE).
- Issues & discussions: GitHub repo.
- Future docs site + SaaS dashboard details will live at `docs.specguard.dev`.

## 11. Current Implementation Status

### Working Commands
- `specguard init` — Create `.specguard/config.yaml` + workspace
- `specguard scan` — Normalize OpenAPI + Proto + doc index into deterministic snapshots
- `specguard diff` — Deep semantic diff between two `spec_snapshot.json` files (endpoints, schemas, params, responses, proto services)
- `specguard serve` — Start the API server + web dashboard (SQLite-backed, no Postgres needed)
- `specguard report` — Full report suite with flags:
  - `--spec` — Standards analysis (10 rules) → `reports/standards.{json,md}`
  - `--chunks` — Doc consistency analysis → `reports/doc_consistency.md`
  - `--diff-changes` — Drift report → `reports/drift.md`
  - `--diff-summary` + `--knowledge` — Risk scoring (with standards violations factored in) → `reports/risk.{json,md}` + `reports/protocol_recommendation.md`

### Planned Commands (not yet implemented)
- `specguard sdk generate` / `specguard sdk package`
- `specguard docs generate`
- `specguard ci github`
- `specguard upload`

### Project Structure

```
specguard/
  cmd/specguard/
    main.go                # Cobra CLI entry point (init, scan, diff, report, serve)
    serve.go               # API server with SQLite backend + all REST handlers
  internal/
    projectconfig/         # .specguard/config.yaml load/write/resolve
    scan/                  # OpenAPI + Proto normalization, doc index, manifest
    diff/                  # Shallow + deep diff, classification rules, drift report
    report/                # Manifest summary generation
    risk/                  # Risk scoring (with standards violations) + protocol rec
    docindex/              # PDF/MD/TXT extraction, chunking, pseudo-embeddings
    standards/             # 10 API standards rules engine (JSON + MD reports)
    docconsistency/        # Doc chunks vs spec endpoint consistency checking
    vectordb/              # SQLite-backed vector store with cosine similarity search
    rag/                   # RAG retrieval: ingest chunks, link endpoints to docs
    protocol/              # SOAF-inspired per-endpoint REST vs gRPC recommender
  web/                     # Next.js 14 + Tailwind + shadcn/ui dashboard
    src/app/               # Pages: dashboard, repos, specs, changes, artifacts, settings
    src/components/        # Sidebar, shadcn/ui components (button, card, badge, etc.)
    src/lib/utils.ts       # API fetch helper, time formatting, severity colors
    next.config.mjs        # Proxy /api/* → backend on :8080
  agent/                   # Python LangChain ReAct agent wrapping specguard CLI
  docs/                    # Pipeline documentation
```

### Running Tests

```bash
go test ./internal/diff/ ./internal/risk/ ./internal/report/ ./internal/docindex/ \
       ./internal/standards/ ./internal/docconsistency/ ./internal/vectordb/ \
       ./internal/rag/ ./internal/protocol/ -v
```

## 12. Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## 13. License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
