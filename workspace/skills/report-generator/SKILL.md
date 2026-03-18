---
name: report-generator
description: Generate structured, professional reports from data and research. Use when producing daily digests, analysis reports, project summaries, status updates, comparison reports, or any document that requires organized presentation of findings with conclusions and recommendations.
---

# Report Generator

Instructions for producing clear, structured, and actionable reports from any data source.

## Report Structure

Every report follows this skeleton. Omit sections only when they genuinely do not apply.

```
# [Report Title]
**Date**: YYYY-MM-DD
**Author**: Sofia
**Status**: Draft | Final

## Executive Summary
[3-5 bullet points — the entire report in 30 seconds]

## Body
[Organized by topic, with headers and subheaders]

## Conclusions
[Key findings distilled into actionable statements]

## Recommendations
[Numbered list of next steps with owners and timelines]

## Appendix (if needed)
[Raw data, detailed tables, methodology notes]
```

## Executive Summary

The most important section. Many readers will read only this.

- **Exactly 3-5 bullet points** — no more, no less.
- Each bullet is a complete, standalone finding.
- Lead with the most important or surprising finding.
- Include numbers and specifics, not vague statements.

Bad: "Performance has changed."
Good: "API response time increased 40% (220ms to 310ms) since the March 12 deployment."

## Body Sections

### Use the Right Format for the Content

| Content Type | Format |
|---|---|
| Comparisons | Tables |
| Action items | Checkbox lists |
| Sequential events | Numbered timeline |
| Analysis and reasoning | Prose paragraphs |
| Metrics and KPIs | Tables or bullet lists with values |
| Code or configuration | Fenced code blocks |

### Data Presentation

- Always include units (ms, %, MB, requests/sec).
- Show change direction and magnitude: "+15% (was 200, now 230)".
- Compare against baselines or previous periods when available.
- Round numbers appropriately — do not report 12.3456789%.

### Confidence Levels

Tag findings with confidence when the data supports varying degrees of certainty:

- **High confidence**: Multiple data sources confirm, clear causal mechanism.
- **Medium confidence**: Single data source or correlation without confirmed causation.
- **Low confidence**: Incomplete data, inference based on limited evidence.

Format: "(confidence: high)" inline, or a confidence column in tables.

## Report Types

### Daily Digest

Aggregate changes since the last report:

1. What happened (events, completions, changes).
2. What is new (new items, alerts, findings).
3. What needs attention (blocked items, anomalies, upcoming deadlines).
4. Metrics snapshot (key numbers compared to yesterday).

Keep to one page. Link to details instead of inlining them.

### Analysis Report

Deep dive into a specific topic:

1. State the question or hypothesis clearly.
2. Describe the methodology (what data, how collected, what tools).
3. Present findings with evidence.
4. Discuss limitations and alternative explanations.
5. Conclude with recommendations.

### Project Status Report

Track progress against goals:

1. Overall status: On Track / At Risk / Blocked.
2. Milestones: completed, in progress, upcoming.
3. Blockers and risks with mitigation plans.
4. Key metrics (velocity, burn rate, coverage).
5. Next period priorities.

### Comparison Report

Side-by-side evaluation of options:

1. Define criteria and weights.
2. Score each option against criteria.
3. Present a summary table.
4. Provide a recommendation with rationale.

## Recurring Reports

For reports that are generated regularly:

- Maintain a consistent template so readers know where to find information.
- Highlight what changed since the last report — do not make the reader diff it mentally.
- Use "NEW" and "CHANGED" markers for items that are different from the previous report.
- Archive previous reports for trend analysis.

## Data Sources

Always cite data sources:

- File paths for local data.
- URLs for web sources.
- Tool names and commands for generated data.
- Timestamps for when data was collected.

If data is stale (older than 24 hours for fast-moving metrics), note it explicitly.

## Recommendations Section

Every report should end with actionable recommendations:

- Number each recommendation.
- Make each one specific and actionable (who should do what by when).
- Prioritize: list the highest-impact recommendation first.
- Distinguish between "must do" (critical) and "should consider" (optional).

## Formatting Standards

- Use ISO dates (YYYY-MM-DD).
- Use consistent header hierarchy (H1 for title, H2 for sections, H3 for subsections).
- Keep paragraphs short — 3-5 sentences maximum.
- Use bold for key terms and metrics on first mention.
- Use code blocks for any technical content (commands, configs, queries).
