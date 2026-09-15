# Discord-Only OSINT Search Workflow Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a production-grade, single-binary Go CLI tool (`discord-osint`) for Discord-based OSINT investigations, discovering public target presence across servers with strict identity separation, interactive manual CAPTCHA solving, anti-bot TLS hardening, two-tier member search, and audit-ready reporting.

**Architecture:** A modular Go 1.22+ application with Cobra CLI entry points, pure-Go SQLite persistence (`modernc.org/sqlite`), a hardened Discord REST/Gateway adapter (handling `X-Super-Properties`, `X-Context-Properties`, and session tracking), an interactive local CAPTCHA bridge, Disboard scraper with direct-invite fallbacks, two-tier member discovery (targeted search first, lazy sync second), and comprehensive audit exporters.

**Tech Stack:** Go 1.22+, `github.com/spf13/cobra`, `modernc.org/sqlite`, `github.com/PuerkitoBio/goquery`, `github.com/gorilla/websocket`, `go.uber.org/zap`, `gopkg.in/yaml.v3`.

**Spec:** `docs/superpowers/specs/2026-09-15-discord-osint-design.md`

## Global Constraints
- **Runtime & Build:** Go 1.22+, single binary `discord-osint`. No CGO (`CGO_ENABLED=0` compatible via `modernc.org/sqlite`).
- **Read-Only Guarantee:** `verify` command MUST NEVER create any files, database rows, or state changes (stdout/stderr only).
- **Atomicity:** Runs must be created atomically with an immutable target snapshot only after operator confirmation.
- **Identity Distinction:** Snowflake user ID is the only confirmed key. Username matches are strictly `candidate`, never `confirmed`.
- **Match State vs Scan State:** `match_status = not_found` is strictly prohibited unless `member_coverage = complete`.
- **Manual Gate Control:** No button clicks, demographic form submissions, or external links executed without operator confirmation.
- **Manual CAPTCHA Bridge:** Interactive prompt + browser bridge for manual hCaptcha / Cloudflare Turnstile completion.
- **Anti-Bot Hardening:** User-Agent, `X-Super-Properties`, and `X-Context-Properties` injection; token scrubbing in all logs.

---

### Task 1: Scaffolding, Configuration, and Target Verification Preflight (Test 1 Milestone)
**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `internal/target/types.go`
- Create: `internal/target/normalize.go`
- Create: `internal/target/resolver.go`
- Create: `internal/target/verify.go`
- Create: `internal/target/verify_test.go`
- Create: `cmd/discord-osint/root.go`
- Create: `cmd/discord-osint/verify.go`
- Create: `cmd/discord-osint/main.go`

**Interfaces:**
- Produces: `config.Config`, `config.Load(path string) (*Config, error)`
- Produces: `target.VerifyTarget(ctx context.Context, in VerifyInput, ui PromptUI) (ConfirmedTarget, error)`
- Produces: CLI command `discord-osint verify --username <user> [--user-id <id>]`

- [ ] **Step 1.1: Write failing tests for Config loader**
  Test YAML loading, environment variable expansion (`${DISCORD_TOKEN}`), default values (`max_joins_per_hour: 8`, `captcha: manual`), and validation.
- [ ] **Step 1.2: Implement `internal/config/config.go`**
  Implement struct and `Load` function supporting YAML unmarshaling and env substitution. Verify config tests pass.
- [ ] **Step 1.3: Write failing unit tests for `internal/target`**
  Test `NormalizeUsername`, snowflake validation, `VerifyTarget` with mock resolver:
  - Path 1: Resolved single candidate -> prompt confirmation.
  - Path 2: Multiple candidates -> selection prompt -> confirm.
  - Path 3: Unresolved -> candidate-only warning -> confirm.
  - Path 4: User ID + username mismatch -> warning -> confirm ID-keyed.
  - Assert: Zero file/DB side effects created.
- [ ] **Step 1.4: Implement `internal/target` modules**
  Implement normalization, snowflake timestamp extraction, resolver interface (with mock and Discord REST user lookup), and terminal UI prompt renderer. Verify all target tests pass.
- [ ] **Step 1.5: Wire Cobra CLI commands (`root.go`, `verify.go`, `main.go`)**
  Connect `verify` subcommand with flags (`--username`, `--user-id`, `--config`, `--yes`). Test running `go run ./cmd/discord-osint verify --help`.
- [ ] **Step 1.6: Commit Milestone 1**
  `git commit -m "feat: implement target verification preflight with read-only guarantee"`

---

### Task 2: Typed Matcher & Observation Models (Test 2 Milestone)
**Files:**
- Create: `internal/matcher/types.go`
- Create: `internal/matcher/matcher.go`
- Create: `internal/matcher/matcher_test.go`

**Interfaces:**
- Consumes: `target.ConfirmedTarget`
- Produces: `matcher.MatchTarget(target ConfirmedTarget, member GuildMember) MatchObservation`
- Produces: `matcher.MatchKind` (`none`, `candidate`, `confirmed`) and specific reason codes.

- [ ] **Step 2.1: Write failing unit tests for Matcher**
  - Test exact snowflake ID match -> `confirmed` (`user_id_exact`).
  - Test exact username match without ID -> `candidate` (`username_exact`).
  - Test case-folded / Unicode-folded username match -> `candidate` (`username_case_fold`, `username_unicode_fold`).
  - Test username match with different ID -> `candidate` (`username_match_id_mismatch`).
  - Test nickname similarity -> `candidate` (`nickname_similar`).
  - Test extraction of `joined_at`, `roles`, `premium_since`, `guild_avatar_url`.
- [ ] **Step 2.2: Implement `internal/matcher`**
  Implement Unicode normalization, Levenshtein/similarity calculation for display names, and typed reason assignment.
- [ ] **Step 2.3: Run tests and verify**
  Run `go test -v ./internal/matcher` and verify all match rules strictly adhere to §5 of spec.
- [ ] **Step 2.4: Commit Milestone 2**
  `git commit -m "feat: implement typed identity matcher with rich metadata capture"`

---

### Task 3: SQLite Persistence, Migrations & State Machine (Test 3 Milestone)
**Files:**
- Create: `internal/store/schema.go`
- Create: `internal/store/store.go`
- Create: `internal/store/store_test.go`
- Create: `internal/store/models.go`

**Interfaces:**
- Produces: `store.New(dbPath string) (*Store, error)`
- Produces: `store.CreateRunAtomic(ctx context.Context, target ConfirmedTarget, options RunOptions) (string, error)`
- Produces: `store.RecordGuildScan(ctx context.Context, scan GuildScanRecord, obs []MatchObservation, msgs []MessageRecord) error`
- Produces: Checkpoint resume queries and export queries.

- [ ] **Step 3.1: Write failing tests for SQLite Store**
  - Test schema creation via `PRAGMA user_version`.
  - Test run creation atomicity: target snapshot is immutable.
  - Test CHECK constraint: `CHECK (match_status != 'not_found' OR member_coverage = 'complete')`.
  - Test observations unique constraint: multiple observations per guild allowed, empty ID distinct by username+nick.
  - Test promotion-only logic for `guild_scans`: candidate -> confirmed allowed; confirmed -> candidate prohibited.
  - Test resume query returning only non-terminal scans.
- [ ] **Step 3.2: Implement `internal/store` with `modernc.org/sqlite`**
  Implement transactions, prepared statements, and data access methods.
- [ ] **Step 3.3: Verify store tests pass**
  Run `go test -v ./internal/store`.
- [ ] **Step 3.4: Commit Milestone 3**
  `git commit -m "feat: implement sqlite store with atomic checkpoints and integrity constraints"`

---

### Task 4: Interactive Manual CAPTCHA Solver Bridge (Test 5 / §13 Milestone)
**Files:**
- Create: `internal/captcha/solver.go`
- Create: `internal/captcha/bridge.go`
- Create: `internal/captcha/bridge_test.go`

**Interfaces:**
- Produces: `captcha.Solver interface { Solve(ctx context.Context, challenge Challenge) (Solution, error) }`
- Produces: Embedded HTTP server at `http://127.0.0.1:8765/captcha` with hCaptcha web widget and submission callback.

- [ ] **Step 4.1: Write failing tests for CAPTCHA Bridge**
  - Test bridge HTTP server startup, static HTML widget rendering with `sitekey` and `rqdata`.
  - Test token submission endpoint `/submit` notifying solver channel.
  - Test CLI manual input fallback (`token`, `s` for skip, `q` for quit).
  - Test 300s timeout handling -> return `ErrCaptchaTimeout`.
- [ ] **Step 4.2: Implement `internal/captcha`**
  Implement local HTTP server, embedded HTML template, terminal bell / banner, and input listener.
- [ ] **Step 4.3: Verify tests pass**
  Run `go test -v ./internal/captcha`.
- [ ] **Step 4.4: Commit Milestone 4**
  `git commit -m "feat: implement interactive manual captcha solver bridge"`

---

### Task 5: Disboard Discovery Scraper & Direct Invites Provider
**Files:**
- Create: `internal/disboard/scraper.go`
- Create: `internal/disboard/scraper_test.go`
- Create: `internal/disboard/invites.go`

**Interfaces:**
- Produces: `disboard.Discover(ctx context.Context, tag string, limit int, cookies string) ([]DiscoveredServer, error)`
- Produces: `disboard.LoadInvites(pathsOrCodes []string) ([]DiscoveredServer, error)`

- [ ] **Step 5.1: Write failing tests with Disboard HTML fixtures**
  - Test parsing server cards, title, member counts, invite codes (`/join/...` or `discord.gg/...`).
  - Test pagination deduplication.
  - Test direct invite file parser (`invites.txt` with raw codes or full URLs).
- [ ] **Step 5.2: Implement `internal/disboard`**
  Implement `goquery` parser with rate limiting (1 req / 3s) and cookie header injection.
- [ ] **Step 5.3: Verify tests pass**
  Run `go test -v ./internal/disboard`.
- [ ] **Step 5.4: Commit Milestone 5**
  `git commit -m "feat: implement disboard scraper and direct invite loader"`

---

### Task 6: Hardened Discord REST & Gateway Adapter with Two-Tier Member Discovery
**Files:**
- Create: `internal/discord/client.go`
- Create: `internal/discord/properties.go`
- Create: `internal/discord/gateway.go`
- Create: `internal/discord/members.go`
- Create: `internal/discord/messages.go`
- Create: `internal/discord/discord_test.go`

**Interfaces:**
- Produces: `discord.Client` with `JoinGuild`, `SearchMembers`, `SearchMessages`, `FetchChannelMessages`, `LeaveGuild`.
- Produces: Gateway client maintaining `session_id`, handling Opcode 14 lazy sync.
- Produces: `internal/ratelimit` token scrubber and bucket limiter.

- [ ] **Step 6.1: Write failing unit tests with mock Discord HTTP/WS server**
  - Test `X-Super-Properties` generation and header injection.
  - Test targeted member search `GET /guilds/{id}/members/search?query=...`.
  - Test message search `GET /guilds/{id}/messages/search?author_id=...`.
  - Test Gateway `READY` event capturing `session_id`.
  - Test 429 backoff and token redaction in logs.
- [ ] **Step 6.2: Implement Discord client and Gateway adapter**
  Implement HTTP client with configurable proxy, custom headers, and Gateway WebSocket client.
- [ ] **Step 6.3: Implement Two-Tier Member Search**
  Phase 1 targeted search + Phase 2 message search + Phase 3 lazy sync for small guilds.
- [ ] **Step 6.4: Verify tests pass**
  Run `go test -v ./internal/discord`.
- [ ] **Step 6.5: Commit Milestone 6**
  `git commit -m "feat: implement discord rest and gateway adapter with anti-bot hardening"`

---

### Task 7: Onboarding & Channel Verification Gate Adapter (§11 & §11.5)
**Files:**
- Create: `internal/onboarding/types.go`
- Create: `internal/onboarding/screening.go`
- Create: `internal/onboarding/channel_gates.go`
- Create: `internal/onboarding/gates_test.go`

**Interfaces:**
- Produces: `onboarding.Handler` handling member verification, onboarding prompts, reaction checkmarks, button interactions (with `session_id`), link detection, and NSFW consent.

- [ ] **Step 7.1: Write failing tests for Gate Handlers**
  - Test rules screening acceptance.
  - Test onboarding prompt extraction and manual answering mode.
  - Test channel gate Stage A deterministic detection (reaction ✅, button components, external links).
  - Test button interaction payload construction (`application_id`, `session_id`, `custom_id`, `type: 3`).
- [ ] **Step 7.2: Implement `internal/onboarding`**
  Implement gate discovery and execution with strict operator approval checks.
- [ ] **Step 7.3: Verify tests pass**
  Run `go test -v ./internal/onboarding`.
- [ ] **Step 7.4: Commit Milestone 7**
  `git commit -m "feat: implement membership screening and channel verification gates"`

---

### Task 8: Search Workflow Orchestrator, CLI Integration & Exporters
**Files:**
- Create: `internal/workflow/engine.go`
- Create: `internal/export/exporter.go`
- Create: `internal/export/report.go`
- Create: `internal/export/export_test.go`
- Create: `cmd/discord-osint/search.go`
- Create: `cmd/discord-osint/resume.go`
- Create: `cmd/discord-osint/export.go`

**Interfaces:**
- Produces: Full workflow loop (`verify -> confirm -> discover -> join -> gate -> search -> evidence -> checkpoint -> leave -> next`).
- Produces: Exporter for `hits.json`, `observations.json`, `messages.csv`, and executive `report.md`.
- Produces: CLI commands `search`, `resume`, `export`.

- [ ] **Step 8.1: Write tests for Exporter**
  Verify `hits.json`, `observations.json`, `messages.csv`, and markdown `report.md` generation.
- [ ] **Step 8.2: Implement Orchestrator engine**
  Implement the main search loop connecting all subsystems with checkpointing and error handling.
- [ ] **Step 8.3: Implement CLI commands `search`, `resume`, and `export`**
  Wire all flags, signals (graceful Ctrl+C shutdown), and terminal tables.
- [ ] **Step 8.4: Verify tests pass**
  Run `go test -v ./internal/workflow ./internal/export`.
- [ ] **Step 8.5: Commit Milestone 8**
  `git commit -m "feat: implement search engine orchestrator, export pipeline and cli commands"`

---

### Task 9: Optional AI Triage & Onboarding Draft Client (§14)
**Files:**
- Create: `internal/ai/client.go`
- Create: `internal/ai/client_test.go`
- Create: `cmd/discord-osint/triage.go`

**Interfaces:**
- Produces: `ai.Client` supporting OpenCode / OpenAI-compatible chat completion endpoints.
- Produces: `DraftOnboardingAnswers` and `TriageEvidence`.

- [ ] **Step 9.1: Write unit tests with mock OpenAI/OpenCode HTTP server**
  Test answer drafting with prompt caching; test evidence triage generating `triage.md`.
- [ ] **Step 9.2: Implement `internal/ai`**
  Implement HTTP client with cache by input hash and token sanitization.
- [ ] **Step 9.3: Connect `--ai=triage` in CLI**
- [ ] **Step 9.4: Commit Milestone 9**
  `git commit -m "feat: implement opt-in ai triage and onboarding drafting client"`

---

### Task 10: Production Hardening, Race Condition Auditing & Verification
- [ ] **Step 10.1: Run race detector on all packages**
  Run `go test -race -v ./...`. Fix any data races.
- [ ] **Step 10.2: Run static analysis & vet**
  Run `go vet ./...` and verify clean compilation.
- [ ] **Step 10.3: Test full end-to-end CLI workflow**
  Build binary `go build -o bin/discord-osint ./cmd/discord-osint`.
  Run test suites across all mock servers and assert all fixtures and CLI exit codes.
- [ ] **Step 10.4: Commit Milestone 10**
  `git commit -m "chore: production hardening, race audit, and binary build verification"`
