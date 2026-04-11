# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.0.x   | :white_check_mark: |

## Reporting a Vulnerability

We take security seriously. If you discover a vulnerability in Sofia, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please:

1. **Email** your findings to the maintainers (see repository contact info)
2. **Encrypt** sensitive details using our PGP key if available
3. **Include** the following in your report:
   - Type of vulnerability
   - Full paths of source files related to the vulnerability
   - Steps to reproduce
   - Proof-of-concept or exploit code
   - Potential impact

## What We Promise

- We will acknowledge your email within **48 hours**
- We will provide a detailed response within **7 days** with our assessment
- We will keep you informed of the progress toward a fix
- We will credit you in the security advisory (unless you prefer to remain anonymous)
- We will not take legal action against researchers who follow this policy

## Security Features

Sofia includes several built-in security features:

- **35+ prompt injection defenses** across 6 languages
- **PII detection** with Luhn/RFC1918 validation
- **Secret scrubbing** on both inbound and outbound messages
- **AES-256-GCM encryption** for sensitive data
- **Action confirmation** for high-risk operations
- **Budget management** to prevent runaway token usage

## Responsible Disclosure

We believe in responsible disclosure and ask that you:

- Give us reasonable time to fix the issue before public disclosure
- Avoid accessing or modifying other users' data
- Not degrade the service or attack other users
- Act in good faith to protect user privacy and safety

Thank you for helping keep Sofia secure! 🛡️