# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.x.x   | :white_check_mark: |
| < 1.0   | :x:               |

## Reporting a Vulnerability

We take security seriously at NannyAPI. If you discover a security vulnerability, please follow these steps:

1. **Do Not** open a public issue
2. Send an email to security@nannyai.dev with:
   - A description of the vulnerability
   - Steps to reproduce
   - Potential impact
   - Suggested fixes (if any)

## What to Expect

- You will receive an acknowledgment within 48 hours
- We will investigate and provide regular updates
- Once fixed, we will:
  - Notify you
  - Release a patch
  - Acknowledge your contribution (if desired)
  - Create a security advisory

## Scope

The following are in scope for security reports:
- The NannyAPI server code
- API authentication mechanisms
- Data handling and storage
- Token management
- Dependencies with known vulnerabilities

## Security Measures

NannyAPI implements several security measures:
- All API endpoints require authentication
- Tokens are encrypted at rest
- HTTPS required for all communications
- Regular dependency updates
- Automated security scanning
- Input validation and sanitization
