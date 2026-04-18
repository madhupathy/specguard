<div align="center">

# SpecGuard

**Catch breaking API changes before your customers do.**

SpecGuard runs in your CI pipeline, computes a deterministic semantic diff of your OpenAPI and gRPC specs on every PR, scores the risk, enforces your API standards, and posts a clear summary comment — so breaking changes are never a surprise on release day.

[![CI](https://github.com/madhupathy/specguard/actions/workflows/ci.yml/badge.svg)](https://github.com/madhupathy/specguard/actions/workflows/ci.yml)
[![Go 1.22+](https://img.shields.io/badge/Go-1.22+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

</div>

---

## The Problem

Your team ships a PR. Someone removed a required field. Renamed an endpoint. Tightened an enum. Nothing fails in staging — but 48 hours after deploy, your biggest customer files a ticket. Their SDK broke.

This happens because:
- **API diffs live in human heads**, not in CI
- **Reviewer fatigue** means spec changes get approved without a full diff read
- **Breaking vs. non-breaking** is a judgment call, not a checked rule
- **Risk is invisible** — nobody knows if this change + that open SDK issue + those missing examples = a bad release

SpecGuard makes API contract review automatic, deterministic, and impossible to skip.

---

## How It Works

```
Your PR opens
     │
     ▼
specguard scan          ← normalize OpenAPI + Proto into deterministic JSON snapshots
     │
     ▼
specguard diff          ← semantic diff: endpoint removed? field type changed? enum narrowed?
     │
     ▼
specguard report        ← risk score (0–100), standards violations, doc consistency
     │
     ▼
PR comment posted  ✅   ← "3 breaking changes · Risk: 82/100 · 6 standards violations"
CI fails on breaking ❌  ← optional hard gate
```

Every step is **deterministic** — the same spec always produces the same diff, the same diff always produces the same risk score. No LLM flakiness in the critical path.

---

## Quick Start

### 1. Install

```bash
git clone https://github.com/madhupathy/specguard.git
cd specguard
go build -o specguard ./cmd/specguard/
```

### 2. Initialize in your API repo

```bash
cd your-api-repo
specguard init --repo .
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

### 3. Run your first diff

```bash
specguard scan --repo . --out .specguard/out
specguard diff --base-ref origin/main --head-ref HEAD --out .specguard/out
specguard report --diff-changes .specguard/out/diff/changes.json --out .specguard/out/reports
```

### 4. Add to GitHub Actions

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
          specguard report --diff-changes .specguard/out/diff/changes.json \
                           --spec .specguard/out/snapshot/openapi.normalized.json \
                           --out .specguard/out/reports
      - uses: actions/upload-artifact@v4
        with:
          name: specguard-artifacts
          path: .specguard/out
```

---

## What SpecGuard Detects

### Breaking Changes (fail CI by default)
| Change | Example |
|--------|---------|
| Endpoint removed | `DELETE /payments` |
| Required field added | `currency` now required in request body |
| Field type changed | `amount: integer` → `amount: string` |
| Enum value removed | `status` no longer accepts `"pending"` |
| Auth scheme changed | `Bearer` → `ApiKey` |
| gRPC field number changed | Field 3 moved to field 7 |

### Potential Breaking Changes (warn)
Optional fields becoming required, response field removals, default value changes, HTTP method changes.

### Standards Violations (10 rules)
| Rule | What it checks |
|------|---------------|
| `consistent-error-shape` | Every error response follows the same schema |
| `pagination-consistency` | List endpoints use the same cursor/page pattern |
| `versioning-present` | URI or header versioning exists and is consistent |
| `operation-id-present` | Every endpoint has a unique `operationId` |
| `examples-present` | Public endpoints have request + response examples |
| `auth-documented` | Global security scheme or per-operation auth is defined |
| `field-naming-convention` | snake_case or camelCase used consistently (not mixed) |
| `nullable-discipline` | No ambiguous nulls; `required` lists are complete |
| `http-status-coverage` | 4xx and 5xx responses documented per operation |
| `deprecation-markers` | Removed endpoints carry `deprecated: true` before removal |

### Risk Score (0–100)
Aggregates breaking changes (weighted highest), potential breakage, standards violations, missing docs, and SDK sync status into a single score with letter grade.

```
Risk Score: 82/100 (HIGH)
Contributing factors:
  • 3 breaking changes        +45
  • 6 standards violations    +18
  • Missing examples on 4 endpoints  +12
  • SDK not regenerated       +7
```

---

## Web Dashboard

SpecGuard ships a dashboard for browsing spec history, change timelines, and artifacts across all your repos.

```bash
# Start backend (SQLite — no Postgres needed)
./specguard serve --port 8080

# Start frontend
cd web && npm install && npm run dev
# → http://localhost:3001
```

| Page | What you see |
|------|-------------|
| **Dashboard** | Health, stats cards, recent activity across all repos |
| **Repositories** | Add repos, trigger scans from local `.specguard/out/` directories |
| **Specs** | Upload spec files, browse normalized content, view metadata |
| **Changes** | Full change history with breaking/potential/non-breaking severity badges |
| **Artifacts** | Download reports, SDK zips, docs tarballs |
| **Settings** | System config reference, DB and storage health |

---

## Project Structure

```
specguard/
├── cmd/specguard/          # Cobra CLI + SQLite-backed API server
├── internal/
│   ├── scan/               # OpenAPI (kin-openapi) + Proto normalization
│   ├── diff/               # Semantic diff engine
│   ├── diff/classify.go    # Breaking / potential / non-breaking classification
│   ├── standards/          # 10 API standards rules
│   ├── risk/               # Risk score aggregation
│   ├── docconsistency/     # Spec ↔ documentation cross-check
│   ├── vectordb/           # SQLite vector store (cosine similarity)
│   ├── rag/                # RAG retrieval over doc chunks
│   └── protocol/           # REST vs gRPC recommender
├── web/                    # Next.js 14 + Tailwind + shadcn/ui dashboard
├── agent/                  # Python LangChain agent wrapping the CLI
└── docs/                   # Pipeline docs and execution plan
```

---

## Running Tests

```bash
go test ./internal/... -v -race
```

---

## Roadmap

- [x] `specguard init`, `scan`, `diff`, `report`, `serve`
- [x] 10 API standards rules + risk scoring
- [x] Web dashboard (Next.js)
- [x] GitHub webhook receiver (HMAC-SHA256 verified)
- [ ] `specguard sdk generate` / `specguard sdk package`
- [ ] `specguard docs generate`
- [ ] `specguard ci github` — PR comment bot + exit codes
- [ ] `specguard upload` — push artifacts to hosted control plane
- [ ] Hosted SaaS at `specguard.dev`

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). The short version: fork → branch → `go test ./internal/...` → PR. Never commit `.env` files.

## License

MIT — see [LICENSE](LICENSE).
