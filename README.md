<div align="center">

# SpecGuard

**Stop shipping API changes that break your customers' code.**

SpecGuard runs in your CI pipeline, produces a deterministic semantic diff of your OpenAPI and gRPC specs on every PR, scores the release risk, enforces 13 API standards rules, and posts a clear summary — so breaking changes are caught before they reach production.

[![CI](https://github.com/madhupathy/specguard/actions/workflows/ci.yml/badge.svg)](https://github.com/madhupathy/specguard/actions/workflows/ci.yml)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

</div>

---

## The Problem

A PR lands on Friday afternoon. Someone removed an enum value. Someone else made a request body field required. Your CI passes. You deploy. Monday morning, three enterprise customers file tickets — their SDKs are broken.

This happens because most teams treat API changes as code reviews, not contract reviews. There's no automated gate that asks: *"Does this change break any existing client?"*

SpecGuard is that gate.

---

## How It Works

```
PR opens
    │
    ▼
specguard scan          normalize OpenAPI + Proto specs into deterministic JSON snapshots
    │
    ▼
specguard diff          semantic diff: 36 change kinds across endpoints, schemas, params,
    │                   request bodies, response bodies, enums, security, and proto fields
    ▼
specguard report        risk score 0–100, 13 standards violations, doc consistency check
    │
    ▼
PR comment posted  ✅   "2 breaking · Risk 74/100 · enum value removed · field now required"
CI exits 1 on breaking  optional hard gate to block the merge
```

Every step is **deterministic** — the same spec always produces the same diff, the same diff always produces the same risk score. No flakiness, no LLM in the critical path.

---

## Quick Start

### Install

```bash
git clone https://github.com/madhupathy/specguard.git
cd specguard
go build -o specguard ./cmd/specguard/
```

### Initialize in your API repo

```bash
cd your-api-repo
./specguard init --repo .
```

This creates `.specguard/config.yaml`:

```yaml
version: 1
project:
  name: payments-api
inputs:
  openapi:
    - path: api/openapi.yaml
      base_ref: origin/main
policies:
  fail_on_breaking: true
  warn_on_potential: true
```

### Run your first diff

```bash
specguard scan --repo . --out .specguard/out
specguard diff --base-ref origin/main --head-ref HEAD --out .specguard/out
specguard report \
  --diff-changes .specguard/out/diff/changes.json \
  --spec .specguard/out/snapshot/openapi.normalized.json \
  --out .specguard/out/reports
```

### Add to GitHub Actions

```yaml
# .github/workflows/specguard.yml
name: SpecGuard
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
          specguard report \
            --diff-changes .specguard/out/diff/changes.json \
            --spec .specguard/out/snapshot/openapi.normalized.json \
            --diff-summary .specguard/out/diff/summary.json \
            --knowledge .specguard/out/snapshot/manifest.json \
            --out .specguard/out/reports
      - uses: actions/upload-artifact@v4
        with:
          name: specguard-artifacts
          path: .specguard/out
```

---

## What SpecGuard Detects

### 36 change kinds across 5 categories

#### Endpoints & Methods
| Change | Severity |
|--------|----------|
| Endpoint removed | 🔴 Breaking |
| HTTP method removed (`GET /users` → gone) | 🔴 Breaking |
| Endpoint added | 🟢 Non-breaking |

#### Parameters
| Change | Severity |
|--------|----------|
| Parameter removed | 🔴 Breaking |
| Parameter location changed (`path` → `query`) | 🔴 Breaking |
| Parameter type changed (`string` → `integer`) | 🔴 Breaking |
| Optional parameter became required | 🔴 Breaking |
| Parameter added (optional) | 🟢 Non-breaking |

#### Request Body
| Change | Severity |
|--------|----------|
| Request body removed | 🔴 Breaking |
| Request body became required | 🔴 Breaking |
| Required field added to request schema | 🔴 Breaking |
| Property type changed in request schema | 🔴 Breaking |
| Request body added | 🟠 Potential |

#### Schemas & Enums
| Change | Severity |
|--------|----------|
| Component schema removed | 🔴 Breaking |
| Property removed | 🔴 Breaking |
| Property type changed | 🔴 Breaking |
| Field became required | 🔴 Breaking |
| **Enum value removed** | 🔴 Breaking — clients sending the old value get 400s |
| Enum constraint removed | 🟠 Potential |
| Security scheme removed | 🔴 Breaking |
| Property added | 🟢 Non-breaking |
| Enum value added | 🟢 Non-breaking |

#### gRPC / Protobuf
| Change | Severity |
|--------|----------|
| **Proto field number reused** | 🔴 Breaking — reassigning a field number corrupts binary wire encoding |
| Proto field removed | 🔴 Breaking |
| Proto field type changed | 🔴 Breaking |
| gRPC service removed | 🔴 Breaking |
| gRPC method removed | 🔴 Breaking |

#### Response Schemas
Response body schemas (properties, types, required fields) are diffed for every status code, not just component schemas.

---

## 13 API Standards Rules

Each rule produces violations with a remediation hint:

| Rule | ID | What it checks |
|------|----|----------------|
| Consistent error shape | STD-001 | Every endpoint documents at least one 4xx/5xx response |
| Pagination consistency | STD-002 | List endpoints use the same pagination style across the API |
| Versioning present | STD-003 | `info.version` set and/or URI versioning (`/v1/`) present |
| `operationId` present | STD-004 | Every endpoint has a unique `operationId` for SDK generation |
| Examples present | STD-005 | Response media types have `example` or `examples` |
| Auth documented | STD-006 | `securitySchemes` and global `security` defined |
| Field naming convention | STD-007 | snake_case or camelCase — not mixed in the same spec |
| Nullable discipline | STD-008 | No field that is both `required` and `nullable` |
| HTTP status coverage | STD-009 | Every endpoint documents a 2xx success response |
| Deprecation markers | STD-010 | Deprecated endpoints have migration guidance in description |
| **Enum documentation** | STD-011 | Enum properties have a description explaining each value |
| **Sunset header** | STD-012 | Deprecated endpoints advertise a `Sunset` header (RFC 8594) |
| **Request body content type** | STD-013 | `requestBody` blocks specify at least one content type |

---

## Risk Score (0–100)

A single number representing the release risk of this diff:

```
Risk Score: 74/100 (HIGH)

Contributing factors:
  • 2 breaking changes      +60
  • 1 potential breaking     +12
  • 3 standards violations    +6
  • 4 doc-only changes        +12
```

| Score | Grade | What it means |
|-------|-------|---------------|
| 85–100 | CRITICAL | Breaking changes that will likely cause immediate client failures |
| 65–84 | HIGH | Breaking or high-risk changes — require careful coordination |
| 40–64 | MEDIUM | Potential breakage or significant quality issues |
| 20–39 | LOW | Minor changes, documentation updates |
| 0–19 | INFO | Safe additive changes only |

Scoring formula: `breaking×30 + potential_breaking×12 + mutations×8 + doc_only×3 + standards_violations×2`

---

## Web Dashboard

SpecGuard ships a local dashboard for browsing spec history, change timelines, and artifacts.

```bash
# Backend (SQLite — no Postgres needed)
./specguard serve --port 8080

# Frontend
cd web && npm install && npm run dev
# → http://localhost:3001
```

| Page | What you see |
|------|-------------|
| **Dashboard** | Health status, repo stats, recent changes with severity badges |
| **Repositories** | Add repos, trigger scans from local `.specguard/out/` |
| **Specs** | Uploaded specs, normalized content, branch/commit metadata |
| **Changes** | Full change history — kind label, severity, impact score, metadata |
| **Reports** | Drift, risk, standards, doc consistency reports |
| **Artifacts** | Download reports, spec snapshots |
| **Settings** | LLM and Git connector configuration |

---

## CLI Reference

| Command | Description |
|---------|-------------|
| `specguard init` | Create `.specguard/config.yaml` workspace |
| `specguard scan` | Normalize OpenAPI + Proto into deterministic snapshots |
| `specguard diff` | Semantic diff between two spec snapshots (36 change kinds) |
| `specguard report` | Generate drift, risk, standards, and doc consistency reports |
| `specguard serve` | Start SQLite-backed API server + web dashboard |
| `specguard sdk generate` | Generate TypeScript/Go SDK from spec *(planned)* |
| `specguard docs generate` | Generate reference docs + static site *(planned)* |
| `specguard ci github` | Post PR comment + exit codes *(planned)* |
| `specguard upload` | Push artifacts to hosted control plane *(planned)* |

### Report flags

```bash
# Standards analysis (13 rules)
specguard report --spec .specguard/out/snapshot/openapi.normalized.json

# Drift report from diff output
specguard report --diff-changes .specguard/out/diff/changes.json

# Risk score (needs both)
specguard report \
  --diff-summary .specguard/out/diff/summary.json \
  --knowledge   .specguard/out/snapshot/manifest.json

# Doc consistency (spec vs documentation chunks)
specguard report --chunks .specguard/out/doc_index/
```

---

## GitHub Webhook

SpecGuard receives GitHub webhooks with HMAC-SHA256 signature verification:

```bash
# Set your webhook secret
GITHUB_WEBHOOK_SECRET=your-secret ./specguard serve

# GitHub webhook URL: https://your-server/api/v1/webhooks/github
# Events: pull_request, push, ping
```

---

## Project Structure

```
specguard/
├── cmd/specguard/
│   ├── main.go           # Cobra CLI entry point
│   ├── serve.go          # SQLite-backed API server + all REST handlers
│   └── serve_ext.go      # Connector endpoints (LLM, Git)
├── internal/
│   ├── scan/             # OpenAPI (kin-openapi) + Proto normalization
│   ├── diff/
│   │   ├── diff.go       # Shallow snapshot diff
│   │   ├── deep.go       # Full semantic diff — 36 change kinds
│   │   ├── classify.go   # Breaking / potential / non-breaking classification
│   │   └── drift_report.go
│   ├── standards/        # 13 API standards rules engine
│   ├── risk/             # Risk score 0–100 with grade and findings
│   ├── docconsistency/   # Spec ↔ documentation cross-check
│   ├── vectordb/         # SQLite vector store (cosine similarity)
│   ├── rag/              # RAG retrieval over doc chunks
│   └── protocol/         # REST vs gRPC recommender
├── web/                  # Next.js 14 + Tailwind + shadcn/ui dashboard
├── agent/                # Python LangChain agent wrapping the CLI
└── docs/                 # Architecture and execution plan docs
```

---

## Running Tests

```bash
go test ./internal/... -v -race
```

---

## Environment Setup

```bash
cp .env.example .env
```

| Variable | Required | Description |
|----------|----------|-------------|
| `PORT` | No | API server port (default: `8080`) |
| `GITHUB_WEBHOOK_SECRET` | Prod | HMAC secret for webhook verification |
| `AI_API_KEY` | Optional | OpenAI key for AI-powered risk narrative summaries |
| `DB_HOST` / `DB_PASSWORD` | Prod only | Postgres credentials (SQLite used for local dev) |

> **Never commit `.env`** — it is gitignored.

---

## Roadmap

- [x] `specguard init`, `scan`, `diff`, `report`, `serve`
- [x] 36 change kinds: enums, request bodies, inline schemas, proto field numbers
- [x] 13 API standards rules
- [x] Risk scoring (0–100) with grade and findings
- [x] GitHub webhook with HMAC-SHA256 verification
- [x] Web dashboard (Next.js 14)
- [ ] `specguard ci github` — PR comment bot + exit codes
- [ ] `specguard sdk generate` — TypeScript + Go SDK generation
- [ ] `specguard docs generate` — reference docs + static site
- [ ] `specguard upload` — push artifacts to hosted control plane
- [ ] Hosted SaaS at `specguard.dev`

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). Run `go test ./internal/... -v` before submitting a PR. Never commit `.env` files or credentials.

## License

MIT — see [LICENSE](LICENSE).
