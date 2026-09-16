# Operational Security (OpSec) & Investigation Tradecraft Guide

This guide establishes standard operating procedures (SOP) for conducting authorized investigations with the **Discord OSINT Investigation Platform** while safeguarding operator identity and preventing account flags.

---

## 1. Identity Partitioning & Burner Account Hygiene

Conducting OSINT investigations on personal or primary Discord accounts introduces severe operational risks, including account suspension and correlation attacks.

### Mandatory Rules:
- **Dedicated Burner Accounts**: Use only throwaway/burner accounts created specifically for OSINT operations.
- **Never Link Personal Identifiers**:
  - Do not link personal phone numbers, recovery email addresses, or PayPal/credit card payment methods to burner accounts.
  - Use privacy-focused, disposable email services or dedicated VOIP numbers for SMS verification.
- **Profile Neutrality**:
  - Maintain a generic avatar, bio, and username that aligns naturally with gaming or community server demographics to avoid triggering human moderation suspicion.
- **Activity Isolation**:
  - Never join personal servers or communicate with real-life contacts using an operational burner account.

---

## 2. Network Isolation & Proxy Chaining

Discord monitors client IP addresses for rapid geographical shifts, datacenter IP ranges, and abnormal API call volumes.

### Best Practices:
- **Residential Proxies**: When configuring the `PROXY` parameter in `.env`, prioritize residential or mobile proxy pools over static datacenter proxies (AWS, DigitalOcean, Linode) which are aggressively flagged by Cloudflare and Discord anti-bot systems:
  ```env
  PROXY=socks5://username:password@residential.proxyprovider.com:10000
  ```
- **Consistent Geo-Location**: Ensure your proxy IP matches the region associated with the burner account's registration IP.
- **VPN / Tor Routing**: If routing through Tor (`socks5://127.0.0.1:9050`), expect frequent Cloudflare and hCaptcha verification challenges. Ensure `CAPTCHA=manual` is enabled to solve them in the browser bridge.

---

## 3. Rate Limiting & Join Pacing

Discord enforces strict heuristics against automated server joins:

- **Max Joins Limit**: Keep `MAX_JOINS_PER_HOUR` at or below **8** (default is 8). Exceeding 10-15 joins in an hour frequently triggers automated account verification locks or temporary account bans.
- **Jitter Delays**: Keep `JOIN_DELAY_SEC` set to a realistic human interval, e.g. `30,90` (between 30 and 90 seconds random pause between server joins).
- **Graceful Backoff**: If Discord returns `HTTP 429 Too Many Requests`, the engine pauses automatically according to the `Retry-After` header. Do not interrupt the cooldown.

---

## 4. Forensic Evidence Preservation & Chain of Custody

When collecting evidence for legal proceedings or formal incident reports, maintaining chain-of-custody integrity is essential:

### 1. Hash Verification
After a run completes and artifacts are generated in `exports_run_<id>/`, compute SHA-256 checksums immediately:
```bash
# macOS:
shasum -a 256 exports_run_*/* > checksums.sha256

# Linux:
sha256sum exports_run_*/* > checksums.sha256
```

### 2. Time Synchronization
All database records (`run.sqlite`) and exported artifacts (`report.html`, `messages.csv`, `report.md`) record timestamps in **UTC (ISO 8601 / RFC 3339)** format to ensure cross-timezone evidentiary consistency.

### 3. Secure Destruction (Post-Investigation Sanitization)
Once evidence has been transferred to a secure forensic vault, safely destroy local operational artifacts:
```bash
# Securely wipe local database and exports:
rm -rf exports_run_* run.sqlite run.sqlite-wal run.sqlite-shm
```
