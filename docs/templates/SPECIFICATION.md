# [Project Name] — Specification

<!-- Optional YAML frontmatter
title: "[Project Name] Specification"
date: YYYY-MM-DD
author: [Author Name]
status: draft | in_review | approved
version: 1.0
-->

## Problem

Describe the problem this project solves. What pain points exist? Why is a solution needed?

## Goal

State the primary objective. What will this project achieve? Include success metrics if applicable.

## Architecture

High-level design overview. Diagrams can be described or linked.

### Components

List and describe the main components/modules.

| Component | Purpose | Technology |
|-----------|---------|------------|
| Component A | Handles X | Go, SQLite |
| Component B | Provides Y | React, TypeScript |

### Data Flow

Describe how data moves through the system.

### Interfaces

APIs, user interfaces, or integration points.

## Files to Create

| File | Purpose |
|------|---------|
| `path/to/file.go` | Core logic for ... |
| `path/to/file.md` | Documentation for ... |

## Files to Modify

| File | Change |
|------|--------|
| `existing/file.go` | Add new method `X` |
| `config.yaml` | Update setting `Y` |

## Testing

Describe testing strategy: unit tests, integration tests, performance tests.

## Verification

How will we know the project is complete? List acceptance criteria.

- [ ] Feature X works according to spec
- [ ] Performance target Y achieved
- [ ] Documentation updated

## Dependencies

External libraries, services, or systems required.

## Risks & Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Technology unfamiliarity | Medium | High | Pair with expert, spike prototype |
| Timeline overrun | High | Medium | Break into smaller milestones |

## Timeline

Optional: high-level milestones with estimated dates.

## Glossary

Define domain-specific terms.

---

*This document follows the [Specification Template](SPECIFICATION.md).*