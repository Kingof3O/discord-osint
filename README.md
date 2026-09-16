# Discord OSINT Investigation Platform

[![Go Version](https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Database](https://img.shields.io/badge/Storage-Pure--Go_SQLite-003B57?logo=sqlite)](https://modernc.org/sqlite)
[![Interface](https://img.shields.io/badge/UI-Glassmorphism_HTML5-5865F2?logo=discord)](https://discord.com)

A hardened, production-grade Discord OSINT (Open Source Intelligence) reconnaissance and forensic investigation platform written in Go. Built for cyber threat intelligence (CTI) analysts, incident responders, fraud investigators, and authorized operators to deterministically verify target presence, capture real CDN user avatars, map guild role structures, and harvest verifiable message evidence across public and gated Discord servers.

---

## Table of Contents

1. [Key Capabilities](#key-capabilities)
2. [System Architecture](#system-architecture)
3. [End-to-End Investigation Lifecycle](#end-to-end-investigation-lifecycle)
4. [Two-Tier Member & Message Discovery](#two-tier-member--message-discovery)
5. [Interactive CAPTCHA Bridge Architecture](#interactive-captcha-bridge-architecture)
6. [Onboarding & Channel Gate State Machine](#onboarding--channel-gate-state-machine)
7. [Database Schema & Provenance Model](#database-schema--provenance-model)
8. [Installation & Build](#installation--build)
9. [Configuration](#configuration)
10. [CLI Command Reference](#cli-command-reference)
11. [Forensic Artifacts & Glassmorphic Reporting](#forensic-artifacts--glassmorphic-reporting)
12. [Operational Security & Stealth Best Practices](#operational-security--stealth-best-practices)
13. [Troubleshooting & Error Matrix](#troubleshooting--error-matrix)

---

## Key Capabilities

- **Deterministic Snowflake Math & Target Preflight**:
  - Extracts millisecond-precision UTC account creation timestamp and Discord worker/process IDs from 64-bit Snowflake User IDs using bitwise arithmetic.
  - Zero-touch, read-only preflight verification validating username and snowflake consistency prior to creating database entries or network walks.
- **Dynamic Real Avatar CDN Resolution**:
  - Performs direct guild member queries upon joining to obtain authoritative Discord user profiles, global display names, and high-resolution avatar CDN hashes (`https://cdn.discordapp.com/avatars/{id}/{hash}.png?size=256`).
- **Human-Readable Channel & Server Mapping**:
  - Automatically queries Discord's channel hierarchy (`GetGuildChannels`) to resolve raw Snowflake IDs into real channel names (e.g. `#🌸・chat`, `#request-channel`, `#・ᰔ┊dm-and-fr-requests`) and pairs them with official server titles across all HTML, CSV, and Markdown exports.
- **Two-Tier Member Discovery & Full Author Probe**:
  - Combines REST targeted member queries (`/guilds/{id}/members/search`) with guild-wide author message search (`/guilds/{id}/messages/search?include_nsfw=true`) to discover users even in silent, unindexed, or massive servers.
- **Interactive CAPTCHA & Cloudflare Bridge**:
  - Detects hCaptcha / Cloudflare Turnstile challenges on guild join requests, spinning up an embedded local HTTP server (`http://127.0.0.1:8765/captcha`) and automatically orchestrating browser-based human-in-the-loop solving with session binding.
- **Onboarding Gate & Screening Automation**:
  - Automatically acknowledges Discord Community membership screening rules and supports button interaction (`type: 3`) and emoji reaction (`PUT /reactions/@me`) bypasses.
- **Discord-Styled Glassmorphic Reporting**:
  - Generates self-contained, standalone offline HTML investigation reports featuring Discord dark styling, frosted glass panels (`backdrop-filter: blur(20px)`), user profile hero cards, glowing KPI statistics, and authentic chat streams.

---

## System Architecture

The platform is designed around modular, decoupled components with strict identity separation between the investigator's burner session and the target footprint.

```mermaid
flowchart TB
    subgraph CLI["Operator Interface & Execution Layer"]
        CMD["CLI Entrypoint<br/>(Cobra Framework)"]
        CONF["Configuration Loader<br/>(.env / config.yaml)"]
        DASH["Embedded Web UI<br/>(discord-osint ui :3000)"]
    end

    subgraph PREFLIGHT["Target Verification Preflight"]
        MATH["Snowflake Math Engine<br/>(Timestamp, Worker, Epoch)"]
        VAL["Read-Only Validator<br/>(Discord REST Check)"]
    end

    subgraph ENGINE["Workflow Orchestrator"]
        WALK["Server Walk Engine<br/>(Queue, Limit, Stay)"]
        JITTER["Rate Limiter & Jitter<br/>(Exponential Backoff / 429)"]
        STATE["State Checkpointer<br/>(Non-Terminal Resume)"]
    end

    subgraph DISCOVERY["Multi-Vector Discovery"]
        DISBOARD["Disboard Scraper<br/>(Category Tags & Search)"]
        INVITES["Direct Invites Provider<br/>(Links, Codes, invites.txt)"]
    end

    subgraph ADAPTERS["Discord & Stealth Layer"]
        PROPS["Super-Properties Spoofing<br/>(Desktop Browser Fingerprint)"]
        REST["Discord REST Adapter<br/>(Token Scrubber Redaction)"]
        GW["Gateway Client<br/>(Opcode 14 Lazy Sync)"]
        GATES["Onboarding & Gate Engine<br/>(Rules, Buttons, Reactions)"]
        CAPTCHA["CAPTCHA Bridge Server<br/>(127.0.0.1:8765)"]
    end

    subgraph STORE["Persistence & Audit Storage"]
        SQLITE[("Pure-Go SQLite Database<br/>(WAL Mode, Schema v2)")]
    end

    subgraph EXPORT["Evidence Exporters"]
        HTML["Glassmorphic HTML Report<br/>(report.html)"]
        CSV["RFC4180 Messages CSV<br/>(messages.csv)"]
        MD["Executive Markdown<br/>(report.md)"]
        JSON["Machine Forensic Hits<br/>(hits.json / observations.json)"]
    end

    CMD --> CONF
    CONF --> ENGINE
    CMD --> PREFLIGHT
    PREFLIGHT --> MATH
    PREFLIGHT --> VAL
    VAL --> WALK

    DISCOVERY --> WALK
    WALK --> JITTER
    JITTER --> ADAPTERS
    ADAPTERS --> CAPTCHA
    ADAPTERS --> GATES
    ADAPTERS --> REST
    ADAPTERS --> GW

    ADAPTERS --> STATE
    STATE --> SQLITE

    SQLITE --> EXPORT
    SQLITE --> DASH
    EXPORT --> HTML
    EXPORT --> CSV
    EXPORT --> MD
    EXPORT --> JSON
```

---

## End-to-End Investigation Lifecycle

The sequence diagram below traces the execution path from initial target verification to evidence finalization.

```mermaid
sequenceDiagram
    autonumber
    actor Operator
    participant CLI as CLI / Engine
    participant Disc as Discord REST API
    participant Solv as CAPTCHA Bridge
    participant Gate as Onboarding Engine
    participant DB as SQLite Store
    participant Exp as Report Exporter

    Operator->>CLI: Run search --username "target" --user-id "snowflake"
    CLI->>Disc: Preflight: Verify target snowflake math & profile
    Disc-->>CLI: Preflight target snapshot
    CLI->>Operator: Display Preflight Confirmation (Y/n)
    Operator->>CLI: Confirm

    CLI->>DB: Atomic CreateRunAtomic(run_id, target_snapshot)
    CLI->>CLI: Load Disboard tags & direct invites (invites.txt)

    loop For each candidate server in queue
        CLI->>Disc: GET /invites/{code} (Resolve metadata)
        Disc-->>CLI: Guild name, ID, member count

        alt Burner account not in server
            CLI->>Disc: POST /invites/{code} (Join request)
            alt CAPTCHA 400 Required
                Disc-->>CLI: CaptchaChallenge (sitekey, rqdata, session_id)
                CLI->>Solv: Start HTTP Bridge (:8765) & launch browser
                Operator->>Solv: Solves hCaptcha challenge in browser
                Solv-->>CLI: Token callback (g-recaptcha-response)
                CLI->>Disc: Retry join with captcha_key & session headers
                Disc-->>CLI: HTTP 200 Joined Guild
            end
        end

        CLI->>Gate: HandleRulesScreening (Accept community rules)
        Gate->>Disc: PUT /guilds/{id}/requests/@me
        Disc-->>Gate: 204 No Content

        CLI->>Disc: Phase 0: Direct member lookup (GetGuildMember)
        Disc-->>CLI: Authoritative member object (Avatar hash, global name, roles)
        CLI->>DB: UpdateRunTargetProfile(avatar_url, global_name)

        CLI->>Disc: Phase 1: SearchGuildMembers & SearchGuildMessages
        Disc-->>CLI: Target messages collected
        CLI->>Disc: GetGuildChannels(guild_id)
        Disc-->>CLI: Channel list (resolve channel IDs -> channel names)

        CLI->>DB: RecordGuildScanTransaction(scan, observations, messages)
        CLI->>CLI: Jitter pause (rate limit compliance)
    end

    CLI->>Exp: ExportArtifacts(run_id)
    Exp->>DB: Query Run, GuildScans, Observations, Messages
    DB-->>Exp: Full forensic dataset
    Exp->>Exp: Generate Glassmorphic report.html (with CDN avatar & server/channel pills)
    Exp->>Exp: Write messages.csv, hits.json, observations.json, report.md
    Exp-->>CLI: Export completed
    CLI-->>Operator: Display summary & path to report.html
```

---

## Two-Tier Member & Message Discovery

To guarantee high coverage while minimizing API footprint, the engine executes a multi-tiered discovery pipeline:

```mermaid
flowchart TD
    START([Begin Guild Inspection]) --> P0[Tier 0: Direct Member Query<br/>GET /guilds/{id}/members/{target_id}]
    
    P0 -->|HTTP 200 Found| CONFIRM0[Confirm Presence: user_id_exact]
    CONFIRM0 --> AVATAR[Extract Real Discord Avatar CDN Hash<br/>and Guild Nickname / Roles]
    AVATAR --> DB_PROF[Update Run Target Profile in SQLite]
    
    P0 -->|HTTP 404 / 403| P1[Tier 1: Targeted REST Query<br/>GET /guilds/{id}/members/search?query=...]
    
    P1 -->|Matched User ID| CONFIRM1[Confirm Presence: user_id_exact]
    P1 -->|Matched Username Only| CAND1[Record Candidate: username_exact]
    P1 -->|Fuzzy Nickname Match >= 0.85| CAND2[Record Candidate: nickname_similar]
    P1 -->|No Member Hit| P2[Tier 2: Guild-Wide Message Probe<br/>GET /guilds/{id}/messages/search?author_id=...]
    
    DB_PROF --> P2
    CONFIRM1 --> P2
    CAND1 --> P2
    CAND2 --> P2
    
    P2 -->|Target Author Hit| CONFIRM2[Confirm Presence: message_author_id_exact]
    CONFIRM2 --> CH_RESOLV[Query GetGuildChannels<br/>Resolve Channel IDs to Names]
    CH_RESOLV --> MSGS_CSV[Append to Messages Collection<br/>with Guild & Channel Names]
    
    P2 -->|0 Messages Returned| COVERAGE[Evaluate Member Coverage<br/>complete vs partial]
    MSGS_CSV --> SAVE[Commit Transaction to Database]
    COVERAGE --> SAVE
    SAVE --> DONE([Inspection Finished])
```

---

## Interactive CAPTCHA Bridge Architecture

When Discord requires a CAPTCHA verification challenge during a guild join request, the engine routes the challenge through a local web bridge:

```mermaid
sequenceDiagram
    autonumber
    participant Engine as Workflow Engine
    participant Discord as Discord Gateway / REST
    participant Bridge as CAPTCHA Local Bridge (:8765)
    participant Browser as Operator's Browser

    Engine->>Discord: POST /api/v9/invites/{code}
    Discord-->>Engine: HTTP 400 Bad Request<br/>{"captcha_key": ["captcha-required"], "captcha_sitekey": "...", "captcha_rqdata": "..."}
    
    Engine->>Bridge: Start Bridge(Challenge{SiteKey, RqData, SessionID})
    Bridge->>Browser: Launch system browser at http://127.0.0.1:8765/captcha
    Browser->>Bridge: GET /captcha
    Bridge-->>Browser: Serve HTML page with explicit hCaptcha widget script
    
    Browser->>Browser: hCaptcha rendered with dynamic sitekey & rqdata
    Browser->>Browser: Operator completes interactive image challenge
    
    Browser->>Bridge: POST /submit {"token": "P1_eyJ..."}
    Bridge-->>Browser: Display "CAPTCHA Solved! You may close this tab."
    Bridge-->>Engine: Return Solution{Token, SessionID}
    
    Engine->>Discord: POST /api/v9/invites/{code}<br/>Header: X-Captcha-Session-Id<br/>Body: {"captcha_key": "P1_...", "captcha_session_id": "...", "session_id": "..."}
    Discord-->>Engine: HTTP 200 OK {"guild": {...}} (Joined successfully)
```

---

## Onboarding & Channel Gate State Machine

Discord servers often guard channels behind membership rules screening, verification bots, or role reaction gates. The state machine navigates these gates autonomously:

```mermaid
stateDiagram-v2
    [*] --> GuildJoined: Successful Invite Join
    
    GuildJoined --> CheckScreening: Inspect Guild Member 'pending' Flag
    
    CheckScreening --> SubmitScreening: pending == true
    SubmitScreening --> RulesAccepted: PUT /guilds/{id}/requests/@me
    CheckScreening --> RulesAccepted: pending == false
    
    RulesAccepted --> InspectChannels: Query Accessible Channels
    
    InspectChannels --> Detection: Analyze Channel Messages for Verification Prompts
    
    Detection --> EmojiReactionGate: Found Reaction Checkmark (✅)
    EmojiReactionGate --> ChannelUnlocked: PUT /channels/{id}/messages/{id}/reactions/✅/@me
    
    Detection --> ComponentButtonGate: Found Verification Button (type: 3)
    ComponentButtonGate --> ChannelUnlocked: POST /interactions (Component interaction)
    
    Detection --> ExternalWebGate: Found Verification Link (AltDentifier / Wick)
    ExternalWebGate --> BrowserOpen: Launch URL in Operator Browser
    BrowserOpen --> ChannelUnlocked: Manual Operator Confirmation
    
    Detection --> ChannelUnlocked: No Gates Detected
    
    ChannelUnlocked --> SearchMessages: Execute Message Search Query
    SearchMessages --> [*]
```

---

## Database Schema & Provenance Model

The platform utilizes a pure-Go SQLite driver (`modernc.org/sqlite`) configured in WAL (Write-Ahead Logging) mode with schema v2 migrations and strict relational integrity constraints:

```mermaid
erDiagram
    runs ||--o{ guild_scans : "executes"
    runs ||--o{ match_observations : "identifies"
    runs ||--o{ messages : "collects"
    runs ||--o{ gate_attempts : "records"
    guild_scans ||--o{ match_observations : "contains"
    guild_scans ||--o{ messages : "contains"

    runs {
        string run_id PK
        string target_input
        string target_user_id
        string target_username
        string target_display_name
        string target_avatar_url
        string target_resolution_source
        timestamp target_verified_at
        string status
        timestamp created_at
    }

    guild_scans {
        int id PK
        string run_id FK
        string guild_id
        string guild_name
        string invite_code
        string scan_status
        string best_match_status
        string best_match_reason
        string best_observed_user_id
        string member_coverage
        int messages_examined
        timestamp started_at
        timestamp completed_at
    }

    match_observations {
        int id PK
        string run_id FK
        string guild_id
        string observed_user_id
        string observed_username
        string observed_nick
        string observed_joined_at
        string observed_roles
        string observed_guild_avatar_url
        string match_status
        string match_reason
        string acquisition_method
        timestamp observed_at
    }

    messages {
        int id PK
        string run_id FK
        string guild_id
        string guild_name
        string channel_id
        string channel_name
        string message_id
        string author_id
        string author_username
        string content
        timestamp timestamp
        timestamp collected_at
        string collector_version
        string acquisition_method
    }

    gate_attempts {
        int id PK
        string run_id FK
        string guild_id
        string gate_type
        string channel_id
        string action_taken
        timestamp attempted_at
    }
```

---

## Installation & Build

### Prerequisites

- **Go**: Version 1.22 or higher.
- **Git**: For version control operations.
- **Operating System**: macOS (Apple Silicon / Intel), Linux, or Windows.

### Build from Source

```bash
# Clone the private repository
git clone https://github.com/Kingof3O/discord-osint.git
cd discord-osint

# Compile self-contained static binary (No CGO required)
CGO_ENABLED=0 go build -o bin/discord-osint ./cmd/discord-osint

# Verify executable build
./bin/discord-osint --help
```

---

## Configuration

Configure environment variables via `.env` or YAML configuration (`config.yaml`):

```bash
cp config.example.yaml config.yaml
```

### Environment Variables Matrix

| Variable | Type | Default | Description |
| :--- | :--- | :--- | :--- |
| `DISCORD_TOKEN` | `string` | *Required* | Burner Discord account user token |
| `DATABASE_PATH` | `string` | `run.sqlite` | Path to SQLite database |
| `PROXY` | `string` | `""` | Optional HTTP or SOCKS5 proxy URL (`socks5://127.0.0.1:9050`) |
| `MAX_JOINS_PER_HOUR` | `int` | `8` | Maximum guild joins allowed per 60-minute window |
| `JOIN_DELAY_SEC` | `string` | `30,90` | Min,Max random jitter interval in seconds between joins |
| `CAPTCHA` | `string` | `manual` | CAPTCHA mode (`manual` to open solver bridge, `skip` to bypass) |
| `CAPTCHA_BRIDGE_PORT`| `int` | `8765` | Local port for interactive solver bridge HTTP server |
| `AUTO_OPEN_BROWSER` | `bool` | `true` | Automatically launch default browser on CAPTCHA challenge |
| `LOG_LEVEL` | `string` | `info` | Logging verbosity (`debug`, `info`, `warn`, `error`) |

---

## CLI Command Reference

### 1. `search` &mdash; Run Multi-Server Investigation Walk

Execute an end-to-end OSINT search walk:

```bash
./bin/discord-osint search \
  --invites-file invites.txt \
  --username "target_user" \
  --user-id "1533658238715695116" \
  --stay \
  --yes
```

#### Flags:
- `--username`: Target Discord username or handle.
- `--user-id`: Target 64-bit Snowflake User ID (authoritative).
- `--invite`: Single invite code or full URL.
- `--invites-file`: Text file with line-separated Discord invite codes/URLs.
- `--server-name`: Target server names to search and prioritize via Disboard.
- `--stay`: Keep burner account in servers after discovering target.
- `--yes`: Auto-confirm target verification preflight prompt.
- `--limit`: Maximum number of servers to walk.

### 2. `export` &mdash; Re-Export Evidence & Reports

Re-generate all forensic artifacts for a historical run:

```bash
./bin/discord-osint export --run-id run_1789521895 -o exports_custom/
```

### 3. `ui` &mdash; Real-Time Investigation Dashboard

Launch the embedded web dashboard:

```bash
./bin/discord-osint ui --port 3000
```

Navigate to `http://127.0.0.1:3000` to inspect runs, live server scans, identity observations, and chat streams.

### 4. `history` &mdash; Display Audit Run History

List all historical investigation runs with confirmed and candidate match counts:

```bash
./bin/discord-osint history
```

### 5. `resume` &mdash; Smart Resume Interrupted Runs

Resume an interrupted or rate-limited scan without re-scanning completed servers:

```bash
# Auto-resumes the most recent incomplete run:
./bin/discord-osint resume

# Or resume a specific run ID:
./bin/discord-osint resume run_1789521895
```

### 6. `verify` &mdash; Read-Only Target Preflight

Perform a standalone target resolution preflight with zero database or file writes:

```bash
./bin/discord-osint verify --user-id "1533658238715695116" --username "target_user"
```

---

## Forensic Artifacts & Glassmorphic Reporting

Every run creates a dedicated export directory `exports_run_<timestamp>/` containing verifiable evidence files:

```
exports_run_1789521895/
├── report.html         # Standalone Glassmorphic visual report
├── report.md           # Executive markdown summary
├── messages.csv        # RFC4180 CSV with server & channel names
├── hits.json           # Machine-readable per-guild match findings
└── observations.json   # Member profile observations & role maps
```

### Visual HTML Report Elements

- **Discord Profile Hero Card**: Real avatar CDN image, verified badges, target handle `@username`, and Snowflake creation timestamps.
- **Scanned Server Audit**: Table detailing scan status, match reason, and member coverage bounds.
- **Identity Observations**: Server nicknames, guild join dates, and assigned role IDs.
- **Authentic Chat Stream**:
  - Author avatar & handle.
  - Server pill: `<span class="msg-server-pill">🏛️ Cornhub</span>`
  - Channel pill: `<span class="msg-chan-pill">#🌸・chat</span>`
  - Message timestamp and escaped raw message content.

---

## Operational Security & Stealth Best Practices

1. **Burner Isolation**: Always use disposable burner Discord accounts. Never use personal or corporate accounts.
2. **Proxy Chaining**: Route requests through residential proxies or SOCKS5 endpoints via the `PROXY` setting.
3. **Jitter & Delays**: Maintain `MAX_JOINS_PER_HOUR` at or below 8 with randomized `JOIN_DELAY_SEC` (30-90s) to prevent automated anti-bot flagging.
4. **Token Scrubbing**: The embedded regex scrubber automatically sanitizes user tokens from terminal outputs, log entries, and database traces.

---

## Troubleshooting & Error Matrix

| Error Code / Symptom | Root Cause | Solution |
| :--- | :--- | :--- |
| `HTTP 401 Unauthorized` | Invalid or expired burner token | Update `DISCORD_TOKEN` in `.env` with a fresh token |
| `HTTP 403 Forbidden on /users` | User tokens cannot query arbitrary user endpoints | Handled automatically: Engine falls back to snowflake math and `GetGuildMember` |
| `HTTP 429 Too Many Requests` | Rate limit hit on Discord endpoint | Engine honors `Retry-After` header and pauses automatically |
| `CAPTCHA required` | Guild requires anti-bot join verification | Solve challenge in solver browser window or set `CAPTCHA=skip` |
| `invalid-input-response` | Captcha token expired before submission | Complete CAPTCHA challenge promptly when browser opens |

---

## License

This project is licensed under the MIT License. See [LICENSE](LICENSE) for details.
