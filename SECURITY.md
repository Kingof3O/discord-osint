# Security Policy

## 1. Ethical Use & Legal Disclaimer

**Discord OSINT** is an open-source intelligence and forensic investigation utility designed exclusively for authorized cybersecurity analysts, law enforcement entities, verified fraud examiners, and threat intelligence researchers.

- **Authorized Use Only**: Users are responsible for ensuring that all reconnaissance and evidence collection activities comply with applicable national and international laws, regulations, and institutional policies.
- **No Unauthorized Access**: This tool does not exploit vulnerabilities, bypass access controls, or access private channels/DMs without authorization. It queries publicly advertised servers or servers to which the operator has been provided legitimate invitations.
- **Operator Accountability**: The authors, contributors, and maintainers assume no liability for misuse, account terminations, or regulatory non-compliance resulting from the use of this tool.

---

## 2. Supported Versions

Security patches and vulnerability updates are actively maintained for the following versions:

| Version | Supported          |
| :---    | :---               |
| 3.x     | :white_check_mark: |
| < 3.0   | :x:                |

---

## 3. Reporting a Vulnerability

If you discover a security vulnerability, credential leakage, or flaw within this codebase, please submit a report privately:

1. **Do NOT open a public issue.**
2. Email your advisory or proof of concept to the repository owner or submit via GitHub Private Vulnerability Reporting.
3. Include:
   - Detailed description of the vulnerability.
   - Affected files and execution path.
   - Remediation recommendations or proposed pull request.
4. We aim to acknowledge receipt within 48 hours and provide a coordinated patch timeline.

---

## 4. Sensitive Data Governance & Token Safety

This project implements rigorous defensive programming patterns to prevent sensitive credential exposure:

- **Token Redaction (`TokenScrubber`)**:
  All Discord user tokens, session tokens, and proxy credentials passed to `internal/discord` and `internal/captcha` are monitored by an active regex token scrubber. Any logged requests, error strings, and exception backtraces automatically redact matching token bytes to `[REDACTED]`.
- **Zero Credentials in Version Control**:
  `.env`, `*.sqlite*`, `invites.txt`, and `exports_*/` are strictly ignored by `.gitignore`.
- **Database Partitioning**:
  Evidence databases (`run.sqlite`) are stored locally in the working directory and never uploaded or synchronized across remote endpoints without operator intervention.
