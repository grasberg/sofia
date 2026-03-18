---
name: competitive-analysis
description: Structured methodology for researching and comparing competitors, products, technologies, or alternatives. Use when the user asks to evaluate options, compare products or tools, analyze the competitive landscape, or recommend between alternatives. Triggers on phrases like "compare these", "which is better", "competitive analysis", "evaluate alternatives", "what are the options", or "recommend a tool/product/service".
---

# Competitive Analysis

A structured methodology for researching, comparing, and evaluating competitors,
products, or technologies to produce an actionable recommendation.

## When to Use

Activate this skill when:
- The user asks to compare two or more products, tools, or services
- The user wants to understand the competitive landscape in a space
- The user asks "which should I use" or "what are the best options"
- A technology decision requires evaluating alternatives
- The user asks for a recommendation between competing options

## Phase 1: Define Comparison Dimensions

Before researching, establish what matters.

### Default Dimensions

Start with these standard dimensions and adjust based on the specific context:

| Dimension       | Description                                        |
|----------------|----------------------------------------------------|
| Features       | Core capabilities and functionality                 |
| Pricing        | Cost structure, tiers, free plans, hidden costs     |
| Performance    | Speed, reliability, scalability                     |
| Ease of use    | Learning curve, documentation, UX quality           |
| Ecosystem      | Integrations, plugins, community, third-party tools |
| Maturity       | Age, stability, release cadence, track record       |
| Support        | Customer support quality, SLAs, community help      |
| Licensing      | Open source vs proprietary, license terms           |

### Customizing Dimensions

1. Review the user's request for explicit priorities.
2. Add domain-specific dimensions (e.g., "security certifications" for enterprise
   software, "battery life" for hardware).
3. Remove irrelevant dimensions.
4. Weight dimensions by importance to the user's specific situation.
5. Document the final dimension list and rationale.

Save to `workspace/analysis/[topic-slug]/dimensions.md`.

## Phase 2: Identify Candidates

Find all relevant options to compare.

### Discovery Process

1. Start with any candidates the user mentioned explicitly.
2. Search for "[category] alternatives" and "[category] comparison".
3. Check "awesome lists", comparison sites, and review aggregators.
4. Include both well-known and emerging options.
5. Aim for 3-7 candidates. Fewer than 3 is too narrow; more than 7 is unwieldy.

### Candidate Inclusion Criteria

Include a candidate if:
- It is actively maintained (updated within the last 12 months).
- It has meaningful adoption (users, stars, downloads, or revenue).
- It addresses the user's core use case.

Exclude a candidate if:
- It is abandoned or deprecated.
- It is in early alpha with no production users.
- It does not address the user's actual need.

Document all candidates with a brief rationale for inclusion or exclusion:

```markdown
## Candidates

### Included
1. **[Name]** — [One-line description]. Included because: [reason]
2. **[Name]** — [One-line description]. Included because: [reason]

### Excluded
1. **[Name]** — Excluded because: [reason]
```

## Phase 3: Research Each Candidate

For each candidate, gather structured data across all dimensions.

### Per-Candidate Research

1. Visit the official website and documentation.
2. Check pricing pages (note the date; pricing changes frequently).
3. Search for recent reviews and user experiences.
4. Check GitHub (if open source) for activity, issues, and community health.
5. Look for benchmark data or performance comparisons.
6. Note the source for every data point.

### Per-Candidate Data Sheet

```markdown
## [Candidate Name]

**Website:** [URL]
**Category:** [What it is]
**Latest version:** [version, date]

### Features
- [Feature 1]: [details]
- [Feature 2]: [details]
(Source: [URL])

### Pricing
- Free tier: [details or "none"]
- Paid plans: [tier names, prices]
- Enterprise: [available? custom pricing?]
(Source: [URL], accessed [date])

### Performance
- [Benchmark or claim with source]

### Ease of Use
- Documentation quality: [poor/fair/good/excellent]
- Getting started: [description of onboarding experience]

### Ecosystem
- Integrations: [list notable ones]
- Community: [size indicators — GitHub stars, Discord members, etc.]

### Known Limitations
- [Limitation 1]
- [Limitation 2]

### Data Confidence
- High confidence: [dimensions with reliable data]
- Low confidence: [dimensions where data is sparse or outdated]
```

Save each to `workspace/analysis/[topic-slug]/candidate-[name].md`.

## Phase 4: Build Comparison Table

Synthesize research into a side-by-side comparison.

### Table Format

```markdown
## Comparison Table

| Dimension    | Candidate A      | Candidate B      | Candidate C      |
|-------------|------------------|------------------|------------------|
| Features    | [summary]        | [summary]        | [summary]        |
| Pricing     | [from $X/mo]     | [from $Y/mo]     | [free / $Z/mo]   |
| Performance | [fast/moderate]  | [fast]           | [slow]           |
| Ease of use | [steep curve]    | [easy]           | [moderate]       |
| Ecosystem   | [large]          | [growing]        | [small]          |
| Maturity    | [5 years]        | [2 years]        | [8 years]        |
| Support     | [email only]     | [24/7 chat]      | [community]      |
| Licensing   | [MIT]            | [proprietary]    | [AGPL]           |
```

### Scoring (Optional)

If the user wants a quantitative comparison, score each dimension 1-5:

```markdown
## Scores (1=poor, 5=excellent)

| Dimension    | Weight | Candidate A | Candidate B | Candidate C |
|-------------|--------|-------------|-------------|-------------|
| Features    | 3x     | 4 (12)      | 5 (15)      | 3 (9)       |
| Pricing     | 2x     | 3 (6)       | 2 (4)       | 5 (10)      |
| Performance | 2x     | 4 (8)       | 5 (10)      | 2 (4)       |
| **Total**   |        | **26**      | **29**      | **23**      |
```

Weights should reflect the user's stated priorities.

## Phase 5: Strengths and Weaknesses

For each candidate, identify standout strengths and notable weaknesses.

### Format

```markdown
## Candidate A

### Strengths
- [Strength 1]: [Why this matters for the user's use case]
- [Strength 2]: [Why this matters]

### Weaknesses
- [Weakness 1]: [Impact on the user's use case]
- [Weakness 2]: [Impact]

### Best for
[The type of user or use case where this candidate is the clear winner]
```

## Phase 6: Synthesize Recommendation

Produce a final recommendation with clear reasoning.

### Report Format

Save to `workspace/analysis/[topic-slug]/report.md`:

```markdown
# Competitive Analysis: [Topic]

**Date:** [YYYY-MM-DD]
**Prepared for:** [Context of the decision]

## Executive Summary

[2-3 paragraphs: what was compared, the key differentiators, and the
recommendation. A reader should be able to make a decision from this
section alone.]

## Comparison Table

[The table from Phase 4]

## Per-Candidate Analysis

### [Candidate A]
[Deep dive: strengths, weaknesses, best for, concerns]

### [Candidate B]
[Deep dive]

### [Candidate C]
[Deep dive]

## Data Gaps and Confidence

- **High confidence:** [What we know well]
- **Low confidence:** [Where data is incomplete or potentially outdated]
- **Unable to verify:** [Claims we could not independently confirm]

## Recommendation

**Primary recommendation:** [Candidate name]

**Reasoning:**
1. [Reason 1, tied to a specific dimension]
2. [Reason 2]
3. [Reason 3]

**When to choose an alternative:**
- Choose [Candidate B] if [specific condition].
- Choose [Candidate C] if [specific condition].

**Risks with the recommendation:**
- [Risk 1 and mitigation]
- [Risk 2 and mitigation]

## Sources

1. [Source with URL and access date]
2. [Continue numbered list]
```

## Workspace File Structure

```
workspace/analysis/[topic-slug]/
  dimensions.md              # Comparison dimensions and weights
  candidate-[name].md        # Per-candidate data sheets
  report.md                  # Final analysis report
```

## Important Rules

- **Never recommend without reasoning.** Every recommendation must explain WHY.
- **Acknowledge data gaps.** If you could not verify a claim, say so.
- **Date everything.** Pricing and features change. Include access dates.
- **Present the runner-up.** Always explain when the alternative would be the
  better choice.
- **Avoid bias toward popular options.** Evaluate on the stated dimensions, not
  on brand recognition.
- **Keep it actionable.** The user should be able to make a decision after reading
  the executive summary.
