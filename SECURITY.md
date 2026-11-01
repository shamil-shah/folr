# Security Policy

## Supported Versions

Security updates and patches are actively maintained for the latest release:

| Version | Supported          |
| ------- | ------------------ |
| 0.1.x   | :white_check_mark: |

## Reporting a Vulnerability

The `folr` project takes security seriously. If you discover a security vulnerability or bypass of our security constraints, please do **NOT** open a public GitHub issue.

Instead, please report vulnerabilities via **GitHub Private Vulnerability Reporting**:
1. Navigate to the repository's **Security** tab on GitHub.
2. Select **Advisories**.
3. Click **Report a vulnerability** to privately submit details to maintainers.

If private vulnerability reporting is unavailable, please contact the maintainers directly via email.

### What to Include
When reporting a vulnerability, please provide:
- A clear description of the issue and potential security impact.
- Step-by-step reproduction steps or a minimal proof-of-concept (PoC).
- Your environment details (Operating system, architecture, Go version).

### Security Guarantees in `folr`
- **Zero Data Loss**: Files are never deleted or overwritten during organize or undo operations.
- **No Network Activity**: `folr` makes zero external network calls, collects no analytics, and performs no background telemetry.
- **Boundary Defense**: Refuses execution on root, system directories (`/etc`, `/boot`, `/dev`, Windows system directories), and user home directories directly.
- **Race Condition Resistance**: TOCTOU verification immediately prior to relocation and directory-level process locking via `.folr.lock`.

