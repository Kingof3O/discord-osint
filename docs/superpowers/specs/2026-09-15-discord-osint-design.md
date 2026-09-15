# Discord-Only OSINT Search Workflow — Design Spec (v3.3, approved draft)

Date: 2026-09-15
Status: v3.3 revision (v3.2 channel gates kept; adds interactive manual CAPTCHA solver, TLS/anti-bot hardening, targeted member search, proxy support, and rich OSINT metadata)
Stack: Go 1.22+, single binary `discord-osint`

## 1. Goal

Given:
- `target`: Discord user ID (snowflake) and/or username (pomelo handle, e.g. `john.doe`)
- `category`: one Disboard tag/topic (e.g. `gaming`, `crypto`, `anime`) OR direct invite input (`--invite`, `--invites-file`)

Do:
1. Verify target identity first (preflight, read-only for `verify`; shared flow for `search`).
2. Discover Discord servers for that category from ONE source (Disboard) or direct invite list.
3. Walk servers one-by-one (20–50 per run).
4. Join (including screening/onboarding gates & interactive manual CAPTCHA) + search member list with typed match semantics.
5. On candidate/confirmed observation: collect evidence with member + message coverage metadata, guild roles, and `joined_at` timestamps.
6. Record `scan_status` vs `match_status` separately; `not_found` only on complete member coverage.
7. Return clear list of servers where user was found + evidence bundle with provenance and human-readable `report.md`.

Non-goals: global user search without joining, mass DM/scraping, multi-source discovery in MVP.

## 2. Key Decisions (v3.3 refined)

- **Source: Disboard + Direct Invites.** `disboard.org/servers/tag/{tag}?page=N` default; `--invite <code|url>` or `--invites-file <path>` supported for targeted investigations.
- **Auth & Anti-Bot: self user token (burner account).** ToS-violating, bannable. Burner only, 2FA, conservative limits, `--preview-only`.
  - **TLS Fingerprint & Client Properties**: Use browser-grade TLS client (`bogdanfinn/tls-client`) with synchronized `X-Super-Properties` and `X-Context-Properties` to prevent immediate JA3/JA4 fingerprint bans.
- **Proxy Support**: Native HTTP / SOCKS5 proxy support (`--proxy` or `config.yaml`) to isolate analyst IP.
- **Interactive Manual CAPTCHA Solver**: When Discord requires hCaptcha or Disboard requires Cloudflare Turnstile, the CLI raises an interactive prompt + local browser bridge for the operator to solve manually.
- **Two-Tier Member Search**:
  1. Targeted Search REST (`GET /guilds/{id}/members/search`) + Message Search probe (`GET /guilds/{id}/messages/search?author_id=...`).
  2. Gateway Opcode 14 (Lazy Guilds) / Chunking only for small servers (<1,000 members) or when exhaustive absence proof is requested.
- **Scale: small (20–50 servers/run).**
- **Identity: verified user ID is primary key when available.** Username-only matches are candidates, never confirmed.
- **Verify is read-only; search confirms then atomically creates the run.** No DB/file side effects before confirmation.
- **Channel verification gates handled explicitly.** Reaction checkmarks, buttons, links, and NSFW age-gates are detected, classified (LLM-assisted), and acted on under operator approval — never blindly auto-clicked. Buttons use complete interaction payloads (`session_id`, `application_id`, `custom_id`).
- **Onboarding/screening questions: manual-first, LLM-assisted drafts allowed.** Never auto-submit demographic answers without operator approval.
- **AI: opt-in OpenCode/OpenAI-compatible API, post-processing + onboarding draft helper only**, separated from raw evidence.

## 3. Architecture

```
cmd/discord-osint/      Cobra CLI (verify, search, resume, export, triage)
internal/config/        config.yaml + flags + env + proxy
internal/target/        normalize.go, resolver.go, verify.go (shared VerifyTarget flow)
internal/disboard/      tag scraper (tls-client, goquery), dedup, invite extract, cookie auth
internal/discord/       REST (tls-client) + gateway ws (session_id tracking, opcode 14 lazy sync)
internal/onboarding/    member-verification + onboarding prompts + channel gates (reaction/button/link/NSFW)
internal/matcher/       typed MatchKind matcher (ID exact, username variants, display/nick)
internal/messages/      fast search -> slow scan + message coverage tracking
internal/captcha/       Interactive Solver: local HTTP bridge + CLI prompt & wait
internal/ai/            OpenCode/OpenAI-compatible client (triage + onboarding drafts, cached)
internal/store/         SQLite (modernc.org/sqlite) checkpoint + export + targets view
internal/ratelimit/     token bucket + jitter + 429 backoff + token scrubber
```

CLI:
```bash
discord-osint verify --username john.doe
discord-osint search --username john.doe --tag gaming
discord-osint search --tag gaming --user-id 123456789012345678 --username john.doe --limit 30 --yes
discord-osint search --invites-file targets.txt --user-id 123456789012345678
discord-osint search --config config.yaml --proxy socks5://127.0.0.1:9050 --ai=triage
discord-osint resume --run-id <id>
discord-osint export --run-id <id> --format json,csv,md
```

`config.yaml`:
```yaml
discord_token: ${DISCORD_TOKEN}
proxy: ""                             # http://... or socks5://... (optional)
tag: gaming
limit: 30
join_delay_sec: [30, 90]
max_joins_per_hour: 8                 # Conservative to avoid join-velocity flags
stay_after_hit: false
captcha: manual                       # manual | skip
captcha_timeout_sec: 300              # 5-minute operator window
captcha_bridge_port: 8765             # Local browser helper port
onboarding: manual                    # manual | assist | rules-only | skip (see §11)
disboard_cookies: ""                  # Optional cf_clearance / session cookies
opencode_api_key: ${OPENCODE_API_KEY}
opencode_base_url: https://api.opencode.ai/v1
openai_api_key: ${OPENAI_API_KEY}     # optional fallback
ai_mode: triage
```

## 4. Target Verification / Preflight (v3.1 corrected)

### 4.1 Read-only guarantee (Fix 1)

`verify` is strictly read-only. It must never:
- create a run row,
- write any database row,
- write any search result,
- create any output file (no JSON/CSV/SQLite/log files; stdout/stderr only).

Implementation: `verify` command path does not open the store DB at all (resolver + renderer only). Unit tests assert no file creation in temp cwd.

### 4.2 Shared flow (Fix 2, Fix 3)

Both commands call the same function:

```go
// internal/target/verify.go
func VerifyTarget(ctx context.Context, in VerifyInput, ui PromptUI) (ConfirmedTarget, error)
```

Ordered steps (no deviations):
```text
resolve -> display -> select candidate if needed -> confirm -> create run+snapshot -> discovery
```

- `search` runs `VerifyTarget` first, with zero Disboard/network-guild side effects before confirmation.
- Only after the operator confirms (interactive `y` or `--yes`) does `search` atomically create the run: `INSERT INTO runs (...)` including the immutable target snapshot in a single transaction. If the insert fails, the run does not exist and discovery never starts.
- Rejection → exit 0, no run, no scrape, no join, no files (same as `verify` reject path).

### 4.3 Multiple candidates (Fix 4)

`multiple_candidates` requires explicit candidate selection before confirmation. No default, no first-pick.

```text
Multiple candidates found for "john.doe":

  [1] john.doe / John Doe / 111...111 (source: mutual-guild-cache)
  [2] john.doe / JD / 222...222 (source: mutual-guild-cache)

Select target [1-2] or 'q' to quit: _
Selected: [1] john.doe (111...111). Use this target? [y/N]: _
```

Selecting `q` or invalid input twice → abort with no side effects. The selected candidate becomes the confirmed snapshot; unselected candidates are discarded (not persisted).

### 4.4 Single-candidate and unresolved display

Resolved single candidate and unresolved cases render as:

```text
Target verification

Username:      john.doe
Display name:  John Doe
User ID:       123456789012345678
Created At:    2020-05-12T14:22:00Z (derived from Snowflake)
Avatar:        https://cdn.discordapp.com/avatars/...
Resolver:      users-api
Resolution:    resolved

Use this target? [y/N]
```

Unresolved adds an explicit warning (Fix 10):

```text
Resolution:    unresolved
WARNING: username could not be resolved to a user ID.
If you continue, subsequent matches can only ever be 'candidate', never 'confirmed'.

Use this username-only target? [y/N]
```

### 4.5 Both --user-id and --username (Fix 9)

When both flags are supplied:
1. Resolve the ID authoritatively (`GET /users/{id}` → canonical username/display/avatar).
2. Compare normalized resolved username vs normalized supplied username.
3. On mismatch, print a visible warning and require confirmation (even with `--yes`, the warning is printed; `--yes` still confirms but the mismatch is logged in the run row):

```text
WARNING: supplied username "john.doe" differs from resolved username for ID
123456789012345678 ("john.doe.42", source: users-api).
The run will key on user ID 123456789012345678 (confirmed path).
Supplied username will be kept as an alias note only.

Continue with ID-keyed target? [y/N]
```

Schema adds `target_username_alias_note TEXT` for this case. Matcher keys on the resolved ID; the supplied username is never used as an alternate confirmed key (it may generate additional `candidate` observations with reason `alias_username_match`, never `confirmed`).

Types (`internal/target/`, unchanged names):
```go
type ResolutionStatus string

const (
    Resolved           ResolutionStatus = "resolved"
    MultipleCandidates ResolutionStatus = "multiple_candidates"
    Unresolved         ResolutionStatus = "unresolved"
)

type Candidate struct {
    UserID      string
    Username    string
    DisplayName string
    AvatarURL   string
    Source      string
}

type Resolution struct {
    Input      string
    Status     ResolutionStatus
    Candidates []Candidate
}

type Resolver interface {
    ResolveUsername(ctx context.Context, username string) (Resolution, error)
}
```

Snapshot columns (immutable after creation):
```text
target_input
target_user_id            -- empty when username-only
target_username           -- confirmed canonical username (or supplied input when unresolved)
target_username_alias_note -- Fix 9 mismatch note, else NULL
target_display_name
target_avatar_url
target_resolution_source
target_verified_at        -- RFC3339
target_confirmed          -- always true in runs table (only confirmed targets get rows)
```

## 5. Match Semantics (typed) + Observations Table (v3.3 enriched)

```go
type MatchKind string

const (
    MatchNone      MatchKind = "none"
    MatchCandidate MatchKind = "candidate"
    MatchConfirmed MatchKind = "confirmed"
)
```

Rules:
- User ID exact (non-empty snapshot ID == member ID) → `confirmed`, reason `user_id_exact`.
- Username-only / ID-mismatch / nickname-similar → at most `candidate` with specific reason (`username_exact`, `username_case_fold`, `username_unicode_fold`, `username_match_id_mismatch`, `alias_username_match`, `nickname_similar`).
- Nickname/display similarity alone never confirms.

Storage (Fix 7, Fix 8 + v3.3 metadata): do not assume one match row per guild, and never key uniqueness on a possibly-empty user ID. Store rich investigator metadata (`joined_at`, `roles`, `avatar`).

```sql
CREATE TABLE match_observations (
  id INTEGER PRIMARY KEY,
  run_id TEXT NOT NULL,
  guild_id TEXT NOT NULL,
  observed_user_id TEXT NOT NULL DEFAULT '',   -- may be '' for unresolvable; never used alone as key
  observed_username_norm TEXT NOT NULL DEFAULT '',
  observed_username TEXT NOT NULL DEFAULT '',
  observed_nick TEXT NOT NULL DEFAULT '',
  observed_joined_at TEXT NOT NULL DEFAULT '',  -- ISO8601 when target joined server
  observed_roles TEXT NOT NULL DEFAULT '[]',    -- JSON array of role names/IDs
  observed_premium_since TEXT NOT NULL DEFAULT '', -- nitro boost timestamp if present
  observed_guild_avatar_url TEXT NOT NULL DEFAULT '',
  match_status TEXT NOT NULL,                  -- candidate | confirmed
  match_reason TEXT NOT NULL,
  observed_at TEXT NOT NULL,
  UNIQUE (run_id, guild_id, observed_user_id, observed_username_norm, observed_nick)
);
```

- Multiple candidate identities from one guild are all retained.
- `guild_scans` keeps the best-match summary only: `best_match_status` (`unknown|not_found|candidate|confirmed`), `best_match_reason`, `best_observed_user_id`, `best_observed_username`. Promotion rule: `confirmed` overwrites `candidate`; never downgrade.

## 6. Scan State vs Match State (v3.1 rule, Fix 6)

```text
scan_status:
    pending | running | complete | blocked | permission_limited | rate_limited | error

match_status (on guild_scans, mirrors best_match_status):
    unknown | not_found | candidate | confirmed
```

Validity rule: `match_status = not_found` is legal only when `member_coverage = complete` (§7). In every other case an absence finding must be `unknown` (with `member_stop_reason` explaining why). Linters/tests enforce: `CHECK (match_status != 'not_found' OR member_coverage = 'complete')`.

## 7. Coverage Metadata (v3.3: Two-Tier Member Search)

### 7.1 Member-search coverage (v3.3 refined)

To operate safely without triggering mass-scraping detections:

1. **Targeted Probe First**:
   - `GET /guilds/{id}/members/search?query={target}&limit=100` (Fast REST probe by prefix).
   - If User ID is known: probe `GET /guilds/{id}/messages/search?author_id={id}` (Global author search).
   - If target is found immediately via targeted search: record `candidate` or `confirmed` observation. Member coverage is marked `bounded` (`member_stop_reason=targeted_search_hit`).
2. **Exhaustive Scanning (Only when absence verification is required)**:
   - For small guilds (<1,000 members) or when explicitly requested via `--exhaustive-members`:
     Gateway Opcode 14 Lazy Guild ranges subscription or chunking is performed.
   - If full enumeration completes without hits: `member_coverage=complete`, `match_status=not_found`.
   - If large guild (>1,000 members) and targeted probe finds nothing: `member_coverage=bounded`, `match_status=unknown`, `member_stop_reason=large_guild_targeted_only`.

```text
member_coverage: complete | bounded | partial
members_examined      -- int, distinct member records evaluated
member_stop_reason:   -- exhaustive | targeted_search_hit | large_guild_targeted_only |
                         page_limit | gateway_timeout | permission_denied | rate_limited | aborted | gated
```

### 7.2 Message coverage

```text
message_coverage: complete | bounded | partial
oldest_checked_at / newest_checked_at
channels_discovered / channels_scanned / channels_unreadable
messages_examined
message_stop_reason  -- exhaustive | per-channel_page_limit | search_api_truncated | ...
```

## 8. Checkpointing and Idempotency

```text
UNIQUE(tag, invite_code)                                  -- servers
UNIQUE(run_id, guild_id)                                  -- guild_scans (summary row)
UNIQUE(run_id, guild_id, observed_user_id,
       observed_username_norm, observed_nick)             -- match_observations
UNIQUE(run_id, guild_id, channel_id, message_id)          -- messages
```

Rules:
- Per-guild transaction writes `guild_scans` summary + `match_observations` batch + message batch atomically.
- Crash + `resume` replays `scan_status IN (pending, running, rate_limited, error)`; terminal `complete/blocked` never re-scanned.
- `PRAGMA user_version` migrations handle schema upgrades seamlessly.

## 9. Provenance

Message rows:
```text
collected_at, collector_version, guild_id, channel_id, message_id,
acquisition_method (guild_search_api | channel_scan | gateway_event),
coverage_status (message_coverage snapshot at collect time)
```

Observation rows:
```text
collected_at, collector_version, guild_id,
acquisition_method (targeted_search | gateway_lazy_sync | gateway_chunk | rest_members),
observed_joined_at, observed_roles, observed_premium_since
```

## 10. Data Flow (v3.3)

```text
0. Pre-Flight Health Check (verify burner token alive, check phone-lock / quarantine)
   ↓
1. Resolve target (normalize input; resolve ID and/or username; read-only, zero side effects)
   ↓
2. Display target verification (render candidates + warning if unresolved or mismatched)
   ↓
3. Operator confirmation (interactive [y/N] or --yes)
   ↓
4. Atomically create run + snapshot in SQLite
   ↓
5. Discover servers (Disboard tag scrape via tls-client OR --invites-file)
   ↓
6. Resolve server invite (REST /invites/{code} with X-Context-Properties)
   ↓
7. Process server:
   a. Check if already member; if not, Join with jitter & X-Super-Properties.
   b. If CAPTCHA triggered -> invoke §13 Interactive Manual Solver (Prompt & Browser bridge).
   c. If Screening/Onboarding gated -> invoke §11 & §11.5 Gate Handlers.
   d. Member discovery: Phase 1 Targeted Search -> Phase 2 Message Probe -> Phase 3 Lazy sync if small.
   ↓
8. Match target:
   Record 0..N match_observations (with joined_at, roles) & update guild_scans summary.
   ↓
9. Evidence collection (on candidate/confirmed): fetch matched messages & surrounding context.
   ↓
10. Checkpoint transactionally (scan + match + coverage + provenance).
   ↓
11. Leave server (unless --stay=true); apply rate-limit jitter (30–90s); proceed to next.
   ↓
12. Export final artifacts: hits.json, observations.json, messages.csv, triage.md, report.md.
```

## 11. Membership Screening & Onboarding Prompts (§11)

### 11.1 Discovery of requirements (read-only first)
On join, fetch:
1. `GET /guilds/{id}/member-verification?with_guild=false`
2. `GET /guilds/{id}/onboarding`
3. `GET /guilds/{id}/widget` / preview

Store raw prompt JSON in checkpoint (`acquisition_method=onboarding_discovery`).

### 11.2 Answering policy
Modes (`--onboarding`):
- `manual` (default): Print prompts in terminal, operator inputs answers, explicit `submit` command.
- `assist`: LLM drafts suggestions (OpenCode / OpenAI) based solely on prompt text + server name (never reveals target identity). Operator reviews/edits draft before submission.
- `rules-only`: Automatically submit acceptance of static text rules, skip servers with complex interactive questionnaires.
- `skip`: Skip gated servers immediately (`scan=blocked, member_stop_reason=gated`).

## 11.5 Channel Verification Gates (Reaction, Button, Link, NSFW)

Candidate gate channels (`verify`, `verification`, `rules`, `welcome`, `nsfw-access`):
- Stage A: Deterministic inspection (buttons, emoji checkmarks, external links, NSFW flags).
- Stage B: LLM helper (`assist` mode) classifies instructions: `ClassifyGateInstruction`.

Action Execution:
- `reaction`: Safe to auto-propose or auto-react in `assist` mode if confidence is high.
- `button`: Execute `POST /interactions` with complete payload:
  ```json
  {
    "type": 3,
    "guild_id": "<guild_id>",
    "channel_id": "<channel_id>",
    "message_id": "<message_id>",
    "application_id": "<bot_author_id>",
    "session_id": "<gateway_session_id>",
    "data": { "component_type": 2, "custom_id": "<custom_id>" }
  }
  ```
  Requires operator `approve` command before clicking.
- `link_external` (AltDentifier, Wick, Double Counter): Print URL in CLI. Operator reviews in browser and confirms completion with `done` or `skip`.
- `nsfw_consent`: Prompt operator to approve age-gate consent (`approve` / `skip`).

## 12. Reuse Design

- `(tag, invite_code)` server cache; `--reuse-servers` skips scraping.
- Resume continues from first non-terminal `scan_status`.
- CLI and presentation remain separated from core logic (`internal/*` CLI-free).

## 13. Interactive Manual CAPTCHA Solver (v3.3 detailed)

When Discord responds with a CAPTCHA challenge during join or interaction (`400/403` with `captcha_key`, `captcha_sitekey`, `captcha_service="hcaptcha"`, `captcha_rqdata`, `captcha_rqtoken`):

### 13.1 Solver Flow
1. **Detection**: Capture all challenge parameters: `sitekey`, `rqdata`, `rqtoken`, `service`.
2. **Local HTTP Bridge**: Spin up a transient local server at `http://127.0.0.1:8765/captcha` serving an embedded HTML page loaded with the official hCaptcha widget initialized with the given `sitekey` and `rqdata`.
3. **Interactive Alert**:
   - Sound terminal bell (`\a`) and display a highlighted CLI notification:
   ```text
   ======================================================================
   [!] CAPTCHA CHALLENGE TRIGGERED for Guild: "Crypto Alpha" (123456789)
       Service: hCaptcha
       Sitekey: a9b5fb07-92ff-493f-86fe-352a2843b3df
       Action:  Solve in browser at http://127.0.0.1:8765/captcha
   ======================================================================
   The bridge has opened in your default browser.
   Waiting for completion (timeout: 300s)...
   Or paste token directly / type 's' to skip / 'q' to abort: _
   ```
4. **Completion**:
   - The browser bridge receives the solved token via JavaScript post-back to `/submit` on localhost and signals the CLI.
   - Alternatively, the operator can paste the raw token directly into the terminal prompt.
5. **Retry**:
   - Re-submit the join request with `X-Captcha-Key: <token>` and `X-Captcha-Rqtoken: <rqtoken>`.
6. **Timeout / Skip**:
   - If timeout expires or operator inputs `s`: set `scan=blocked, match=unknown, member_stop_reason=captcha_unsolved`. Proceed to next server without killing the run.
   - If operator inputs `q`: checkpoint progress and cleanly terminate.

## 14. AI Integration

- Opt-in via `--ai=triage` or `--onboarding=assist`.
- Client: `internal/ai` supports OpenCode / OpenAI-compatible API.
- Caching: Responses cached by hash of prompt + context to minimize cost and token latency.
- Isolation: Target identity, user tokens, and raw member lists are never passed to AI endpoints.

## 15. Rate Limits, Safety & OpSec

- **TLS Fingerprinting**: All outgoing HTTP requests handled by `tls-client` configured with Chrome 120+ / Discord Desktop JA3/JA4 profiles and headers (`User-Agent`, `Accept`, `Accept-Language`, `Sec-Ch-Ua`, `X-Super-Properties`).
- **Proxy Routing**: Full proxy support for REST and Gateway connections.
- **Sensitive Token Scrubbing**: Logging middleware unconditionally masks the `Authorization` token (e.g. `MTAx... -> [REDACTED]`) from zap logs, SQLite databases, and stdout.
- **Rate Limit Buckets**:
  - Global Discord REST: 5 rps / 8 burst
  - Guild Joins: 8–10 joins/hour with 30–90s randomized jitter
  - Onboarding Submissions: Max 1/60s per guild
  - Gate Reactions: 1/5s per guild
  - Disboard Scraper: 1 req / 3–5s

## 16. Testing Order

### Test 1 — Target verification (FIRST MILESTONE, Fix 10)
```bash
discord-osint verify --username <username>
```
Displays: username, display name, user ID, avatar URL, creation date derived from Snowflake, resolver source, resolution status. Unresolved shows candidate-only warning. Multiple candidates prompt selection. Assert: zero files or database entries created.

### Test 2 — Matcher + Observations + Rich Metadata
Assert ID exact -> confirmed; username variants -> candidate; capture of `joined_at` and `roles`. Empty-ID rows distinct by username+nick.

### Test 3 — SQLite / State Machine & Checkpointing
Atomicity of run creation; immutable target snapshot; idempotency on resume; promotion-only match logic; CHECK constraint enforcement.

### Test 4 — Discovery Parser & Invites Input
Disboard HTML fixture parser; `--invite` and `--invites-file` parser.

### Test 5 — Discord Adapter, Captcha Bridge & Gate Handler
Fake HTTP/gateway server. Test interactive manual CAPTCHA bridge; test reaction/button/link gate execution; test complete interaction payload format.

### Test 6 — Owned-Server Canary
Live test against controlled Discord servers: verify -> confirm -> join -> gate approval -> match -> evidence -> leave -> report export.

## 17. Outputs

- Terminal: Live progress table showing server, scan status, match status, member coverage, and message count.
- `hits.json`: Per-guild summary with match status, reason, member/message coverage, and stop reasons.
- `observations.json`: All observed member identities per guild with `observed_joined_at`, `observed_roles`, `observed_nick`.
- `messages.csv`: Collected messages with channel IDs, message IDs, timestamps, and acquisition method.
- `triage.md`: AI-generated triage insights (opt-in).
- `report.md`: Executive investigation report formatted in Markdown with target overview, server findings table, and key evidence.
- `run.sqlite`: Complete SQLite database for full resume capability and deep relational queries.

## 18. Risks & Ethics Notice

Self-bots violate Discord Terms of Service and risk account termination. Always use dedicated burner accounts. Screening answers are attributable to the account: use `manual` or `assist` with operator review.

---
v3.3 applies TLS/anti-bot hardening, interactive manual CAPTCHA solver bridge, two-tier member discovery, proxy support, and rich OSINT metadata.
Ready for `writing-plans` (beginning with Test 1 milestone).
