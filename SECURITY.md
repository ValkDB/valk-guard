# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in Valk Guard, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, email: **security@valkdb.com**

Include:
- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

We will acknowledge receipt within 48 hours and aim to provide a fix or mitigation plan within 7 days.

## Scope

Valk Guard is a static analysis tool that reads source files. It does not:
- Execute SQL statements
- Connect to databases
- Make network requests

Security concerns are primarily around:
- Path traversal in file scanning
- Arbitrary code execution via crafted input files
- Dependency vulnerabilities
