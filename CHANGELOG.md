# Changelog

All notable changes are documented here. Format follows [Keep a Changelog](https://keepachangelog.com/).

## [Unreleased]

### Added
- **Enum value detection**: Removed enum values now reported as `breaking`, added values as `non_breaking`
- **Request body diffing**: Changes to requestBody schema, required flag, and content types detected
- **Inline response schema diffing**: Property changes in response schemas (not just component schemas)
- **Parameter detail diffs**: Location change (path→query), type change, and required change all detected
- **Proto field number reuse detection**: The most critical gRPC breaking change — field number reassignment
- **Proto field removal and type change detection**
- **STD-011 Enum documentation**: Flags enum properties missing descriptions
- **STD-012 Sunset header**: Flags deprecated endpoints not advertising a `Sunset` response header (RFC 8594)
- **STD-013 Request body content type**: Flags request bodies with no content type specified
- **Real health check**: `/health` endpoint now pings the SQLite DB and reports `degraded` if unavailable
- **`changeKindLabel()` helper**: Human-readable labels for all 25+ change kinds in the web UI
- **`severityIcon()` helper**: Emoji icons for severity levels
- `potential_breaking` severity now correctly coloured orange in the web UI

### Fixed
- **Risk score double-counting**: `removals` field was double-counted (already included in `breaking`); corrected formula
- **STD-002 false positives**: Pagination consistency rule no longer fires for APIs with fewer than 3 list endpoints
- **`parameter.became_required`**: Was silently dropped; now classified as `breaking` correctly
- **Health endpoint**: Was returning hardcoded `"healthy"` regardless of DB state

### Changed
- Risk scoring weights rebalanced: `breaking*30`, `potential_breaking*12`, `mutations*8`, `docs*3`, `standards*2`
- 10 standards rules → 13 standards rules (STD-011, STD-012, STD-013 added)

## [1.0.0] — Initial Release

- `specguard init`, `scan`, `diff`, `report`, `serve`
- 10 API standards rules
- Risk scoring (0–100)
- Web dashboard (Next.js 14)
- Python LangChain agent
