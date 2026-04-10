# Contributing to Sofia

Thanks for your interest in contributing to Sofia! Whether it's a bug report, feature idea, documentation improvement, or code contribution — every bit helps.

## Getting Started

1. **Fork** the repository and clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/sofia.git
   cd sofia
   ```

2. **Install dependencies and build:**
   ```bash
   make deps
   make build
   ```

3. **Run Sofia locally:**
   ```bash
   ./build/sofia onboard   # First time only
   ./build/sofia gateway
   ```

4. **Create a branch** for your change:
   ```bash
   git checkout -b my-feature
   ```

## How to Contribute

### Reporting Bugs

Open a [bug report](https://github.com/grasberg/sofia/issues/new?template=bug_report.md) with:
- What you expected to happen
- What actually happened
- Steps to reproduce
- Your Go version and OS

### Suggesting Features

Open a [feature request](https://github.com/grasberg/sofia/issues/new?template=feature_request.md) describing your idea and why it would be useful.

### Submitting Code

1. Make sure your code builds cleanly (`make build`)
2. Follow the existing code style — Sofia is written in idiomatic Go
3. Keep commits focused — one logical change per commit
4. Write a clear PR description explaining *what* and *why*

### Improving Documentation

Documentation improvements are always welcome. If you spot a typo, unclear instruction, or missing info — feel free to open a PR.

## Code Structure

```
sofia/
├── cmd/           # CLI entry points
├── pkg/           # Core packages
├── assets/        # Static web UI assets
├── docs/          # Documentation
├── workspace/     # Default workspace templates
├── Makefile       # Build targets
└── go.mod         # Go module definition
```

## Good First Issues

Look for issues labeled [`good first issue`](https://github.com/grasberg/sofia/labels/good%20first%20issue) — these are specifically chosen to be approachable for new contributors.

## Questions?

Open a [Discussion](https://github.com/grasberg/sofia/discussions) or file an issue — happy to help.

---

Thank you for helping make Sofia better!
