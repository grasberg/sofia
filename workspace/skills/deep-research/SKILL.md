---
name: deep-research
description: Structured multi-step research methodology. Use when the user asks you to research a topic in depth, investigate a question thoroughly, compile findings from multiple sources, or produce a comprehensive research report. Triggers on phrases like "research this", "deep dive into", "investigate", "find out everything about", or "comprehensive analysis of".
---

# Deep Research

A systematic research methodology that decomposes questions, searches across multiple
sources, cross-references findings, and synthesizes structured reports.

## When to Use

Activate this skill when the user requests:
- In-depth research on any topic
- A comprehensive overview of a subject
- Investigation of a question that requires multiple sources
- A research report with citations
- Comparison or analysis requiring broad evidence gathering

## Phase 1: Question Decomposition

Before searching anything, decompose the research question into 3-5 focused
sub-questions. Each sub-question should target a distinct facet of the topic.

### Steps

1. Restate the user's question in your own words to confirm understanding.
2. Identify the key dimensions: who, what, when, where, why, how, and impact.
3. Generate 3-5 sub-questions that collectively cover the full scope.
4. Order sub-questions by dependency (foundational knowledge first).
5. Write sub-questions to `workspace/research/[topic-slug]/plan.md`.

### Example Decomposition

User asks: "What is the current state of quantum computing?"

Sub-questions:
1. What are the leading quantum computing architectures and their trade-offs?
2. Which companies and labs are at the forefront, and what milestones have they reached?
3. What are the primary use cases where quantum advantage has been demonstrated?
4. What are the remaining technical barriers to practical quantum computing?
5. What is the projected timeline for commercially viable quantum computers?

## Phase 2: Systematic Search

For each sub-question, conduct a structured search.

### Per Sub-Question Process

1. Search for the sub-question using web search tools.
2. Open and read at least 2-3 distinct sources per sub-question.
3. For each source, extract:
   - Key claims and data points
   - Source URL and publication date
   - Author or organization credibility indicators
4. Note any contradictions between sources.
5. Save raw findings to `workspace/research/[topic-slug]/sq-[N]-findings.md`.

### Source Quality Guidelines

- Prefer primary sources (official docs, papers, direct announcements).
- Cross-check claims that appear in only one source.
- Note when sources are outdated (older than 12 months for fast-moving topics).
- Flag opinions vs. facts explicitly.
- If a claim lacks a credible source, mark it as unverified.

## Phase 3: Cross-Reference and Validate

After completing searches for all sub-questions:

1. Identify findings that appear across multiple sub-questions.
2. Check for contradictions between sub-question findings.
3. Resolve contradictions by:
   - Checking which source is more recent
   - Checking which source is more authoritative
   - If unresolvable, present both perspectives
4. Identify gaps where no reliable information was found.
5. Save cross-reference analysis to `workspace/research/[topic-slug]/cross-ref.md`.

## Phase 4: Synthesis and Report

Compile findings into a structured report.

### Report Format

```markdown
# Research Report: [Topic]

**Date:** [YYYY-MM-DD]
**Scope:** [Brief description of what was researched]

## Executive Summary

[2-3 paragraph overview of the most important findings. This should stand alone
as a useful summary for someone who reads nothing else.]

## Key Findings

### [Finding Category 1]

[Detailed findings with inline source citations.]

Source: [URL] (accessed [date])

### [Finding Category 2]

[Continue for each major finding area.]

## Contradictions and Uncertainties

- **[Topic]:** Source A claims X ([URL]), while Source B claims Y ([URL]).
  Assessment: [Which is more likely correct and why, or "unresolved".]

## Data Gaps

- [Areas where reliable information could not be found]
- [Questions that remain unanswered]

## Recommendations

[Actionable next steps based on the findings. If the research was to inform a
decision, provide a clear recommendation with reasoning.]

## Sources

1. [Title] - [URL] (accessed [date])
2. [Continue numbered list]
```

### Report Quality Checklist

Before delivering the report, verify:
- [ ] Every factual claim has a cited source
- [ ] Executive summary captures the essence without requiring the full report
- [ ] Contradictions are explicitly acknowledged, not hidden
- [ ] Data gaps are clearly stated
- [ ] Recommendations follow logically from findings

## Using Subagents

When subagents are available, use them for parallel research:

- Spawn one subagent per sub-question for parallel investigation.
- Provide each subagent with: the sub-question, search guidelines, and output format.
- Collect results and perform cross-referencing in the main thread.
- This significantly reduces research time for broad topics.

## Workspace File Structure

All research artifacts go under `workspace/research/[topic-slug]/`:

```
workspace/research/quantum-computing/
  plan.md              # Sub-questions and research plan
  sq-1-findings.md     # Raw findings for sub-question 1
  sq-2-findings.md     # Raw findings for sub-question 2
  ...
  cross-ref.md         # Cross-reference analysis
  report.md            # Final synthesized report
```

## Important Rules

- Never fabricate sources or citations.
- If you cannot find information, say so explicitly.
- Always include access dates for web sources.
- Distinguish clearly between facts, expert opinions, and your own analysis.
- If the topic is time-sensitive, emphasize the date of your research.
- Prefer depth over breadth: 3 well-researched findings beat 10 shallow ones.
