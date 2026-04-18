# SpecGuard

> **Production-grade API change guardrail** — deterministic diffs, drift reports, risk scoring, and a web dashboard for OpenAPI and gRPC specs.

[![CI](https://github.com/madhupathy/specguard/actions/workflows/ci.yml/badge.svg)](https://github.com/madhupathy/specguard/actions/workflows/ci.yml)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22%2B-blue)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow)](LICENSE)

SpecGuard catches breaking API changes before they reach production. The CLI runs inside your CI pipeline and produces deterministic artifacts — diffs, reports, SDK zips, docs — while the optional control plane stores metadata and powers a team dashboard.

---

## Screenshots

> **Dashboard** — health status, repo stats, and recent activity  
> ![SpecGuard Dashboard](docs/screenshots/dashboard.png)
>
> *(Add your own screenshots to `docs/screenshots/` and update the paths above)*

---

## Features

| Capability | Description |
|---|---|
| **Deterministic diffs** | Semantic REST + gRPC change detection — no flaky output |
| **Breaking-change detection** | Classifies changes as breaking / potential / non-breaking |
| **Risk scoring** | 0–100 risk score aggregated from breaking changes, standards, docs |
| **10 API standards rules** | Pagination, versioning, operationId, auth docs, naming, deprecation |
| **Doc consistency** | Cross-checks spec endpoints against documentation chunks |
| **RAG + vector search** | SQLite-backed vector store with cosine similarity |
| **Web dashboard** | Next.js + Tailwind UI to browse repos, specs, changes, and artifacts |
| **Python agent** | LangChain ReAct agent wrapping the CLI for AI-assisted workflows |

---

## Quick Start

### Prerequisites

- **Go ≥ 1.22**
- **Node.js ≥ 18** (for the web dashboard only)

### Install

```bash
git clone https://github.com/madhupathy/specguard.git
cd specguard
go build -o specguard ./cmd/specguard/
```

### Run the dashboard

```bash
# 1. Start the API server (SQLite, no Postgres needed)
./specguard serve --port 8080

# 2. Start the frontend (separate terminal)
cd web && npm install && npm run dev
# → http://localhost:3001
```

### Initialize in your API repo

```bash
cd your-api-repo
specguard init --repo .
specguard scan --repo . --out .specguard/out
specguard diff --base-ref origin/main --head-ref HEAD --out .specguard/out
specguard report --diff-changes .specguard/out/diff/changes.json --out .specguard/out/reports
```

---

## Environment Setup

Copy the example and fill in your values:

```bash
cp .env.example .env
```

| Variable | Required | Description |
|---|---|---|
| `PORT` | No | API server port (default: `8080`) |
| `DB_HOST` | Prod only | Postgres host |
| `DB_PASSWORD` | Prod only | Postgres password |
| `GITHUB_WEBHOOK_SECRET` | Prod only | GitHub App webhook secret |
| `AI_API_KEY` | Optional | OpenAI key for narrative summaries |

> ⚠️ **Never commit `.env`** — it is gitignored. Use `.env.example` to document variables.

---

## CLI Reference

| Command | Description |
|---|---|
| `specguard init` | Create `.specguard/config.yaml` + workspace |
| `specguard scan` | Normalize OpenAPI + Proto specs into snapshots |
| `specguard diff` | Deep semantic diff between two snapshots |
| `specguard report` | Generate drift, risk, standards, and doc-consistency reports |
| `specguard serve` | Start API server + web dashboard (SQLite-backed) |

**Report flags:**

```bash
specguard report --spec .specguard/out/snapshot/openapi.normalized.json   # standards
specguard report --chunks .specguard/out/snapshot/                         # doc consistency
specguard report --diff-changes .specguard/out/diff/changes.json           # drift
specguard report --diff-summary ... --knowledge ...                        # risk score
```

---

## GitHub Actions Integration

Add SpecGuard to your CI in `.github/workflows/specguard-pr.yml`:

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
        with: { fetch-depth: 0 }
      - uses: actions/setup-go@v5
        with: { go-version: "1.22" }
      - run: go install github.com/madhupathy/specguard/cmd/specguard@latest
      - run: |
          specguard scan --repo . --out .specguard/out
          specguard diff --base-ref origin/main --head-ref HEAD --out .specguard/out
          specguard report --diff-changes .specguard/out/diff/changes.json --out .specguard/out/reports
          specguard ci github --reports .specguard/out/reports --comment-out .specguard/out/ci/comment.md --fail-on-breaking
      - uses: actions/upload-artifact@v4
        with: { name: specguard-artifacts, path: .specguard/out }
```

---

## Dashboard Pages

| Page | URL | Description |
|---|---|---|
| Dashboard | `/` | Health status, stats, recent activity |
| Repositories | `/repositories` | List, create, scan from disk |
| Specs | `/specs` | Upload spec files, view content + changes |
| Changes | `/changes` | Breaking/potential/non-breaking with severity badges |
| Artifacts | `/artifacts` | Download reports, SDK zips, docs |
| Settings | `/settings` | System health, config reference |

---

## Architecture

```
Browser (:3001)  →  Next.js (proxy /api/* → :8080)  →  Go Gin API  →  SQLite
```

```
specguard/
  cmd/specguard/      # Cobra CLI entry point + API server
  internal/
    projectconfig/    # .specguard/config.yaml
    scan/             # OpenAPI + Proto normalization
    diff/             # Semantic diff + classification
    report/           # Report generation
    risk/             # Risk scoring
    standards/        # 10 API standards rules
    docconsistency/   # Doc vs spec consistency
    vectordb/         # SQLite vector store (cosine similarity)
    rag/              # RAG retrieval
    protocol/         # REST vs gRPC recommender
  web/                # Next.js 14 + Tailwind + shadcn/ui
  agent/              # Python LangChain agent
  docs/               # Pipeline docs
```

---

## Running Tests

```bash
go test ./internal/diff/ ./internal/risk/ ./internal/report/ \
       ./internal/docindex/ ./internal/standards/ ./internal/docconsistency/ \
       ./internal/vectordb/ ./internal/rag/ ./internal/protocol/ -v
```

---

## Roadmap

- [x] `specguard init`, `scan`, `diff`, `report`, `serve`
- [ ] `specguard sdk generate` / `specguard sdk package`
- [ ] `specguard docs generate`
- [ ] `specguard ci github` (PR comment + exit codes)
- [ ] `specguard upload` (control plane ingestion)
- [ ] Hosted SaaS dashboard at `docs.specguard.dev`

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for the full guide. In short:

1. Fork → branch → commit → PR
2. Run `go test ./internal/...` before pushing
3. Never commit `.env` files or credentials

---

## License

MIT — see [LICENSE](LICENSE).
