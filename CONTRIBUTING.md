# Contributing to SpecGuard

Thank you for your interest in contributing! This document explains the workflow and standards for contributions.

## Getting Started

1. **Fork** the repository and clone your fork locally.
2. Create a feature or fix branch:
   ```bash
   git checkout -b feature/my-feature
   # or
   git checkout -b fix/issue-123
   ```
3. Make your changes, following the guidelines below.
4. Push and open a **Pull Request** against `main`.

## Development Setup

### Prerequisites
- **Go ≥ 1.22** — for the CLI and API server
- **Node.js ≥ 18** — for the web dashboard
- **SQLite** — bundled via Go driver, no install needed for local dev

### Run the backend
```bash
go build -o specguard ./cmd/specguard/
./specguard serve --port 8080
```

### Run the frontend
```bash
cd web
npm install
npm run dev
# → http://localhost:3001
```

### Run tests
```bash
go test ./internal/... -v
```

## Code Style

- **Go**: run `go vet ./...` and `gofmt -w .` before committing.
- **TypeScript/React**: the project uses ESLint; run `npm run lint` in `web/`.
- Keep functions small and well-named. Prefer clarity over brevity.

## Security

- **Never** commit `.env` files, API keys, tokens, passwords, or private keys.
- Use `.env.example` as a template — add new variables there with placeholder values only.
- If you accidentally commit a secret, rotate it immediately and open an issue.

## Commit Messages

Use the conventional commits format:

```
feat: add proto diff support
fix: handle empty openapi spec gracefully
docs: update CLI reference for v1 commands
chore: upgrade kin-openapi to v0.127
```

## Pull Request Checklist

- [ ] Tests pass (`go test ./internal/...`)
- [ ] No new lint warnings
- [ ] `.env.example` updated if new environment variables were added
- [ ] README updated if new commands or features were added
- [ ] No hardcoded secrets, tokens, or passwords

## Reporting Bugs

Open a [GitHub Issue](../../issues/new) with:
- A clear description of the problem
- Steps to reproduce
- Expected vs actual behavior
- Go/Node versions and OS

## Feature Requests

Open a [GitHub Discussion](../../discussions/new) before starting large changes, so we can align on design first.

## License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).
