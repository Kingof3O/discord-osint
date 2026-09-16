# Contributing Guidelines

Thank you for your interest in contributing to **Discord OSINT**. To ensure code quality, operational safety, and forensic integrity, please follow these guidelines.

---

## Development Principles

1. **Defensive Programming & OpSec First**:
   - Every network endpoint or logger that touches credentials must integrate with `internal/ratelimit.TokenScrubber`.
   - Never log authorization headers, passwords, or session tokens.
   - Never commit `.env` or real operational data to git.
2. **Deterministic & Verifiable Evidence**:
   - All observations, member lookups, and message records must maintain provenance metadata (`run_id`, `collected_at`, `collector_version`, `acquisition_method`).
   - Match states must strictly adhere to the typed hierarchy: `confirmed` (`user_id_exact`, `message_author_id_exact`) vs `candidate` (`username_exact`, `nickname_similar`).
3. **Pure-Go Portability**:
   - Maintain pure-Go SQLite (`modernc.org/sqlite`). Do not introduce CGO dependencies that complicate cross-compilation.

---

## Verification & Quality Standards

Before submitting a pull request or pushing commits, verify the following checks pass locally:

### 1. Run All Unit Tests
```bash
go test -v ./...
```

### 2. Run Concurrency & Race Detector
```bash
go test -race -count=1 ./...
```

### 3. Static Analysis
```bash
go vet ./...
```

### 4. Static Binary Build
```bash
CGO_ENABLED=0 go build -o bin/discord-osint ./cmd/discord-osint
```

---

## Pull Request Workflow

1. Create a descriptive feature branch: `git checkout -b feat/your-feature-name`.
2. Ensure commit messages follow Conventional Commits (e.g. `feat:`, `fix:`, `docs:`, `test:`).
3. Ensure no personal target IDs, burner tokens, or server names are hardcoded in test fixtures or documentation.
4. Open a Pull Request against `main`.
