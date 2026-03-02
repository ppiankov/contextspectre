# Security Policy

## Supported versions

| Version | Supported |
|---------|-----------|
| 0.1.x   | Yes       |

## Reporting a vulnerability

If you discover a security vulnerability, please report it responsibly:

1. **Do not** open a public GitHub issue
2. Email security@obstalabs.dev with details
3. Include steps to reproduce if possible

We will acknowledge receipt within 48 hours and aim to provide a fix within 7 days for critical issues.

## Scope

ContextSpectre operates on local files only. It does not make network connections, does not transmit data, and does not access credentials. The primary security consideration is safe file handling:

- Mandatory backup before any modification
- Atomic writes (temp file + rename)
- No execution of content from JSONL files
- Read-only mode for active sessions
