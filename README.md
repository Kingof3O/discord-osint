# Discord OSINT Investigation Platform

A production-grade, hardened Discord OSINT (Open Source Intelligence) search and reconnaissance workflow engine written in Go. Designed for cybersecurity analysts, fraud investigators, and authorized threat intelligence operators to conduct forensic presence verification across Discord servers with strict identity separation, audit logging, and modern Glassmorphism visual reporting.

---

## Key Features

- **Authoritative Target Verification Preflight**:
  - Validates targets using Discord Snowflake math (extracts exact UTC account creation date and internal worker IDs).
  - Cross-references user identity against Discord User REST APIs.
  - Interactive operator preflight with fuzzy username matching and safety overrides.

- **Multi-Vector Server Discovery**:
  - Built-in Disboard category scraper supporting keyword tags, server metadata, and member count filtering.
  - Direct invite link and file loading (`--invite`, `--invites-file`) with automatic invite code normalization.

- **Hardened Anti-Bot & Stealth Engine**:
  - Embedded Chromium User-Agent headers, `X-Super-Properties` payload generation, and browser fingerprint emulation.
  - Automatic token scrubber redacting credentials from console outputs, logs, and database records.
  - Jittered rate limiting with exponential backoff on HTTP 429 and server faults.

- **Two-Tier Target Member Discovery**:
  - **Tier 1 (Fast-Path Probe)**: Direct guild member profile query extracting authoritative display names, server nicknames, assigned roles, and real avatar CDN hashes.
  - **Tier 2 (Search API Probe)**: REST member query and guild-wide author message search across regular and age-restricted (NSFW) channels.

- **Interactive Manual & Autonomous CAPTCHA Bridge**:
  - Detects hCaptcha / Cloudflare challenges on guild join attempts.
  - Spawns an isolated local solver bridge with automatic browser orchestration for human-in-the-loop CAPTCHA solving.

- **Discord-Styled Glassmorphic Reporting**:
  - Self-contained, portable offline HTML investigation reports (`report.html`).
  - Dark Discord aesthetic with frosted glass panels (`backdrop-filter: blur(20px)`), radial glowing mesh backdrops, and glowing KPI counters.
  - Discord Target Profile Hero Card displaying real Discord avatar, global display name, `@username` handle, snowflake account age, and audit badges.
  - Authentic Discord chat stream showing all captured evidence messages with user avatars, channel pills, and timestamp metadata.

- **Embedded Real-Time Web Dashboard**:
  - Live investigation dashboard served via `discord-osint ui` with multi-run history, real-time progress auto-refresh, and direct artifact inspection.

- **Forensic Artifact Preservation**:
  - Comprehensive export suite in JSON, CSV, Markdown, and HTML (`hits.json`, `observations.json`, `messages.csv`, `report.md`, `report.html`).
  - Immutable SQLite audit database tracking every request, member observation, message, and onboarding gate attempt.

---

## Architecture

```
┌───────────────────────────────────────────────────────────────┐
│                   CLI Entrypoint (Cobra)                      │
│      search | history | resume | ui | verify | disboard       │
└──────────────────────────────┬────────────────────────────────┘
                               │
┌──────────────────────────────▼────────────────────────────────┐
│                   Workflow Orchestrator                       │
│    • Target Verification Preflight    • Cooldown & Jitter     │
│    • Disboard & Invites Loader        • Multi-Server Walk     │
└───────────┬──────────────────┬──────────────────┬─────────────┘
            │                  │                  │
┌───────────▼──────────┐ ┌─────▼──────────┐ ┌─────▼───────────┐
│    Discord Adapter   │ │ CAPTCHA Bridge │ │ SQLite Store    │
│  • REST & SuperProps │ │ • Local HTTP   │ │ • Runs & Scans  │
│  • Member Discovery  │ │ • Browser Auto │ │ • Observations  │
│  • Message Search    │ │ • Token Relays │ │ • Messages CSV  │
└──────────────────────┘ └────────────────┘ └─────┬───────────┘
                                                  │
                                      ┌───────────▼───────────┐
                                      │ Exporters & UI Engine │
                                      │  • Glassmorphic HTML  │
                                      │  • Web Dashboard      │
                                      │  • Forensic CSV/JSON  │
                                      └───────────────────────┘
```

---

## Getting Started

### Prerequisites

- **Go**: Version 1.22 or newer.
- **Operating System**: macOS, Linux, or Windows.
- **Discord User Token**: A burner Discord account token with standard membership access.

### Installation

Clone the repository and build the binary:

```bash
git clone https://github.com/Kingof3O/discord-osint.git
cd discord-osint

# Compile single standalone binary (no CGO required)
CGO_ENABLED=0 go build -o bin/discord-osint ./cmd/discord-osint
```

Verify installation:

```bash
./bin/discord-osint --help
```

---

## Configuration

Create a `.env` file in the working directory (or use `config.yaml`):

```bash
cp config.example.yaml config.yaml
```

Example `.env`:

```env
# Discord Burner Account User Token (Required)
DISCORD_TOKEN=your_burner_token_here

# Optional HTTP/SOCKS5 Proxy
PROXY=

# Rate limit controls
MAX_JOINS_PER_HOUR=8
JOIN_DELAY_SEC=30,90

# CAPTCHA solver mode ("manual" or "skip")
CAPTCHA=manual
CAPTCHA_BRIDGE_PORT=8765
AUTO_OPEN_BROWSER=true

# Database storage
DATABASE_PATH=run.sqlite
LOG_LEVEL=info
```

---

## Usage Guide

### 1. Execute an OSINT Search Walk

Search across Discord servers from an invites text file:

```bash
./bin/discord-osint search \
  --invites-file invites.txt \
  --username "target_username" \
  --user-id "1533658238715695116" \
  --stay \
  --yes
```

#### Key Search Flags:
| Flag | Description |
| :--- | :--- |
| `--username` | Target Discord username or display name to locate |
| `--user-id` | Target 64-bit Snowflake User ID (authoritative match) |
| `--invite` | Single or multiple direct invite codes/URLs |
| `--invites-file` | Path to text file containing list of Discord invites |
| `--server-name` | Specific server names to query on Disboard first |
| `--stay` | Retain server memberships after locating target |
| `--yes` | Non-interactive auto-confirm for preflight prompt |
| `--limit` | Maximum number of servers to inspect in a single run |

### 2. Launch the Web Dashboard

Start the built-in investigation dashboard:

```bash
./bin/discord-osint ui --port 8080
```

Open `http://localhost:8080` in any browser to inspect runs, server scan states, member profiles, and message evidence in real-time.

### 3. Inspect Run History

List all historical investigation runs with confirmed/candidate hit counts:

```bash
./bin/discord-osint history
```

### 4. Smart Resume an Interrupted Run

Resume an aborted or rate-limited run without repeating completed server scans:

```bash
./bin/discord-osint resume run_1789521895
```

---

## Output Artifacts

Each search run exports structured forensic evidence to `exports_run_<timestamp>/`:

- **`report.html`**: Standalone Glassmorphism HTML report with authentic Discord user cards, real CDN avatar resolution, glowing KPI cards, and all captured messages.
- **`report.md`**: Human-readable executive markdown report with observation audit proofs.
- **`hits.json`**: Machine-readable JSON array of servers where the target was confirmed or candidate matched.
- **`observations.json`**: Granular profile snapshot (guild ID, nickname, roles, join timestamp, avatar URL).
- **`messages.csv`**: Verifiable message archive containing full content, message IDs, timestamps, author IDs, and channel IDs.

---

## Security & Operational Safety

- **Identity Partitioning**: Never run investigations using personal or primary Discord accounts. Always operate through dedicated, partitioned burner accounts.
- **Audit Logging**: All database records preserve timestamps, acquisition methods, and collector version provenance for forensic verification.
- **Rate Limit Compliance**: The workflow enforces configurable cooldowns and honors HTTP 429 `Retry-After` headers to avoid account flagging.

---

## License

This project is licensed under the MIT License.
